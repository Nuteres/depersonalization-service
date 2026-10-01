package formats

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

// XLSHandler implements inspection and anonymization for Microsoft Excel Binary (.xls) files.
type XLSHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewXLSHandler creates a new XLSHandler with the provided Anonymizer.
func NewXLSHandler(anon *anonymizer.Anonymizer) *XLSHandler {
	return &XLSHandler{Anon: anon}
}

// SupportedExtensions returns the file extensions supported by this handler.
func (h *XLSHandler) SupportedExtensions() []string {
	return []string{".xls"}
}

func safeGetXLSRow(sheet *xls.WorkSheet, i int) (r *xls.Row) {
	defer func() {
		if rec := recover(); rec != nil {
			r = nil
		}
	}()
	return sheet.Row(i)
}

func getXLSRowValues(row *xls.Row) (cols []string) {
	if row == nil {
		return nil
	}
	defer func() {
		if rec := recover(); rec != nil {
			cols = nil
		}
	}()
	lastCol := row.LastCol()
	if lastCol <= 0 {
		return nil
	}
	cols = make([]string, lastCol)
	for c := 0; c < lastCol; c++ {
		cols[c] = row.Col(c)
	}
	return cols
}

// Inspect examines the sheets in the .xls workbook, detecting column headers and row counts.
func (h *XLSHandler) Inspect(path string) ([]SheetInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xls file: %w", err)
	}
	defer f.Close()

	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("failed to parse xls: %w", err)
	}
	if wb == nil {
		return nil, fmt.Errorf("failed to parse xls workbook: %s", path)
	}

	numSheets := wb.NumSheets()
	var sheets []SheetInfo

	for i := 0; i < numSheets; i++ {
		sheet := wb.GetSheet(i)
		if sheet == nil {
			continue
		}

		sheetRows := 0
		if int(sheet.MaxRow) > 0 || safeGetXLSRow(sheet, 0) != nil {
			sheetRows = int(sheet.MaxRow) + 1
		}

		var sample [][]string
		for r := 0; r < sheetRows && len(sample) < 40; r++ {
			row := safeGetXLSRow(sheet, r)
			cols := getXLSRowValues(row)
			sample = append(sample, cols)
		}

		_, headers := FindHeaderRow(sample)
		if len(headers) > 0 {
			headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
		}

		sheets = append(sheets, SheetInfo{
			Index:    i,
			Name:     sheet.Name,
			Columns:  headers,
			Headers:  headers,
			RowCount: sheetRows,
		})
	}

	return sheets, nil
}

// ProcessToXLSX reads sheets from an .xls workbook at path, anonymizes sensitive PII cells,
// and streams the result into an Excel (.xlsx) file written to w, returning a populated DepMap.
func (h *XLSHandler) ProcessToXLSX(ctx context.Context, path string, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error) {
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

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open xls file: %w", err)
	}
	defer f.Close()

	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		return nil, fmt.Errorf("failed to parse xls: %w", err)
	}
	if wb == nil {
		return nil, fmt.Errorf("failed to parse xls workbook: %s", path)
	}

	numSheets := wb.NumSheets()
	if numSheets == 0 {
		return nil, errors.New("xls workbook has no sheets")
	}

	var targetSheets []*xls.WorkSheet
	if targetSheetName, ok := options["sheet"].(string); ok && targetSheetName != "" {
		for i := 0; i < numSheets; i++ {
			if s := wb.GetSheet(i); s != nil && s.Name == targetSheetName {
				targetSheets = []*xls.WorkSheet{s}
				break
			}
		}
		if len(targetSheets) == 0 {
			return nil, fmt.Errorf("sheet %q not found in workbook", targetSheetName)
		}
	} else {
		for i := 0; i < numSheets; i++ {
			if s := wb.GetSheet(i); s != nil {
				targetSheets = append(targetSheets, s)
			}
		}
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

	for sheetIdx, sheet := range targetSheets {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		sheetName := sheet.Name
		if sheetName == "" {
			sheetName = fmt.Sprintf("Sheet%d", sheetIdx+1)
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

		sheetRows := 0
		if int(sheet.MaxRow) > 0 || safeGetXLSRow(sheet, 0) != nil {
			sheetRows = int(sheet.MaxRow) + 1
		}

		// Sample leading rows to locate the header row
		var sample [][]string
		for r := 0; r < sheetRows && len(sample) < 40; r++ {
			row := safeGetXLSRow(sheet, r)
			cols := getXLSRowValues(row)
			sample = append(sample, cols)
		}

		headerRowIndex, headers := FindHeaderRow(sample)
		if len(headers) > 0 {
			headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
		}
		colTypes := ResolveColumnTypes(headers, options)

		// Build map of disabled columns (1-based index)
		disabledMap := BuildDisabledMap(sheetName, options)

		for r := 0; r < sheetRows; r++ {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			rowNum := r + 1
			cellCoordinate, err := excelize.CoordinatesToCellName(1, rowNum)
			if err != nil {
				return nil, fmt.Errorf("failed to get cell coordinate for row %d: %w", rowNum, err)
			}

			row := safeGetXLSRow(sheet, r)
			colValues := getXLSRowValues(row)

			if len(colValues) < len(headers) {
				padded := make([]string, len(headers))
				copy(padded, colValues)
				colValues = padded
			}

			if r < headerRowIndex {
				// Preamble row before table header: write unaltered
				preambleCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					preambleCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, preambleCells); err != nil {
					return nil, fmt.Errorf("failed to write preamble row %d: %w", rowNum, err)
				}
			} else if r == headerRowIndex {
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
