package formats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/TsubasaBE/go-xlsb"
	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

// XLSBHandler implements inspection and anonymization for Microsoft Excel Binary Workbook (.xlsb) files.
type XLSBHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewXLSBHandler creates a new XLSBHandler with the provided Anonymizer.
func NewXLSBHandler(anon *anonymizer.Anonymizer) *XLSBHandler {
	return &XLSBHandler{Anon: anon}
}

// SupportedExtensions returns the file extensions supported by this handler.
func (h *XLSBHandler) SupportedExtensions() []string {
	return []string{".xlsb"}
}

// Inspect examines the sheets in the .xlsb workbook, detecting column headers and row counts.
func (h *XLSBHandler) Inspect(path string) ([]SheetInfo, error) {
	wb, err := xlsb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsb workbook: %w", err)
	}
	defer wb.Close()

	sheetNames := wb.Sheets()
	var sheets []SheetInfo

	for i, name := range sheetNames {
		sheet, err := wb.SheetByName(name)
		if err != nil {
			continue
		}

		var sample [][]string
		rowCount := 0

		for row := range sheet.Rows(false) {
			rowCount++
			if len(sample) < 40 {
				cols := make([]string, len(row))
				for cIdx, cell := range row {
					cols[cIdx] = sheet.FormatCell(cell)
				}
				sample = append(sample, cols)
			}
		}

		if sheet.Err != nil {
			return nil, fmt.Errorf("error reading sheet %s: %w", name, sheet.Err)
		}

		_, headers := FindHeaderRow(sample)
		if len(headers) > 0 {
			headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
		}

		sheets = append(sheets, SheetInfo{
			Index:    i,
			Name:     name,
			Columns:  headers,
			Headers:  headers,
			RowCount: rowCount,
		})
	}

	return sheets, nil
}

// ProcessToXLSX reads sheets from an .xlsb workbook at path, anonymizes sensitive PII cells,
// and streams the result into an Excel (.xlsx) file written to w, returning a populated DepMap.
func (h *XLSBHandler) ProcessToXLSX(ctx context.Context, path string, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
	if h.Anon == nil {
		return nil, fmt.Errorf("anonymizer is not initialized")
	}
	if w == nil {
		return nil, errors.New("writer cannot be nil")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	wb, err := xlsb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsb workbook: %w", err)
	}
	defer wb.Close()

	sheetList := wb.Sheets()
	if len(sheetList) == 0 {
		return nil, errors.New("xlsb workbook has no sheets")
	}

	// Filter sheets if a specific sheet is requested
	if targetSheet, ok := options["sheet"].(string); ok && targetSheet != "" {
		found := false
		for _, s := range sheetList {
			if s == targetSheet {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("sheet %q not found in workbook", targetSheet)
		}
		sheetList = []string{targetSheet}
	}

	origFileName := filepath.Base(path)
	if options != nil {
		if fn, ok := options["filename"].(string); ok && fn != "" {
			origFileName = fn
		} else if fn, ok := options["file_name"].(string); ok && fn != "" {
			origFileName = fn
		}
	}
	dm := depmap.New(origFileName)

	outFile := excelize.NewFile()
	defer outFile.Close()

	dateRange := getDateRange(options)

	var progressFn func(int)
	if options != nil {
		if cb, ok := options["progress_callback"].(func(int)); ok {
			progressFn = cb
		}
	}

	totalProcessedRows := 0

	for sheetIdx, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if sheetIdx == 0 {
			if sheetName != "Sheet1" {
				if err := outFile.SetSheetName("Sheet1", sheetName); err != nil {
					_, _ = outFile.NewSheet(sheetName)
				}
			}
		} else {
			if _, err := outFile.NewSheet(sheetName); err != nil {
				return nil, fmt.Errorf("failed to create sheet %s: %w", sheetName, err)
			}
		}

		sw, err := outFile.NewStreamWriter(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream writer for sheet %s: %w", sheetName, err)
		}

		sheet, err := wb.SheetByName(sheetName)
		if err != nil {
			return nil, fmt.Errorf("failed to open sheet %s: %w", sheetName, err)
		}

		// Sample leading rows to locate the header row
		var sample [][]string
		for row := range sheet.Rows(false) {
			cols := make([]string, len(row))
			for cIdx, cell := range row {
				cols[cIdx] = sheet.FormatCell(cell)
			}
			sample = append(sample, cols)
			if len(sample) >= 40 {
				break
			}
		}

		headerRowIndex, headers := FindHeaderRow(sample)
		if len(headers) > 0 {
			headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
		}
		colTypes := ResolveColumnTypes(headers, options)

		// Build map of disabled columns (1-based index)
		disabledMap := BuildDisabledMap(sheetName, options)

		rowNum := 1
		for row := range sheet.Rows(false) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			colValues := make([]string, len(row))
			for cIdx, cell := range row {
				colValues[cIdx] = sheet.FormatCell(cell)
			}

			cellCoordinate, err := excelize.CoordinatesToCellName(1, rowNum)
			if err != nil {
				return nil, fmt.Errorf("failed to get cell coordinate for row %d: %w", rowNum, err)
			}

			if rowNum < headerRowIndex+1 {
				// Preamble row before table header: write unaltered
				preambleCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					preambleCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, preambleCells); err != nil {
					return nil, fmt.Errorf("failed to write preamble row %d: %w", rowNum, err)
				}
			} else if rowNum == headerRowIndex+1 {
				// Header row: write unaltered
				headerCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					headerCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, headerCells); err != nil {
					return nil, fmt.Errorf("failed to write header row in sheet %s: %w", sheetName, err)
				}
			} else {
				// Data row: anonymize according to detected column types
				dataCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					var colType ColumnType
					col1Based := i + 1
					if i < len(colTypes) && !disabledMap[col1Based] {
						colType = colTypes[i]
					}
					anonVal := AnonymizeCell(h.Anon, colType, v, dateRange)
					dataCells[i] = anonVal

					if colType != ColUnknown && anonVal != v && strings.TrimSpace(v) != "" {
						dm.Put(anonVal, v)
						if strings.TrimSpace(anonVal) != anonVal {
							dm.Put(strings.TrimSpace(anonVal), v)
						}
					}
				}
				if err := sw.SetRow(cellCoordinate, dataCells); err != nil {
					return nil, fmt.Errorf("failed to write data row %d in sheet %s: %w", rowNum, sheetName, err)
				}
			}

			totalProcessedRows++
			if progressFn != nil && totalProcessedRows%50 == 0 {
				progressFn(totalProcessedRows)
			}

			rowNum++
		}

		if sheet.Err != nil {
			return nil, fmt.Errorf("error reading sheet %s: %w", sheetName, sheet.Err)
		}

		if err := sw.Flush(); err != nil {
			return nil, fmt.Errorf("failed to flush stream writer for sheet %s: %w", sheetName, err)
		}
	}

	if progressFn != nil {
		progressFn(totalProcessedRows)
	}

	outFile.SetActiveSheet(0)

	if err := outFile.Write(w); err != nil {
		return nil, fmt.Errorf("failed to write output excel: %w", err)
	}

	return dm, nil
}
