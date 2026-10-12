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

// Process processes the input file according to options and reports progress via reporter.
// It supports streaming anonymization, graceful context cancellation, Windows-safe cleanup,
// and optional .depmap key container generation.
func (p *Processor) Process(ctx context.Context, opts domain.ProcessOptions, reporter domain.ProgressReporter) (res *domain.ProcessResult, err error) {
	var (
		sourceHash string
		resultHash string
		totalRows  int
		entities   []string
		keyCreated bool
		outPath    = opts.OutputPath
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
	targetExt := ext
	if ext == ".xls" || ext == ".xlsb" {
		targetExt = ".xlsx"
	}

	switch ext {
	case ".csv", ".tsv", ".xlsx", ".xlsm", ".xltx", ".xltm", ".ods", ".xods", ".xlsb", ".xls", ".docx", ".odt", ".pdf", ".txt":
		// Supported format
	default:
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, ext)
	}

	if strings.TrimSpace(outPath) == "" {
		dir := filepath.Dir(opts.InputPath)
		base := strings.TrimSuffix(filepath.Base(opts.InputPath), filepath.Ext(opts.InputPath))
		outPath = filepath.Join(dir, base+"_anonymized"+targetExt)
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

	options := map[string]interface{}{
		"mode":                      opts.Mode,
		"strategy":                  opts.Strategy,
		"disabled_columns":          opts.DisabledCols,
		"disabled_columns_by_sheet": opts.DisabledBySheet,
		"progress_callback":         progressCallback,
		"shuffle_rows":              opts.ShuffleRows,
		"salt":                      salt,
	}

	if ext == ".docx" || ext == ".odt" || ext == ".pdf" || ext == ".txt" {
		if len(opts.DisabledCols) > 0 && info != nil {
			var disabledEntityTypes []string
			for _, cIdx := range opts.DisabledCols {
				if cIdx >= 1 && cIdx <= len(info.Entities) {
					disabledEntityTypes = append(disabledEntityTypes, info.Entities[cIdx-1].Type)
				}
			}
			options["disabled_entities"] = disabledEntityTypes
		}
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
		if opts.CreateKey && keyPath != "" {
			_ = os.Remove(keyPath)
		}
	}

	anon := anonymizer.New(salt)
	startTime := time.Now()
	var directDepMap *depmap.DepMap

	if opts.CreateKey {
		directDepMap = depmap.New(filepath.Base(opts.InputPath))
		directDepMap.Mode = string(opts.Mode)
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

	if ctx.Err() != nil {
		cleanup()
		return nil, ctx.Err()
	}

	if reporter != nil {
		reporter(totalRows, totalRows, 100)
	}

	durationMs := time.Since(startTime).Milliseconds()

	res = &domain.ProcessResult{
		Success:              true,
		InputPath:            opts.InputPath,
		OutputPath:           outPath,
		RowsCount:            totalRows,
		EntitiesCount:        entitiesCount,
		DurationMs:           durationMs,
		Message:              fmt.Sprintf("Файл успешно сохранен: %s (%d мс)", filepath.Base(outPath), durationMs),
		EffectivePassword:    effectivePassword,
		KeyPasswordGenerated: keyPasswordGenerated,
	}
	if opts.CreateKey {
		res.KeyPath = keyPath
	}

	return res, nil
}
