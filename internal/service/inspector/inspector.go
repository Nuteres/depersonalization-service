package inspector

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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

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
	reFIOPatronymic    = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])([А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична))`)
	reFIOPrefixed      = regexp.MustCompile(`(?i)(?:сотрудник|клиент|пациент|фио|работник|физлицо)[:\s]*([А-ЯЁ][а-яё]+(?:\s+[А-ЯЁ][а-яё]+){1,2})`)
	rePage             = regexp.MustCompile(`/Type\s*/Page\b`)
	reLen              = regexp.MustCompile(`/Length\s+(\d+)`)
	reLit              = regexp.MustCompile(`\(([^)\\]*(?:\\.[^)\\]*)*)\)`)
)

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

	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".csv", ".tsv":
		return inspectCSV(path)
	case ".xlsx", ".xlsm", ".xltx", ".xltm":
		return inspectXLSX(path)
	case ".ods":
		return inspectODS(path)
	case ".xlsb":
		return inspectXLSB(path)
	case ".xls":
		return inspectXLS(path)
	case ".docx":
		return inspectDOCX(path)
	case ".odt":
		return inspectODT(path)
	case ".pdf":
		return inspectPDF(path)
	default:
		return nil, fmt.Errorf("%w: %s", domain.ErrUnsupportedFormat, ext)
	}
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

func inspectCSV(path string) (*domain.FileInfo, error) {
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

	anon := anonymizer.New("preview-salt-152fz")
	var previewRows []domain.PreviewRow
	rowCount := 0

	for i := headerIdx + 1; i < len(sample); i++ {
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
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
			}
			previewRows = append(previewRows, domain.PreviewRow{
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
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
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

func inspectSingleSheet(xl *excelize.File, sheetName string, sheetIndex int, anon *anonymizer.Anonymizer) (*domain.SheetInfo, error) {
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

	colTypes := formats.ResolveColumnTypes(headers, nil)
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

	var previewRows []domain.PreviewRow
	rowCount := 0

	// Preview rows and row count from sample
	for i := headerIdx + 1; i < len(sample); i++ {
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
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
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
						after[j] = formats.AnonymizeCell(anon, ct, val, 30)
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

func inspectXLSX(path string) (*domain.FileInfo, error) {
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

	anon := anonymizer.New("preview-salt-152fz")
	var allSheets []domain.SheetInfo
	for idx, sName := range sheets {
		sInfo, err := inspectSingleSheet(xl, sName, idx, anon)
		if err == nil && sInfo != nil {
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

	colTypes := formats.ResolveColumnTypes(headers, nil)
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
				after[j] = formats.AnonymizeCell(anon, ct, val, 30)
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

func inspectODS(path string) (*domain.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open ods file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat ods file: %w", err)
	}

	anon := anonymizer.New("preview-salt-152fz")
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

func inspectXLSB(path string) (*domain.FileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat xlsb file: %w", err)
	}

	anon := anonymizer.New("preview-salt-152fz")
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

func inspectXLS(path string) (*domain.FileInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat xls file: %w", err)
	}

	anon := anonymizer.New("preview-salt-152fz")
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
	default:
		return t
	}
}

func textEntityDetails(t string) (desc, action string) {
	switch t {
	case "FIO":
		return "Русские фамилии, имена и отчества", "Синтетический профиль"
	case "SNILS":
		return "Страховые номера РФ (модуль 101)", "Валидный СНИЛС"
	case "PASSPORT":
		return "Серия и номер гражданина РФ", "Случайная серия/номер"
	case "PHONE":
		return "Мобильные операторы РФ", "Псевдонимный номер"
	case "INN":
		return "ИНН физлица (алгоритм ФНС)", "Контрольная сумма ФНС"
	case "BANK_CARD":
		return "Номера банковских карт", "Карта МИР (Luhn)"
	case "DATE":
		return "Даты и периоды", "Смещение ±30 дней"
	case "EMAIL":
		return "Корпоративная и личная почта", "Псевдонимный адрес"
	default:
		return "Персональные данные", "Обезличивание"
	}
}

func inspectDOCX(path string) (*domain.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open docx file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat docx file: %w", err)
	}

	anon := anonymizer.New("preview-salt-152fz")
	handler := formats.NewDOCXHandler(anon)
	paramCount, entitiesCounts, snippets, err := handler.Inspect(file, stat.Size())
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
	}, nil
}

func inspectODT(path string) (*domain.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open odt file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat odt file: %w", err)
	}

	anon := anonymizer.New("preview-salt-152fz")
	handler := formats.NewODTHandler(anon)
	paramCount, entitiesCounts, snippets, err := handler.Inspect(file, stat.Size())
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
	}, nil
}

func inspectPDF(path string) (*domain.FileInfo, error) {
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

	return &domain.FileInfo{
		Path:                   path,
		Name:                   filepath.Base(path),
		Extension:              "pdf",
		FileType:               "pdf",
		Format:                 "pdf",
		SupportedExportFormats: []string{"pdf"},
		Size:                   int64(len(pdfBytes)),
		RowCount:               pageCount,
		PageCount:              pageCount,
		Entities:               entities,
		Headers:                []string{"Сущность", "Категория", "Режим защиты"},
		PDFTextSnippet:         snippetBefore,
		PDFRedactedSnippet:     snippetAfter,
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

func detectPDFEntitiesAndSnippets(text string) ([]domain.EntityInfo, string, string) {
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

	var entities []domain.EntityInfo
	cardIdx := 1
	if hasFIO {
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "ФИО (PDF)",
			Type:        "FIO",
			Description: "Русские фамилии, имена и отчества",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasSNILS {
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "СНИЛС (PDF)",
			Type:        "SNILS",
			Description: "Страховые номера РФ (модуль 101)",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasPassport {
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "Паспорт РФ (PDF)",
			Type:        "PASSPORT",
			Description: "Серия и номер гражданина РФ",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasPhone {
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "Телефон (PDF)",
			Type:        "PHONE",
			Description: "Мобильные номера РФ",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasINN {
		entities = append(entities, domain.EntityInfo{
			ColumnIndex: cardIdx,
			ColumnName:  "ИНН (PDF)",
			Type:        "INN",
			Description: "ИНН физического лица (алгоритм ФНС)",
			Action:      "Черная векторная плашка",
		})
		cardIdx++
	}
	if hasCard {
		entities = append(entities, domain.EntityInfo{
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
