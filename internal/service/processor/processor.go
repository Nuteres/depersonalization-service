package processor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/audit"
	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
	"dep_service/pkg/ziputil"
)

// Processor handles streaming file anonymization, cancellation, and key generation.
type Processor struct {
	km        *keymanager.KeyManager
	inspector *inspector.Inspector
	audit     *audit.AuditLogger
}

// ProcessorService is an alias to Processor for architectural consistency.
type ProcessorService = Processor

// New creates a new Processor instance.
// If km is nil, a default KeyManager is created.
// If insp is omitted or nil, a default Inspector is created.
func New(km *keymanager.KeyManager, insp ...*inspector.Inspector) *Processor {
	var in *inspector.Inspector
	if len(insp) > 0 && insp[0] != nil {
		in = insp[0]
	} else {
		in = inspector.New()
	}
	if km == nil {
		km = keymanager.New()
	}
	al, _ := audit.DefaultLogger()
	return &Processor{
		km:        km,
		inspector: in,
		audit:     al,
	}
}

// SetAuditLogger sets the audit logger for the processor.
func (p *Processor) SetAuditLogger(l *audit.AuditLogger) {
	p.audit = l
}

// AuditLogger returns the current audit logger.
func (p *Processor) AuditLogger() *audit.AuditLogger {
	return p.audit
}

// ValidTargetExtensions returns the allowed target extensions for a given input file extension.
func ValidTargetExtensions(inputExt string) []string {
	switch strings.ToLower(inputExt) {
	case ".csv", ".tsv":
		return []string{".csv", ".tsv"}
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return []string{".xlsx", ".xlsm", ".xltx", ".xltm"}
	case ".ods", ".xods":
		return []string{".ods", ".xlsx"}
	case ".xlsb", ".xls":
		return []string{".xlsx"}
	case ".docx":
		return []string{".docx"}
	case ".odt":
		return []string{".odt"}
	case ".txt":
		return []string{".txt"}
	case ".pdf":
		return []string{".pdf"}
	default:
		return nil
	}
}

// Process processes the input file according to options and reports progress via reporter.
// It supports streaming anonymization, graceful context cancellation, Windows-safe cleanup,
// and optional .depmap key container generation.
func (p *Processor) Process(ctx context.Context, opts domain.ProcessOptions, reporter domain.ProgressReporter) (res *domain.ProcessResult, err error) {
	var (
		sourceHash     string
		resultHash     string
		totalRows      int
		entities       []string
		keyCreated     bool
		archiveCreated bool
		outPath        = opts.OutputPath
	)

	if opts.Mode == "" {
		opts.Mode = domain.ModeReplace
	}

	if strings.TrimSpace(opts.InputPath) != "" {
		sourceHash, _ = audit.ComputeFileSHA256(opts.InputPath)
	}

	defer func() {
		if p.audit == nil {
			return
		}
		status := "success"
		errMsg := ""
		if err != nil {
			if errors.Is(err, context.Canceled) || (ctx != nil && ctx.Err() != nil && errors.Is(ctx.Err(), context.Canceled)) {
				status = "canceled"
			} else {
				status = "error"
			}
			errMsg = err.Error()
		} else {
			if outPath != "" {
				resultHash, _ = audit.ComputeFileSHA256(outPath)
			}
		}

		_ = p.audit.Log(audit.AuditRecord{
			Operation:    "depersonalize",
			Mode:         string(opts.Mode),
			SourceFile:   opts.InputPath,
			SourceHash:   sourceHash,
			ResultFile:   outPath,
			ResultHash:   resultHash,
			RecordsCount: totalRows,
			Entities:     entities,
			KeyCreated:   keyCreated,
			Status:       status,
			Error:        errMsg,
		})
	}()

	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if strings.TrimSpace(opts.InputPath) == "" {
		return nil, errors.New("file path cannot be empty")
	}

	stat, err := os.Stat(opts.InputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", opts.InputPath, err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%w: path is a directory, not a file: %s", domain.ErrUnsupportedFormat, opts.InputPath)
	}
	ext := strings.ToLower(filepath.Ext(opts.InputPath))
	if ext == ".pdf" && stat.Size() > domain.MaxPDFInteractiveFileSize {
		return nil, fmt.Errorf("%w: размер PDF документа (%s) превышает допустимый лимит безопасности %s",
			domain.ErrFileTooLarge,
			formatBytes(stat.Size()),
			formatBytes(domain.MaxPDFInteractiveFileSize),
		)
	}

	validExts := ValidTargetExtensions(ext)
	if len(validExts) == 0 {
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, ext)
	}

	defaultTargetExt := validExts[0]

	if strings.TrimSpace(outPath) == "" {
		dir := filepath.Dir(opts.InputPath)
		base := strings.TrimSuffix(filepath.Base(opts.InputPath), filepath.Ext(opts.InputPath))
		outPath = filepath.Join(dir, base+"_anonymized"+defaultTargetExt)
	} else {
		outExt := strings.ToLower(filepath.Ext(outPath))
		hasValidExt := false
		for _, v := range validExts {
			if outExt == v {
				hasValidExt = true
				break
			}
		}
		if !hasValidExt {
			// Enforce authentic target extension to guarantee file structure integrity
			outPath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + defaultTargetExt
		}
	}

	if filepath.Clean(opts.InputPath) == filepath.Clean(outPath) {
		return nil, errors.New("output path cannot be the same as input path")
	}

	salt := opts.Salt
	if strings.TrimSpace(salt) == "" {
		generatedSalt, err := anonymizer.GenerateSalt(16)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random salt: %w", err)
		}
		salt = generatedSalt
	}

	keyPath := opts.KeyPath
	if opts.CreateKey && strings.TrimSpace(keyPath) == "" {
		keyPath = outPath + ".depmap"
	}

	if opts.CreateKey && strings.TrimSpace(opts.KeyPassword) != "" {
		if err := depmap.ValidatePassword(opts.KeyPassword); err != nil {
			return nil, err
		}
	}

	archivePath := strings.TrimSpace(opts.ArchivePath)
	if opts.CreateArchive && archivePath == "" {
		archivePath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".zip"
	}

	if opts.CreateArchive && strings.TrimSpace(opts.ArchivePassword) != "" {
		if err := ziputil.ValidateArchivePassword(opts.ArchivePassword); err != nil {
			return nil, err
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Pre-inspect input file for accurate progress estimation and entity metadata
	var entitiesCount int
	var info *domain.FileInfo
	if p.inspector != nil {
		info, _ = p.inspector.Inspect(ctx, opts.InputPath)
		if info != nil {
			seen := make(map[string]bool)
			addEntity := func(t string) {
				if t != "" && !seen[t] {
					seen[t] = true
					entities = append(entities, t)
				}
			}
			for _, e := range info.Entities {
				addEntity(e.Type)
			}
			if len(info.Sheets) > 1 {
				for _, s := range info.Sheets {
					totalRows += s.RowCount
					entitiesCount += len(s.Entities)
					for _, e := range s.Entities {
						addEntity(e.Type)
					}
				}
			} else {
				totalRows = info.RowCount
				entitiesCount = len(info.Entities)
			}
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	progressCallback := func(processed int) {
		if reporter != nil {
			pct := 0
			if totalRows > 0 {
				pct = (processed * 100) / totalRows
				if pct > 99 {
					pct = 99
				}
			}
			reporter(processed, totalRows, pct)
		}
	}

	isTableFormat := ext == ".csv" || ext == ".tsv" || ext == ".xlsx" || ext == ".xlsm" || ext == ".xltx" || ext == ".xltm" || ext == ".ods" || ext == ".xods" || ext == ".xlsb" || ext == ".xls"
	effectiveShuffle := opts.ShuffleRows && isTableFormat

	options := map[string]interface{}{
		"mode":                      opts.Mode,
		"strategy":                  opts.Strategy,
		"columns":                   opts.Columns,
		"disabled_columns":          opts.DisabledCols,
		"disabled_columns_by_sheet": opts.DisabledBySheet,
		"progress_callback":         progressCallback,
		"shuffle_rows":              effectiveShuffle,
		"salt":                      salt,
	}

	if ext == ".docx" || ext == ".odt" || ext == ".pdf" || ext == ".txt" {
		var disabledEntityTypes []string
		if len(opts.DisabledCols) > 0 && info != nil {
			for _, cIdx := range opts.DisabledCols {
				if cIdx >= 1 && cIdx <= len(info.Entities) {
					disabledEntityTypes = append(disabledEntityTypes, info.Entities[cIdx-1].Type)
				}
			}
			options["disabled_entities"] = disabledEntityTypes
		}

		if info != nil && len(info.TextSnippets) > 0 {
			disabledSet := make(map[string]bool)
			for _, d := range disabledEntityTypes {
				disabledSet[d] = true
			}
			var terms []string
			if existing, ok := options["redact_terms"].([]string); ok {
				terms = append(terms, existing...)
			}
			for _, snip := range info.TextSnippets {
				if !disabledSet[snip.Type] && snip.Original != "" {
					terms = append(terms, snip.Original)
				}
			}
			options["redact_terms"] = terms
		}
	}

	if len(opts.PDFRedactionBoxes) > 0 {
		options["redaction_boxes"] = opts.PDFRedactionBoxes
	}

	if len(opts.PDFRedactTerms) > 0 {
		options["redact_terms"] = opts.PDFRedactTerms
	} else if len(opts.PDFRedactionBoxes) > 0 {
		var activeTerms []string
		termSet := make(map[string]bool)
		if cur, ok := options["redact_terms"].([]string); ok {
			for _, t := range cur {
				termSet[t] = true
				activeTerms = append(activeTerms, t)
			}
		}
		for _, b := range opts.PDFRedactionBoxes {
			if b.Text != "" && !termSet[b.Text] {
				activeTerms = append(activeTerms, b.Text)
				termSet[b.Text] = true
			}
		}
		if len(activeTerms) > 0 {
			options["redact_terms"] = activeTerms
		}
	}

	if ext == ".pdf" {
		opts.CreateKey = false
	}

	inFile, err := os.Open(opts.InputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read input file: %w", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	var outFileClosed bool
	closeOutFile := func() {
		if !outFileClosed {
			_ = outFile.Close()
			outFileClosed = true
		}
	}

	cleanup := func() {
		closeOutFile()
		_ = os.Remove(outPath)
		if keyCreated && keyPath != "" {
			_ = os.Remove(keyPath)
		}
		if archiveCreated && archivePath != "" {
			_ = os.Remove(archivePath)
		}
	}

	anon := anonymizer.New(salt)
	startTime := time.Now()
	var directDepMap *depmap.DepMap

	if opts.CreateKey {
		directDepMap = depmap.New(filepath.Base(opts.InputPath))
		if opts.Strategy != "" {
			directDepMap.Mode = string(opts.Strategy)
		} else {
			directDepMap.Mode = string(opts.Mode)
		}
		options["depmap"] = directDepMap
	}

	switch ext {
	case ".csv", ".tsv":
		handler := formats.NewCSVHandler(anon)
		options["write_bom"] = true
		err = handler.Process(ctx, inFile, outFile, options)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		handler := formats.NewExcelHandler(anon)
		err = handler.Process(ctx, inFile, outFile, options)
	case ".ods", ".xods":
		odsHandler := formats.NewODSHandler(anon)
		if strings.EqualFold(filepath.Ext(outPath), ".xlsx") {
			directDepMap, err = odsHandler.ProcessToXLSX(ctx, inFile, stat.Size(), outFile, options)
		} else {
			directDepMap, err = odsHandler.Process(ctx, inFile, stat.Size(), outFile, options)
		}
	case ".xlsb":
		xlsbHandler := formats.NewXLSBHandler(anon)
		directDepMap, err = xlsbHandler.ProcessToXLSX(ctx, opts.InputPath, outFile, options)
	case ".xls":
		xlsHandler := formats.NewXLSHandler(anon)
		directDepMap, err = xlsHandler.ProcessToXLSX(ctx, opts.InputPath, outFile, options)
	case ".docx":
		docxHandler := formats.NewDOCXHandler(anon)
		directDepMap, err = docxHandler.Process(ctx, inFile, stat.Size(), outFile, options)
	case ".odt":
		odtHandler := formats.NewODTHandler(anon)
		directDepMap, err = odtHandler.Process(ctx, inFile, stat.Size(), outFile, options)
	case ".txt":
		txtHandler := formats.NewTXTHandler(anon)
		err = txtHandler.Process(ctx, inFile, outFile, options)
	case ".pdf":
		handler := formats.NewPDFHandler(anon)
		err = handler.Process(ctx, inFile, outFile, options)
	default:
		cleanup()
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, ext)
	}

	closeOutFile()

	if err != nil {
		cleanup()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, fmt.Errorf("anonymization error: %w", err)
	}

	if ctx.Err() != nil {
		cleanup()
		return nil, ctx.Err()
	}

	// Key generation
	var effectivePassword string
	var keyPasswordGenerated bool
	if opts.CreateKey {
		if ctx.Err() != nil {
			cleanup()
			return nil, ctx.Err()
		}
		isAuto := strings.TrimSpace(opts.KeyPassword) == ""
		var savedPwd string
		if directDepMap != nil {
			savedPwd, err = directDepMap.SaveFileWithAutoPassword(keyPath, opts.KeyPassword)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to save key file: %w", err)
			}
		} else {
			dm := depmap.New(filepath.Base(opts.InputPath))
			if err := p.km.ExtractMappings(opts.InputPath, outPath, ext, dm); err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to extract key mappings: %w", err)
			}
			savedPwd, err = dm.SaveFileWithAutoPassword(keyPath, opts.KeyPassword)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to save key file: %w", err)
			}
		}
		effectivePassword = savedPwd
		keyPasswordGenerated = isAuto
		keyCreated = true
	}

	// Archive generation
	var effectiveArchivePassword string
	var archivePasswordGenerated bool
	var finalArchivePath string
	if opts.CreateArchive {
		if ctx.Err() != nil {
			cleanup()
			return nil, ctx.Err()
		}

		pwd := strings.TrimSpace(opts.ArchivePassword)
		if pwd == "" {
			genPwd, err := depmap.GenerateRandomPassword(16)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to generate archive password: %w", err)
			}
			effectiveArchivePassword = genPwd
			archivePasswordGenerated = true
		} else {
			effectiveArchivePassword = pwd
		}

		if err := ziputil.CreatePasswordZip(ctx, outPath, archivePath, effectiveArchivePassword); err != nil {
			cleanup()
			return nil, fmt.Errorf("failed to create password-protected archive: %w", err)
		}
		archiveCreated = true
		finalArchivePath = archivePath
	}

	if ctx.Err() != nil {
		cleanup()
		return nil, ctx.Err()
	}

	if reporter != nil {
		reporter(totalRows, totalRows, 100)
	}

	durationMs := time.Since(startTime).Milliseconds()

	res = &domain.ProcessResult{
		Success:                  true,
		InputPath:                opts.InputPath,
		OutputPath:               outPath,
		RowsCount:                totalRows,
		EntitiesCount:            entitiesCount,
		DurationMs:               durationMs,
		Message:                  fmt.Sprintf("Файл успешно сохранен: %s (%d мс)", filepath.Base(outPath), durationMs),
		EffectivePassword:        effectivePassword,
		KeyPasswordGenerated:     keyPasswordGenerated,
		ArchivePath:              finalArchivePath,
		ArchivePassword:          effectiveArchivePassword,
		ArchivePasswordGenerated: archivePasswordGenerated,
	}
	if opts.CreateKey {
		res.KeyPath = keyPath
	}

	return res, nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d Б", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

