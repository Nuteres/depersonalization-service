package inspector

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

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
	reFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:овича|евича|ича|овны|евны|ичны|овичу|евичу|ичу|овне|евне|ичне|овичем|евичем|ичем|овной|евной|ичной|овиче|евиче|иче|ович|евич|ич|овна|евна|ична|оглы|кызы|уулу|инична))`)
	reFIOPrefixed      = regexp.MustCompile(`(?i:(?:сотрудник|клиент|пациент|фио|работник|физлицо|подписал|сдал|принял|директор|гендиректор|руководитель|представитель)[:\s]+)([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	reFIOInitials1     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s+[А-ЯЁ]\.))`)
	reFIOInitials2     = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])((?:[А-ЯЁ]\.\s*[А-ЯЁ]\.|\s*[А-ЯЁ]\.)\s+[А-ЯЁ][а-яё]+)`)
)

const defaultPreviewSalt = "preview-salt-152fz"

// Inspector provides document analysis and inspection across supported formats.
type Inspector struct{}

// New creates a new Inspector instance.
func New() *Inspector {
	return &Inspector{}
}

// Inspect inspects CSV, XLSX, ODS, XLSB, XLS, DOCX, ODT, or PDF files:
// - Determines extension and row/page count.
// - Detects PII columns or entities using pkg/detector and pkg/formats.
// - Generates preview rows (first rows: headers, original before values, anonymized after values).
// - Returns domain.FileInfo.
func (i *Inspector) Inspect(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if strings.TrimSpace(path) == "" {
		return nil, errors.New("file path cannot be empty")
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("%w: path is a directory", domain.ErrUnsupportedFormat)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv", ".tsv":
		return inspectCSV(ctx, path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return inspectXLSX(ctx, path)
	case ".ods", ".xods":
		return inspectODS(ctx, path)
	case ".xlsb":
		return inspectXLSB(ctx, path)
	case ".xls":
		return inspectXLS(ctx, path)
	case ".docx":
		return inspectDOCX(ctx, path)
	case ".odt":
		return inspectODT(ctx, path)
	case ".txt", ".text":
		return inspectTXT(ctx, path)
	case ".pdf":
		return inspectPDF(ctx, path)
	default:
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, ext)
	}
}

// columnTypeDetails returns user-friendly metadata for each PII ColumnType.
func columnTypeDetails(colType formats.ColumnType) (typeName, desc, action string) {
	switch colType {
	case formats.ColFIO:
		return "FIO", "Фамилии, имена и отчества", "Обезличивание ФИО"
	case formats.ColLastName:
		return "LAST_NAME", "Фамилии", "Обезличивание фамилий"
	case formats.ColFirstName:
		return "FIRST_NAME", "Имена", "Обезличивание имён"
	case formats.ColPatronymic:
		return "PATRONYMIC", "Отчества", "Обезличивание отчеств"
	case formats.ColSNILS:
		return "SNILS", "Страховые номера РФ", "Обезличивание СНИЛС"
	case formats.ColINN:
		return "INN", "Идентификационный номер налогоплательщика", "Обезличивание ИНН"
	case formats.ColPassport:
		return "PASSPORT", "Серия и номер гражданина РФ", "Обезличивание паспортов"
	case formats.ColPhone:
		return "PHONE", "Мобильные операторы РФ", "Обезличивание телефонов"
	case formats.ColCard:
		return "BANK_CARD", "Банковские карты", "Обезличивание карт"
	case formats.ColDate:
		return "DATE", "Даты и периоды", "Обезличивание дат"
	case formats.ColEmail:
		return "EMAIL", "Корпоративная и личная почта", "Обезличивание email"
	case formats.ColAddress:
		return "ADDRESS", "Адреса проживания и регистрации", "Обезличивание адресов"
	case formats.ColGrade:
		return "GRADE", "Грейды и квалификационные разряды", "Обезличивание грейдов"
	case formats.ColEmployeeID:
		return "EMPLOYEE_ID", "Табельные номера сотрудников", "Обезличивание табельных номеров"
	default:
		return "UNKNOWN", "Обычное поле", "Без изменений"
	}
}

func inspectCSV(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	csvReader, _, err := formats.NewEncodingAwareCSVReader(file, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv reader: %w", err)
	}

	var sample [][]string
	for len(sample) < 40 {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
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

	colTypes := formats.ResolveColumnTypesWithSamples(headers, sample[headerIdx+1:], nil)
	var entities []domain.EntityInfo
	for i, ct := range colTypes {
		if ct != formats.ColUnknown {
			typeName, desc, action := columnTypeDetails(ct)
			colName := ""
			if i < len(headers) {
				colName = headers[i]
			}
			entities = append(entities, domain.EntityInfo{
				ColumnIndex: i + 1,
				ColumnName:  colName,
				Type:        typeName,
				Description: desc,
				Action:      action,
			})
		}
	}

	anon := anonymizer.New(defaultPreviewSalt)
	ts := anonymizer.NewTokenSession(defaultPreviewSalt)
	var previewRows []domain.PreviewRow
	rowCount := 0

	for i := headerIdx + 1; i < len(sample); i++ {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rowCount++
		if len(previewRows) < 10 {
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
				after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, domain.StrategySemanticTokens, 30)
			}
			previewRows = append(previewRows, domain.PreviewRow{
				Index:  rowCount,
				Before: before,
				After:  after,
			})
		}
	}

	for {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		record, err := csvReader.Read()
		if err != nil {
			break
		}
		rowCount++
		if len(previewRows) < 10 {
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
				after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, domain.StrategySemanticTokens, 30)
			}
			previewRows = append(previewRows, domain.PreviewRow{
				Index:  rowCount,
				Before: before,
				After:  after,
			})
		}
	}

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		FileType:               "table",
		Format:                 "csv",
		SupportedExportFormats: []string{"csv", "xlsx"},
		Size:                   stat.Size(),
		RowCount:               rowCount,
		PageCount:              1,
		Entities:               entities,
		Headers:                headers,
		PreviewRows:            previewRows,
	}, nil
}

func inspectSingleSheet(ctx context.Context, xl *excelize.File, sheetName string, sheetIndex int, anon *anonymizer.Anonymizer) (*domain.SheetInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Sample up to 40 rows to find table header row (skipping preamble metadata if present)
	sampleIter, err := xl.Rows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
	}
	var sample [][]string
	for sampleIter.Next() && len(sample) < 40 {
		if ctx != nil && ctx.Err() != nil {
			_ = sampleIter.Close()
			return nil, ctx.Err()
		}
		cols, err := sampleIter.Columns()
		if err != nil {
			break
		}
		sample = append(sample, cols)
	}
	_ = sampleIter.Close()

	if len(sample) == 0 {
		return &domain.SheetInfo{
			Name:        sheetName,
			Index:       sheetIndex,
			RowCount:    0,
			Headers:     nil,
			Entities:    nil,
			PreviewRows: nil,
		}, nil
	}

	headerIdx, headers := formats.FindHeaderRow(sample)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	colTypes := formats.ResolveColumnTypesWithSamples(headers, sample[headerIdx+1:], nil)
	var entities []domain.EntityInfo
	for i, ct := range colTypes {
		if ct != formats.ColUnknown {
			typeName, desc, action := columnTypeDetails(ct)
			colName := ""
			if i < len(headers) {
				colName = headers[i]
			}
			entities = append(entities, domain.EntityInfo{
				ColumnIndex: i + 1,
				ColumnName:  colName,
				Type:        typeName,
				Description: desc,
				Action:      action,
			})
		}
	}

	ts := anonymizer.NewTokenSession(defaultPreviewSalt)
	var previewRows []domain.PreviewRow
	rowCount := 0

	// Preview rows and row count from sample
	for i := headerIdx + 1; i < len(sample); i++ {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		rowCount++
		if len(previewRows) < 10 {
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
				after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, domain.StrategySemanticTokens, 30)
			}
			previewRows = append(previewRows, domain.PreviewRow{
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
			if ctx != nil && ctx.Err() != nil {
				_ = remainingIter.Close()
				return nil, ctx.Err()
			}
			skipped++
			if skipped > len(sample) {
				rowCount++
				if len(previewRows) < 10 {
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
						after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, domain.StrategySemanticTokens, 30)
					}
					previewRows = append(previewRows, domain.PreviewRow{
						Index:  rowCount,
						Before: before,
						After:  after,
					})
				}
			}
		}
		_ = remainingIter.Close()
	}

	return &domain.SheetInfo{
		Name:        sheetName,
		Index:       sheetIndex,
		RowCount:    rowCount,
		Headers:     headers,
		Entities:    entities,
		PreviewRows: previewRows,
	}, nil
}

func inspectXLSX(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

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

	anon := anonymizer.New(defaultPreviewSalt)
	var allSheets []domain.SheetInfo
	for idx, sName := range sheets {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sInfo, err := inspectSingleSheet(ctx, xl, sName, idx, anon)
		if err != nil {
			if ctx != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if sInfo != nil {
			allSheets = append(allSheets, *sInfo)
		}
	}

	if len(allSheets) == 0 {
		return nil, errors.New("xlsx file has no readable sheets")
	}

	primarySheet := allSheets[0]

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
		FileType:               "table",
		Format:                 "xlsx",
		SupportedExportFormats: []string{"xlsx", "csv"},
		Size:                   stat.Size(),
		RowCount:               primarySheet.RowCount,
		PageCount:              len(sheets),
		Entities:               primarySheet.Entities,
		Headers:                primarySheet.Headers,
		PreviewRows:            primarySheet.PreviewRows,
		Sheets:                 allSheets,
		ActiveSheet:            primarySheet.Name,
	}, nil
}

func buildSheetInfoFromRows(sheetName string, sheetIndex int, rowCount int, sampleRows [][]string, anon *anonymizer.Anonymizer) domain.SheetInfo {
	if len(sampleRows) == 0 {
		return domain.SheetInfo{
			Name:        sheetName,
			Index:       sheetIndex,
			RowCount:    rowCount,
			Headers:     nil,
			Entities:    nil,
			PreviewRows: nil,
		}
	}

	headerIdx, headers := formats.FindHeaderRow(sampleRows)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	colTypes := formats.ResolveColumnTypesWithSamples(headers, sampleRows[headerIdx+1:], nil)
	var entities []domain.EntityInfo
	for i, ct := range colTypes {
		if ct != formats.ColUnknown {
			typeName, desc, action := columnTypeDetails(ct)
			colName := ""
			if i < len(headers) {
				colName = headers[i]
			}
			entities = append(entities, domain.EntityInfo{
				ColumnIndex: i + 1,
				ColumnName:  colName,
				Type:        typeName,
				Description: desc,
				Action:      action,
			})
		}
	}

	ts := anonymizer.NewTokenSession(defaultPreviewSalt)
	var previewRows []domain.PreviewRow
	limit := 10
	currRow := 0
	for i := headerIdx + 1; i < len(sampleRows); i++ {
		currRow++
		if len(previewRows) < limit {
			cols := sampleRows[i]
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
				after[j] = formats.ProcessCellWithStrategy(anon, ts, ct, val, domain.StrategySemanticTokens, 30)
			}
			previewRows = append(previewRows, domain.PreviewRow{
				Index:  currRow,
				Before: before,
				After:  after,
			})
		}
	}

	return domain.SheetInfo{
		Name:        sheetName,
		Index:       sheetIndex,
		RowCount:    rowCount,
		Headers:     headers,
		Entities:    entities,
		PreviewRows: previewRows,
	}
}

func inspectODS(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ods file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat ods file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewODSHandler(anon)
	sheets, samples, err := handler.InspectRows(file, stat.Size(), 40)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect ods file: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("ods file has no sheets")
	}

	var allSheets []domain.SheetInfo
	for _, s := range sheets {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sampleRows := samples[s.Name]
		sInfo := buildSheetInfoFromRows(s.Name, s.Index, s.RowCount, sampleRows, anon)
		allSheets = append(allSheets, sInfo)
	}

	primarySheet := allSheets[0]
	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "ods",
		FileType:               "table",
		Format:                 "ods",
		SupportedExportFormats: []string{"ods", "xlsx", "csv"},
		Size:                   stat.Size(),
		RowCount:               primarySheet.RowCount,
		PageCount:              len(allSheets),
		Entities:               primarySheet.Entities,
		Headers:                primarySheet.Headers,
		PreviewRows:            primarySheet.PreviewRows,
		Sheets:                 allSheets,
		ActiveSheet:            primarySheet.Name,
	}, nil
}

func inspectXLSB(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat xlsb file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewXLSBHandler(anon)
	sheets, samples, err := handler.InspectRows(path, 40)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect xlsb file: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("xlsb file has no sheets")
	}

	var allSheets []domain.SheetInfo
	for _, s := range sheets {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sampleRows := samples[s.Name]
		sInfo := buildSheetInfoFromRows(s.Name, s.Index, s.RowCount, sampleRows, anon)
		allSheets = append(allSheets, sInfo)
	}

	primarySheet := allSheets[0]
	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "xlsb",
		FileType:               "table",
		Format:                 "xlsb",
		SupportedExportFormats: []string{"xlsx"},
		Size:                   stat.Size(),
		RowCount:               primarySheet.RowCount,
		PageCount:              len(allSheets),
		Entities:               primarySheet.Entities,
		Headers:                primarySheet.Headers,
		PreviewRows:            primarySheet.PreviewRows,
		Sheets:                 allSheets,
		ActiveSheet:            primarySheet.Name,
	}, nil
}

func inspectXLS(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat xls file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewXLSHandler(anon)
	sheets, samples, err := handler.InspectRows(path, 40)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect xls file: %w", err)
	}
	if len(sheets) == 0 {
		return nil, errors.New("xls file has no sheets")
	}

	var allSheets []domain.SheetInfo
	for _, s := range sheets {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		sampleRows := samples[s.Name]
		sInfo := buildSheetInfoFromRows(s.Name, s.Index, s.RowCount, sampleRows, anon)
		allSheets = append(allSheets, sInfo)
	}

	primarySheet := allSheets[0]
	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "xls",
		FileType:               "table",
		Format:                 "xls",
		SupportedExportFormats: []string{"xlsx"},
		Size:                   stat.Size(),
		RowCount:               primarySheet.RowCount,
		PageCount:              len(allSheets),
		Entities:               primarySheet.Entities,
		Headers:                primarySheet.Headers,
		PreviewRows:            primarySheet.PreviewRows,
		Sheets:                 allSheets,
		ActiveSheet:            primarySheet.Name,
	}, nil
}

func textEntityTypeName(t string) string {
	switch t {
	case "FIO":
		return "ФИО"
	case "LAST_NAME":
		return "Фамилия"
	case "FIRST_NAME":
		return "Имя"
	case "PATRONYMIC":
		return "Отчество"
	case "SNILS":
		return "СНИЛС"
	case "PASSPORT":
		return "Паспорт РФ"
	case "PHONE":
		return "Телефон"
	case "INN":
		return "ИНН"
	case "BANK_CARD":
		return "Банковская карта"
	case "DATE":
		return "Дата"
	case "EMAIL":
		return "Email"
	case "ADDRESS":
		return "Адрес"
	case "GRADE":
		return "Грейд"
	case "EMPLOYEE_ID":
		return "Табельный номер"
	default:
		return t
	}
}

func textEntityDetails(t string) (desc, action string) {
	switch t {
	case "FIO":
		return "Фамилии, имена и отчества", "Обезличивание ФИО"
	case "LAST_NAME":
		return "Фамилии", "Обезличивание фамилий"
	case "FIRST_NAME":
		return "Имена", "Обезличивание имён"
	case "PATRONYMIC":
		return "Отчества", "Обезличивание отчеств"
	case "SNILS":
		return "Страховые номера РФ (модуль 101)", "Обезличивание СНИЛС"
	case "PASSPORT":
		return "Серия и номер гражданина РФ", "Обезличивание паспортов"
	case "PHONE":
		return "Мобильные операторы РФ", "Обезличивание телефонов"
	case "INN":
		return "ИНН физлица (алгоритм ФНС)", "Обезличивание ИНН"
	case "BANK_CARD":
		return "Номера банковских карт", "Обезличивание карт"
	case "DATE":
		return "Даты и периоды", "Обезличивание дат"
	case "EMAIL":
		return "Корпоративная и личная почта", "Обезличивание email"
	case "ADDRESS":
		return "Адреса проживания и регистрации", "Обезличивание адресов"
	case "GRADE":
		return "Грейды и квалификационные разряды", "Обезличивание грейдов"
	case "EMPLOYEE_ID":
		return "Табельные номера сотрудников", "Обезличивание табельных номеров"
	default:
		return "Персональные данные", "Обезличивание данных"
	}
}

func inspectDOCX(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open docx file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat docx file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewDOCXHandler(anon)
	paramCount, entitiesCounts, snippets, paragraphPreviews, documentBlocks, err := handler.Inspect(file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to inspect docx file: %w", err)
	}

	var entities []domain.EntityInfo
	for i, ec := range entitiesCounts {
		desc, action := textEntityDetails(ec.Type)
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: i + 1,
			ColumnName:  fmt.Sprintf("%s (%d)", textEntityTypeName(ec.Type), ec.Count),
			Type:        ec.Type,
			Description: desc,
			Action:      action,
		})
	}

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "docx",
		FileType:               "text",
		Format:                 "docx",
		SupportedExportFormats: []string{"docx", "odt"},
		Size:                   stat.Size(),
		RowCount:               paramCount,
		ParagraphCount:         paramCount,
		PageCount:              1,
		Entities:               entities,
		Headers:                []string{"Тип сущности", "Количество", "Действие"},
		TextSnippets:           snippets,
		ParagraphPreviews:      paragraphPreviews,
		DocumentBlocks:         documentBlocks,
	}, nil
}

func inspectODT(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open odt file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat odt file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewODTHandler(anon)
	paramCount, entitiesCounts, snippets, paragraphPreviews, documentBlocks, err := handler.Inspect(file, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to inspect odt file: %w", err)
	}

	var entities []domain.EntityInfo
	for i, ec := range entitiesCounts {
		desc, action := textEntityDetails(ec.Type)
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: i + 1,
			ColumnName:  fmt.Sprintf("%s (%d)", textEntityTypeName(ec.Type), ec.Count),
			Type:        ec.Type,
			Description: desc,
			Action:      action,
		})
	}

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "odt",
		FileType:               "text",
		Format:                 "odt",
		SupportedExportFormats: []string{"odt", "docx"},
		Size:                   stat.Size(),
		RowCount:               paramCount,
		ParagraphCount:         paramCount,
		PageCount:              1,
		Entities:               entities,
		Headers:                []string{"Тип сущности", "Количество", "Действие"},
		TextSnippets:           snippets,
		ParagraphPreviews:      paragraphPreviews,
		DocumentBlocks:         documentBlocks,
	}, nil
}

func inspectTXT(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open txt file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat txt file: %w", err)
	}

	anon := anonymizer.New(defaultPreviewSalt)
	handler := formats.NewTXTHandler(anon)
	paramCount, entitiesCounts, snippets, paragraphPreviews, documentBlocks, err := handler.Inspect(file)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect txt file: %w", err)
	}

	var entities []domain.EntityInfo
	for i, ec := range entitiesCounts {
		desc, action := textEntityDetails(ec.Type)
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: i + 1,
			ColumnName:  fmt.Sprintf("%s (%d)", textEntityTypeName(ec.Type), ec.Count),
			Type:        ec.Type,
			Description: desc,
			Action:      action,
		})
	}

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "txt",
		FileType:               "text",
		Format:                 "txt",
		SupportedExportFormats: []string{"txt"},
		Size:                   stat.Size(),
		RowCount:               paramCount,
		ParagraphCount:         paramCount,
		PageCount:              1,
		Entities:               entities,
		Headers:                []string{"Тип сущности", "Количество", "Действие"},
		TextSnippets:           snippets,
		ParagraphPreviews:      paragraphPreviews,
		DocumentBlocks:         documentBlocks,
	}, nil
}

func inspectPDF(ctx context.Context, path string) (*domain.FileInfo, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	pdfBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pdf: %w", err)
	}
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		return nil, errors.New("invalid PDF file: missing %PDF- header")
	}

	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	snippets, entities, fullText, pageCount := extractPDFInfo(ctx, pdfBytes)

	snippetPreview := fullText
	if len([]rune(snippetPreview)) > 600 {
		snippetPreview = string([]rune(snippetPreview)[:600]) + "..."
	}

	snippetRedacted := snippetPreview
	for _, s := range snippets {
		if strings.Contains(snippetRedacted, s.Original) {
			snippetRedacted = strings.ReplaceAll(snippetRedacted, s.Original, formats.BlackoutMarker)
		}
	}
	if !strings.Contains(snippetRedacted, formats.BlackoutMarker) && len(snippets) > 0 {
		firstOrig := snippets[0].Original
		if idx := strings.Index(fullText, firstOrig); idx != -1 {
			start := idx - 100
			if start < 0 {
				start = 0
			}
			end := idx + len(firstOrig) + 200
			if end > len(fullText) {
				end = len(fullText)
			}
			focused := fullText[start:end]
			for _, s := range snippets {
				if strings.Contains(focused, s.Original) {
					focused = strings.ReplaceAll(focused, s.Original, formats.BlackoutMarker)
				}
			}
			snippetRedacted = focused
		}
	}

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "pdf",
		FileType:               "pdf",
		Format:                 "pdf",
		SupportedExportFormats: []string{"pdf"},
		Size:                   int64(len(pdfBytes)),
		RowCount:               0,
		ParagraphCount:         0,
		PageCount:              pageCount,
		Entities:               entities,
		Headers:                []string{"Тип сущности", "Количество", "Действие"},
		TextSnippets:           snippets,
		PDFTextSnippet:         snippetPreview,
		PDFRedactedSnippet:     snippetRedacted,
	}, nil
}

type pdfEntityMatch struct {
	start      int
	end        int
	value      string
	entityType string
}

func findPDFSensitiveMatches(text string) []pdfEntityMatch {
	var matches []pdfEntityMatch

	addRegexMatches := func(re *regexp.Regexp, submatchIdx int, entityType string, validateFunc func(string) bool) {
		indexes := re.FindAllStringSubmatchIndex(text, -1)
		for _, loc := range indexes {
			var start, end int
			if submatchIdx*2+1 < len(loc) && loc[submatchIdx*2] >= 0 {
				start = loc[submatchIdx*2]
				end = loc[submatchIdx*2+1]
			} else {
				start = loc[0]
				end = loc[1]
			}

			val := text[start:end]
			trimmed := strings.TrimSpace(val)
			if trimmed == "" {
				continue
			}
			leadTrim := strings.Index(val, trimmed)
			if leadTrim > 0 {
				start += leadTrim
				val = trimmed
				end = start + len(val)
			}

			if validateFunc == nil || validateFunc(val) {
				matches = append(matches, pdfEntityMatch{
					start:      start,
					end:        end,
					value:      val,
					entityType: entityType,
				})
			}
		}
	}

	// 1. SNILS
	addRegexMatches(reSNILSFormatted, 1, string(detector.EntitySNILS), nil)
	addRegexMatches(reSNILS11Digits, 1, string(detector.EntitySNILS), func(s string) bool {
		return detector.ValidateSNILS(s)
	})

	// 2. Passport
	addRegexMatches(rePassport4_6, 1, string(detector.EntityPassport), nil)
	addRegexMatches(rePassport2_2_6, 1, string(detector.EntityPassport), nil)
	addRegexMatches(rePassportPrefixed, 1, string(detector.EntityPassport), nil)

	// 3. INN
	addRegexMatches(reINN12, 1, string(detector.EntityINN), func(s string) bool {
		return detector.ValidateINN12(s)
	})
	addRegexMatches(reINNPrefixed, 1, string(detector.EntityINN), nil)

	// 4. Phone
	addRegexMatches(rePhone, 1, string(detector.EntityPhone), nil)
	addRegexMatches(rePhonePrefixed, 1, string(detector.EntityPhone), nil)

	// 5. Bank Card
	addRegexMatches(reCard16, 1, string(detector.EntityCard), nil)

	// 6. FIO
	addRegexMatches(reFIOPatronymic, 1, string(detector.EntityFIO), isFIOValid)
	addRegexMatches(reFIOPrefixed, 1, string(detector.EntityFIO), isFIOValid)
	addRegexMatches(reFIOInitials1, 1, string(detector.EntityFIO), nil)
	addRegexMatches(reFIOInitials2, 1, string(detector.EntityFIO), nil)

	if len(matches) <= 1 {
		return matches
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].start == matches[j].start {
			return matches[i].end > matches[j].end
		}
		return matches[i].start < matches[j].start
	})

	var deduped []pdfEntityMatch
	lastEnd := -1
	for _, m := range matches {
		if m.start >= lastEnd {
			deduped = append(deduped, m)
			lastEnd = m.end
		}
	}
	return deduped
}

func isFIOValid(s string) bool {
	parts := strings.Fields(s)
	if len(parts) < 2 || len(parts) > 3 {
		return false
	}
	stopWords := map[string]bool{
		"дает": true, "обязуется": true, "обязан": true, "вправе": true, "имеет": true,
		"согласен": true, "поручает": true, "принимает": true, "согласие": true,
		"обработку": true, "компании": true, "договора": true, "настоящий": true,
		"трудовой": true, "договор": true, "общество": true, "ответственностью": true,
		"стороны": true, "сторона": true, "работодатель": true, "работник": true,
		"предмет": true, "оплата": true, "режим": true, "срок": true, "действие": true,
		"защита": true, "персональных": true, "данных": true, "конфиденциальность": true,
		"адреса": true, "реквизиты": true, "подписи": true, "отметка": true, "получении": true,
		"экземпляра": true, "генеральный": true, "директор": true, "руководитель": true,
		"ведущего": true, "разработчика": true, "программного": true, "обеспечения": true,
		"условия": true, "рабочего": true, "времени": true, "отдыха": true, "гарантии": true,
	}
	for _, p := range parts {
		if stopWords[strings.ToLower(p)] {
			return false
		}
		subparts := strings.Split(p, "-")
		for _, sp := range subparts {
			runes := []rune(sp)
			if len(runes) < 2 {
				return false
			}
			if !unicode.IsUpper(runes[0]) {
				return false
			}
			for _, r := range runes[1:] {
				if !unicode.IsLower(r) {
					return false
				}
			}
			if stopWords[strings.ToLower(sp)] {
				return false
			}
		}
	}
	return true
}

func extractPDFInfo(ctx context.Context, pdfBytes []byte) ([]formats.DocxTextSnippet, []domain.EntityInfo, string, int) {
	pageCount, rawText, err := formats.ExtractPDFRawTextAndPageCount(pdfBytes)
	if err != nil || pageCount <= 0 {
		pageCount = 1
	}

	matches := findPDFSensitiveMatches(rawText)
	entityTypeCounts := make(map[string]int)
	var snippets []formats.DocxTextSnippet

	for _, m := range matches {
		if ctx != nil && ctx.Err() != nil {
			break
		}
		entityTypeCounts[m.entityType]++
		snippets = append(snippets, formats.DocxTextSnippet{
			Original:   m.value,
			Anonymized: formats.BlackoutMarker,
			Type:       m.entityType,
		})
	}

	entityOrder := []string{"FIO", "SNILS", "PASSPORT", "PHONE", "INN", "BANK_CARD", "CARD", "EMAIL", "ADDRESS", "DATE"}
	var entities []domain.EntityInfo
	cardIdx := 1

	for _, et := range entityOrder {
		count, ok := entityTypeCounts[et]
		if ok && count > 0 {
			desc, _ := textEntityDetails(et)
			entities = append(entities, domain.EntityInfo{
				ColumnIndex: cardIdx,
				ColumnName:  fmt.Sprintf("%s (%d)", textEntityTypeName(et), count),
				Type:        et,
				Description: desc,
				Action:      "Черная векторная плашка",
			})
			cardIdx++
		}
	}

	for et, count := range entityTypeCounts {
		found := false
		for _, ordered := range entityOrder {
			if ordered == et {
				found = true
				break
			}
		}
		if !found && count > 0 {
			desc, _ := textEntityDetails(et)
			entities = append(entities, domain.EntityInfo{
				ColumnIndex: cardIdx,
				ColumnName:  fmt.Sprintf("%s (%d)", textEntityTypeName(et), count),
				Type:        et,
				Description: desc,
				Action:      "Черная векторная плашка",
			})
			cardIdx++
		}
	}

	return snippets, entities, rawText, pageCount
}

