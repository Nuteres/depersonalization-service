package main

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

// EntityInfo describes a detected sensitive data column or entity.
type EntityInfo struct {
	ColumnIndex int    `json:"columnIndex"`
	ColumnName  string `json:"columnName"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Action      string `json:"action"`
}

// PreviewRow represents a single row showing values before and after anonymization.
type PreviewRow struct {
	Index  int      `json:"index"`
	Before []string `json:"before"`
	After  []string `json:"after"`
}

// FileInfo holds metadata, detected entities, and preview rows for an inspected file.
type FileInfo struct {
	Path               string       `json:"path"`
	Name               string       `json:"name"`
	Extension          string       `json:"extension"`
	FileType           string       `json:"fileType"` // "table" or "pdf"
	Size               int64        `json:"size"`
	RowCount           int          `json:"rowCount"`
	PageCount          int          `json:"pageCount"`
	Entities           []EntityInfo `json:"entities"`
	Headers            []string     `json:"headers"`
	PreviewRows        []PreviewRow `json:"previewRows"`
	PDFTextSnippet     string       `json:"pdfTextSnippet,omitempty"`
	PDFRedactedSnippet string       `json:"pdfRedactedSnippet,omitempty"`
}

// ProcessResult holds the outcome of a batch anonymization run.
type ProcessResult struct {
	Success       bool   `json:"success"`
	InputPath     string `json:"inputPath"`
	OutputPath    string `json:"outputPath"`
	RowsCount     int    `json:"rowsCount"`
	EntitiesCount int    `json:"entitiesCount"`
	DurationMs    int64  `json:"durationMs"`
	Message       string `json:"message"`
	Error         string `json:"error,omitempty"`
}

// App is the main application struct bound to the Wails runtime.
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// Startup is called at application startup by Wails.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

// GetAppInfo returns system environment metadata (current operating system, architecture, app version).
func (a *App) GetAppInfo() map[string]string {
	return map[string]string{
		"version": "v1.0",
		"os":      detectOS(),
		"arch":    goruntime.GOARCH,
	}
}

func detectOS() string {
	switch goruntime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		if data, err := os.ReadFile("/etc/altlinux-release"); err == nil && len(data) > 0 {
			return "Alt Linux"
		}
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			s := string(data)
			if strings.Contains(s, "ALT") || strings.Contains(s, "altlinux") {
				return "Alt Linux"
			}
			for _, line := range strings.Split(s, "\n") {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					val := strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
					if val != "" {
						return val
					}
				}
			}
		}
		return "Linux"
	default:
		return goruntime.GOOS
	}
}

// SaveFileDialog opens a native system file save dialog with defaultFilename
// and returns the user-selected destination path.
func (a *App) SaveFileDialog(defaultName string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	ext := strings.ToLower(filepath.Ext(defaultName))
	var filters []runtime.FileFilter
	switch ext {
	case ".xlsx", ".xlsm":
		filters = []runtime.FileFilter{
			{DisplayName: "Таблица Excel (*.xlsx)", Pattern: "*.xlsx"},
			{DisplayName: "Все файлы (*.*)", Pattern: "*.*"},
		}
	case ".pdf":
		filters = []runtime.FileFilter{
			{DisplayName: "Документ PDF (*.pdf)", Pattern: "*.pdf"},
			{DisplayName: "Все файлы (*.*)", Pattern: "*.*"},
		}
	default:
		filters = []runtime.FileFilter{
			{DisplayName: "Таблица CSV (*.csv)", Pattern: "*.csv"},
			{DisplayName: "Все файлы (*.*)", Pattern: "*.*"},
		}
	}

	selected, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Сохранить обработанный файл",
		DefaultFilename: defaultName,
		Filters:         filters,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open save dialog: %w", err)
	}
	return selected, nil
}

// OpenContainingFolder opens the folder containing targetPath in the OS file manager.
func (a *App) OpenContainingFolder(targetPath string) error {
	if strings.TrimSpace(targetPath) == "" {
		return errors.New("path cannot be empty")
	}

	dir := filepath.Dir(targetPath)
	switch goruntime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", targetPath).Start()
	case "windows":
		return exec.Command("explorer.exe", "/select,", targetPath).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}

// SelectFile opens a native system file selection dialog and returns the selected path.
func (a *App) SelectFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application context not initialized")
	}

	selected, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Выберите файл для обезличивания",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Поддерживаемые документы (*.csv, *.tsv, *.xlsx, *.pdf)",
				Pattern:     "*.csv;*.tsv;*.xlsx;*.xlsm;*.pdf",
			},
			{
				DisplayName: "Таблицы Excel и CSV (*.csv, *.tsv, *.xlsx, *.xlsm)",
				Pattern:     "*.csv;*.tsv;*.xlsx;*.xlsm",
			},
			{
				DisplayName: "Документы PDF (*.pdf)",
				Pattern:     "*.pdf",
			},
			{
				DisplayName: "Все файлы (*.*)",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	return selected, nil
}

// InspectFile inspects CSV, XLSX, or PDF files:
// - Determines extension and row/page count.
// - Detects PII columns or entities using pkg/detector and pkg/formats.
// - Generates preview rows (first 3-5 rows: headers, original before values, anonymized after values).
// - Returns FileInfo.
func (a *App) InspectFile(path string) (*FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("file path cannot be empty")
	}

	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv", ".tsv":
		return inspectCSV(path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return inspectXLSX(path)
	case ".pdf":
		return inspectPDF(path)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", ext)
	}
}

// ProcessFile processes the input file using the appropriate handler from pkg/formats,
// writing to <dir>/<basename>_anonymized.<ext>, calculating execution time and stats.
func (a *App) ProcessFile(path string, salt string) (*ProcessResult, error) {
	return a.ProcessFileTo(path, "", salt)
}

// ProcessFileTo processes the input file and writes to outPath (or default if outPath is empty),
// emitting progress events via Wails runtime.
func (a *App) ProcessFileTo(path string, outPath string, salt string) (*ProcessResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("file path cannot be empty")
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %w", path, err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file: %s", path)
	}

	if strings.TrimSpace(salt) == "" {
		salt = "152fz-secure-salt"
	}

	ext := strings.ToLower(filepath.Ext(path))
	if strings.TrimSpace(outPath) == "" {
		dir := filepath.Dir(path)
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		outPath = filepath.Join(dir, base+"_anonymized"+ext)
	}

	anon := anonymizer.New(salt)

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	inFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read input file: %w", err)
	}
	defer inFile.Close()

	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	var handler formats.FormatHandler
	switch ext {
	case ".csv", ".tsv":
		handler = formats.NewCSVHandler(anon)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		handler = formats.NewExcelHandler(anon)
	case ".pdf":
		handler = formats.NewPDFHandler(anon)
	default:
		outFile.Close()
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}

	// Determine total row count for progress percentage
	var totalRows int
	var entitiesCount int
	info, inspectErr := a.InspectFile(path)
	if inspectErr == nil && info != nil {
		totalRows = info.RowCount
		entitiesCount = len(info.Entities)
	}

	options := map[string]interface{}{
		"progress_callback": func(processed int) {
			if a.ctx != nil {
				pct := 0
				if totalRows > 0 {
					pct = (processed * 100) / totalRows
					if pct > 99 {
						pct = 99
					}
				}
				runtime.EventsEmit(a.ctx, "process-progress", map[string]interface{}{
					"processed": processed,
					"total":     totalRows,
					"percent":   pct,
				})
			}
		},
	}

	startTime := time.Now()
	err = handler.Process(ctx, inFile, outFile, options)
	outFile.Close()

	if err != nil {
		_ = os.Remove(outPath)
		return nil, fmt.Errorf("anonymization error: %w", err)
	}

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "process-progress", map[string]interface{}{
			"processed": totalRows,
			"total":     totalRows,
			"percent":   100,
		})
	}

	durationMs := time.Since(startTime).Milliseconds()

	return &ProcessResult{
		Success:       true,
		InputPath:     path,
		OutputPath:    outPath,
		RowsCount:     totalRows,
		EntitiesCount: entitiesCount,
		DurationMs:    durationMs,
		Message:       fmt.Sprintf("Файл успешно сохранен: %s (%d мс)", filepath.Base(outPath), durationMs),
	}, nil
}

// columnTypeDetails returns user-friendly metadata for each PII ColumnType.
func columnTypeDetails(colType formats.ColumnType) (typeName, desc, action string) {
	switch colType {
	case formats.ColFIO:
		return "FIO", "Русские фамилии, имена и отчества", "Синтетический профиль"
	case formats.ColSNILS:
		return "SNILS", "Страховые номера РФ", "Валидный модуль 101"
	case formats.ColINN:
		return "INN", "Идентификационный номер налогоплательщика", "Контрольная сумма ФНС"
	case formats.ColPassport:
		return "PASSPORT", "Серия и номер гражданина РФ", "Случайная серия/номер"
	case formats.ColPhone:
		return "PHONE", "Мобильные операторы РФ", "Псевдонимный номер"
	case formats.ColCard:
		return "BANK_CARD", "Банковские карты", "Карта МИР (Luhn)"
	case formats.ColDate:
		return "DATE", "Даты и периоды", "Смещение ±30 дней"
	case formats.ColEmail:
		return "EMAIL", "Корпоративная и личная почта", "Псевдонимный адрес"
	default:
		return "UNKNOWN", "Обычное поле", "Без изменений"
	}
}

func inspectCSV(path string) (*FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	bufReader := bufio.NewReader(file)

	// Auto-detect delimiter
	comma := ','
	peekBytes, _ := bufReader.Peek(2048)
	peekStr := strings.TrimPrefix(string(peekBytes), "\ufeff")
	firstLine := peekStr
	if idx := strings.IndexAny(peekStr, "\r\n"); idx != -1 {
		firstLine = peekStr[:idx]
	}
	semiCount := strings.Count(firstLine, ";")
	tabCount := strings.Count(firstLine, "\t")
	commaCount := strings.Count(firstLine, ",")
	if semiCount > commaCount && semiCount >= tabCount {
		comma = ';'
	} else if tabCount > commaCount && tabCount > semiCount {
		comma = '\t'
	}

	csvReader := csv.NewReader(bufReader)
	csvReader.Comma = comma
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	var sample [][]string
	for len(sample) < 40 {
		rec, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		sample = append(sample, rec)
	}

	if len(sample) == 0 {
		return nil, errors.New("csv file is empty")
	}

	headerIdx, headers := formats.FindHeaderRow(sample)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	colTypes := formats.ResolveColumnTypes(headers, nil)
	var entities []EntityInfo
	for i, ct := range colTypes {
		if ct != formats.ColUnknown {
			typeName, desc, action := columnTypeDetails(ct)
			colName := ""
			if i < len(headers) {
				colName = headers[i]
			}
			entities = append(entities, EntityInfo{
				ColumnIndex: i + 1,
				ColumnName:  colName,
				Type:        typeName,
				Description: desc,
				Action:      action,
			})
		}
	}

	anon := anonymizer.New("preview-salt-152fz")
	var previewRows []PreviewRow
	rowCount := 0

	for i := headerIdx + 1; i < len(sample); i++ {
		rowCount++
		if len(previewRows) < 5 {
			cols := sample[i]
			before := make([]string, len(headers))
			after := make([]string, len(headers))
			for j := 0; j < len(headers); j++ {
				val := ""
				if j < len(cols) {
					val = cols[j]
				}
				before[j] = val
				ct := formats.ColUnknown
				if j < len(colTypes) {
					ct = colTypes[j]
				}
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
			}
			previewRows = append(previewRows, PreviewRow{
				Index:  rowCount,
				Before: before,
				After:  after,
			})
		}
	}

	for {
		record, err := csvReader.Read()
		if err != nil {
			break
		}
		rowCount++
		if len(previewRows) < 5 {
			before := make([]string, len(headers))
			after := make([]string, len(headers))
			for j := 0; j < len(headers); j++ {
				val := ""
				if j < len(record) {
					val = record[j]
				}
				before[j] = val
				ct := formats.ColUnknown
				if j < len(colTypes) {
					ct = colTypes[j]
				}
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
			}
			previewRows = append(previewRows, PreviewRow{
				Index:  rowCount,
				Before: before,
				After:  after,
			})
		}
	}

	return &FileInfo{
		Path:        path,
		Name:        filepath.Base(path),
		Extension:   strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		FileType:    "table",
		Size:        stat.Size(),
		RowCount:    rowCount,
		PageCount:   1,
		Entities:    entities,
		Headers:     headers,
		PreviewRows: previewRows,
	}, nil
}

func inspectXLSX(path string) (*FileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	xl, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsx file: %w", err)
	}
	defer xl.Close()

	sheets := xl.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("xlsx file has no sheets")
	}

	sheetName := sheets[0]

	// Sample up to 40 rows to find table header row (skipping preamble metadata if present)
	sampleIter, err := xl.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
	}
	var sample [][]string
	for sampleIter.Next() && len(sample) < 40 {
		cols, err := sampleIter.Columns()
		if err != nil {
			break
		}
		sample = append(sample, cols)
	}
	_ = sampleIter.Close()

	if len(sample) == 0 {
		return nil, errors.New("xlsx sheet is empty")
	}

	headerIdx, headers := formats.FindHeaderRow(sample)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	colTypes := formats.ResolveColumnTypes(headers, nil)
	var entities []EntityInfo
	for i, ct := range colTypes {
		if ct != formats.ColUnknown {
			typeName, desc, action := columnTypeDetails(ct)
			colName := ""
			if i < len(headers) {
				colName = headers[i]
			}
			entities = append(entities, EntityInfo{
				ColumnIndex: i + 1,
				ColumnName:  colName,
				Type:        typeName,
				Description: desc,
				Action:      action,
			})
		}
	}

	anon := anonymizer.New("preview-salt-152fz")
	var previewRows []PreviewRow
	rowCount := 0

	// Preview rows and row count from sample
	for i := headerIdx + 1; i < len(sample); i++ {
		rowCount++
		if len(previewRows) < 5 {
			cols := sample[i]
			before := make([]string, len(headers))
			after := make([]string, len(headers))
			for j := 0; j < len(headers); j++ {
				val := ""
				if j < len(cols) {
					val = cols[j]
				}
				before[j] = val
				ct := formats.ColUnknown
				if j < len(colTypes) {
					ct = colTypes[j]
				}
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
			}
			previewRows = append(previewRows, PreviewRow{
				Index:  rowCount,
				Before: before,
				After:  after,
			})
		}
	}

	// Count remaining rows in sheet
	remainingIter, err := xl.Rows(sheetName)
	if err == nil {
		skipped := 0
		for remainingIter.Next() {
			skipped++
			if skipped > len(sample) {
				rowCount++
				if len(previewRows) < 5 {
					cols, _ := remainingIter.Columns()
					before := make([]string, len(headers))
					after := make([]string, len(headers))
					for j := 0; j < len(headers); j++ {
						val := ""
						if j < len(cols) {
							val = cols[j]
						}
						before[j] = val
						ct := formats.ColUnknown
						if j < len(colTypes) {
							ct = colTypes[j]
						}
						after[j] = formats.AnonymizeCell(anon, ct, val, 30)
					}
					previewRows = append(previewRows, PreviewRow{
						Index:  rowCount,
						Before: before,
						After:  after,
					})
				}
			}
		}
		_ = remainingIter.Close()
	}

	return &FileInfo{
		Path:        path,
		Name:        filepath.Base(path),
		Extension:   strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		FileType:    "table",
		Size:        stat.Size(),
		RowCount:    rowCount,
		PageCount:   len(sheets),
		Entities:    entities,
		Headers:     headers,
		PreviewRows: previewRows,
	}, nil
}

// Regex patterns for inspecting sensitive data in PDF streams
var (
	reSNILSFormatted   = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2})`)
	reSNILS11Digits    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{11})`)
	rePassport4_6      = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{4}\s+\d{6})`)
	rePassport2_2_6    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{2}\s+\d{2}\s+\d{6})`)
	rePassportPrefixed = regexp.MustCompile(`(?i)(?:паспорт(?:а| гражданина рф)?|серия|номер|№)[:\s]*([0-9\s]{10,13})`)
	reINN12            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(\d{12})`)
	reINNPrefixed      = regexp.MustCompile(`(?i)(?:ИНН|INN)[:\s]*\b(\d{10,12})\b`)
	rePhone            = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\+7|8)[\s\-\(]*\d{3}[\s\-\)]*\d{3}[\s\-]*\d{2}[\s\-]*\d{2})`)
	rePhonePrefixed    = regexp.MustCompile(`(?i)(?:тел(?:ефон)?|моб\.?|сот\.?)[:\s]*((?:\+7|8)?[0-9\s\-\(\)]{10,18})`)
	reCard16           = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:\d{4}[\s\-]?){4})`)
	reFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична))`)
	reFIOPrefixed      = regexp.MustCompile(`(?i)(?:сотрудник|клиент|пациент|фио|работник|физлицо)[:\s]*([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	rePage             = regexp.MustCompile(`/Type\s*/Page\b`)
	reLen              = regexp.MustCompile(`/Length\s+(\d+)`)
	reLit              = regexp.MustCompile(`\(([^)\\]*(?:\\.[^)\\]*)*)\)`)
)

func inspectPDF(path string) (*FileInfo, error) {
	pdfBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pdf: %w", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return nil, errors.New("invalid PDF file: missing %PDF- header")
	}

	pageMatches := rePage.FindAll(pdfBytes, -1)
	pageCount := len(pageMatches)
	if pageCount == 0 {
		pageCount = 1
	}

	extractedText := extractTextFromPDFBytes(pdfBytes)
	entities, snippetBefore, snippetAfter := detectPDFEntitiesAndSnippets(extractedText)

	return &FileInfo{
		Path:               path,
		Name:               filepath.Base(path),
		Extension:          "pdf",
		FileType:           "pdf",
		Size:               int64(len(pdfBytes)),
		RowCount:           pageCount,
		PageCount:          pageCount,
		Entities:           entities,
		Headers:            []string{"Сущность", "Категория", "Режим защиты"},
		PDFTextSnippet:     snippetBefore,
		PDFRedactedSnippet: snippetAfter,
	}, nil
}

func extractTextFromPDFBytes(pdfBytes []byte) string {
	var sb strings.Builder
	idx := 0

	for {
		sPos := bytes.Index(pdfBytes[idx:], []byte("stream"))
		if sPos == -1 {
			break
		}
		streamStart := idx + sPos + len("stream")
		if streamStart < len(pdfBytes) && pdfBytes[streamStart] == '\r' {
			streamStart++
		}
		if streamStart < len(pdfBytes) && pdfBytes[streamStart] == '\n' {
			streamStart++
		}

		ePos := bytes.Index(pdfBytes[streamStart:], []byte("endstream"))
		if ePos == -1 {
			break
		}
		streamEnd := streamStart + ePos

		// Check dict header for /FlateDecode and /Length
		dictStart := idx + sPos - 250
		if dictStart < 0 {
			dictStart = 0
		}
		dictHeader := pdfBytes[dictStart : idx+sPos]

		var streamData []byte
		if lenMatch := reLen.FindSubmatch(dictHeader); len(lenMatch) > 1 {
			if l, err := strconv.Atoi(string(lenMatch[1])); err == nil && l >= 0 && streamStart+l <= len(pdfBytes) {
				streamData = pdfBytes[streamStart : streamStart+l]
			}
		}
		if streamData == nil {
			rawEnd := streamEnd
			if rawEnd > streamStart && pdfBytes[rawEnd-1] == '\n' {
				rawEnd--
				if rawEnd > streamStart && pdfBytes[rawEnd-1] == '\r' {
					rawEnd--
				}
			} else if rawEnd > streamStart && pdfBytes[rawEnd-1] == '\r' {
				rawEnd--
			}
			streamData = pdfBytes[streamStart:rawEnd]
		}

		if bytes.Contains(dictHeader, []byte("/FlateDecode")) {
			zr, err := zlib.NewReader(bytes.NewReader(streamData))
			if err == nil {
				decomp, readErr := io.ReadAll(zr)
				_ = zr.Close()
				if readErr == nil {
					streamData = decomp
				}
			}
		}

		// Extract literal string contents ( ... )
		litMatches := reLit.FindAllSubmatch(streamData, -1)
		for _, m := range litMatches {
			if len(m) > 1 {
				s := string(m[1])
				s = strings.ReplaceAll(s, `\n`, "\n")
				s = strings.ReplaceAll(s, `\r`, "")
				s = strings.ReplaceAll(s, `\t`, " ")
				s = strings.ReplaceAll(s, `\(`, "(")
				s = strings.ReplaceAll(s, `\)`, ")")
				s = strings.ReplaceAll(s, `\\`, "\\")
				sb.WriteString(s)
				sb.WriteString(" ")
			}
		}

		idx = streamEnd + len("endstream")
	}

	res := strings.TrimSpace(sb.String())
	if res == "" {
		res = string(pdfBytes)
	}
	return res
}

func detectPDFEntitiesAndSnippets(text string) ([]EntityInfo, string, string) {
	hasFIO := reFIOPatronymic.MatchString(text) || reFIOPrefixed.MatchString(text)
	hasSNILS := false
	if reSNILSFormatted.MatchString(text) {
		hasSNILS = true
	} else {
		for _, m := range reSNILS11Digits.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 && detector.ValidateSNILS(m[1]) {
				hasSNILS = true
				break
			}
		}
	}

	hasPassport := rePassport4_6.MatchString(text) || rePassport2_2_6.MatchString(text) || rePassportPrefixed.MatchString(text)
	hasPhone := rePhone.MatchString(text) || rePhonePrefixed.MatchString(text)
	hasINN := false
	if reINNPrefixed.MatchString(text) {
		hasINN = true
	} else {
		for _, m := range reINN12.FindAllStringSubmatch(text, -1) {
			if len(m) > 1 && detector.ValidateINN12(m[1]) {
				hasINN = true
				break
			}
		}
	}
	hasCard := reCard16.MatchString(text)

	var entities []EntityInfo
	cardIdx := 1
	if hasFIO {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "ФИО (PDF)",
			Type:        "FIO",
			Description: "Русские фамилии, имена и отчества",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasSNILS {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "СНИЛС (PDF)",
			Type:        "SNILS",
			Description: "Страховые номера РФ (модуль 101)",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasPassport {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "Паспорт РФ (PDF)",
			Type:        "PASSPORT",
			Description: "Серия и номер гражданина РФ",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasPhone {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "Телефон (PDF)",
			Type:        "PHONE",
			Description: "Мобильные номера РФ",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasINN {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "ИНН (PDF)",
			Type:        "INN",
			Description: "ИНН физического лица (алгоритм ФНС)",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasCard {
		entities = append(entities, EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "Банковская карта (PDF)",
			Type:        "BANK_CARD",
			Description: "Номера банковских карт",
			Action:      "Черная векторная плашка",
		})
	}

	snippetBefore := text
	if len([]rune(snippetBefore)) > 600 {
		snippetBefore = string([]rune(snippetBefore)[:600]) + "..."
	}

	snippetAfter := snippetBefore
	redactWithRe := func(re *regexp.Regexp) {
		snippetAfter = re.ReplaceAllStringFunc(snippetAfter, func(s string) string {
			matches := re.FindStringSubmatch(s)
			if len(matches) > 1 && len(matches[1]) > 0 {
				return strings.Replace(s, matches[1], formats.BlackoutMarker, 1)
			}
			return formats.BlackoutMarker
		})
	}

	redactWithRe(reSNILSFormatted)
	redactWithRe(rePassport4_6)
	redactWithRe(rePassport2_2_6)
	redactWithRe(rePassportPrefixed)
	redactWithRe(rePhone)
	redactWithRe(rePhonePrefixed)
	redactWithRe(reFIOPatronymic)
	redactWithRe(reFIOPrefixed)
	redactWithRe(reINNPrefixed)
	redactWithRe(reCard16)

	return entities, snippetBefore, snippetAfter
}
