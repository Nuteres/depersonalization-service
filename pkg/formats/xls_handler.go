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

	"dep_service/internal/domain"
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

// SafeGetXLSRow retrieves row i safely, catching any panics from extrame/xls.
func SafeGetXLSRow(sheet *xls.WorkSheet, i int) (r *xls.Row) {
	defer func() {
		if rec := recover(); rec != nil {
			r = nil
		}
	}()
	return sheet.Row(i)
}

func safeGetXLSRow(sheet *xls.WorkSheet, i int) *xls.Row {
	return SafeGetXLSRow(sheet, i)
}

// GetXLSRowValues extracts cell string values from an xls.Row safely.
func GetXLSRowValues(row *xls.Row) (cols []string) {
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

func getXLSRowValues(row *xls.Row) []string {
	return GetXLSRowValues(row)
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

// InspectRows returns sheet metadata and up to sampleLimit rows per sheet.
func (h *XLSHandler) InspectRows(path string, sampleLimit int) ([]SheetInfo, map[string][][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open xls file: %w", err)
	}
	defer f.Close()

	wb, err := xls.OpenReader(f, "utf-8")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse xls: %w", err)
	}
	if wb == nil {
		return nil, nil, fmt.Errorf("failed to parse xls workbook: %s", path)
	}

	if sampleLimit <= 0 {
		sampleLimit = 40
	}

	numSheets := wb.NumSheets()
	var sheets []SheetInfo
	samples := make(map[string][][]string)

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
		for r := 0; r < sheetRows && len(sample) < sampleLimit; r++ {
			row := safeGetXLSRow(sheet, r)
			cols := getXLSRowValues(row)
			sample = append(sample, cols)
		}

		samples[sheet.Name] = sample
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

	return sheets, samples, nil
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
	strategy := getProcessingStrategy(options)
	salt := ""
	if options != nil {
		if s, ok := options["salt"].(string); ok {
			salt = s
		}
	}
	ts := anonymizer.NewTokenSession(salt)

	var dm *depmap.DepMap
	if d, ok := options["depmap"].(*depmap.DepMap); ok {
		dm = d
	} else if d, ok := options["depMap"].(*depmap.DepMap); ok {
		dm = d
	}
	if dm == nil {
		dm = depmap.New(origFileName)
	}
	dm.Mode = string(strategy)

	outFile := excelize.NewFile()
	defer outFile.Close()

	dateRange := getDateRange(options)

	var progressFn func(int)
	if options != nil {
		if cb, ok := options["progress_callback"].(func(int)); ok {
			progressFn = cb
		}
	}

	shouldShuffle := isShuffleRows(options)
	type pendingXlsRow struct {
		dataCells []interface{}
		origCells []string
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
		colTypes := ResolveColumnTypesWithSamples(headers, sample[headerRowIndex+1:], options)

		// Build map of disabled columns (1-based index)
		disabledMap := BuildDisabledMap(sheetName, options)

		var pendingDataRows []pendingXlsRow

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
					newVal := ProcessCellWithStrategy(h.Anon, ts, colType, v, strategy, dateRange)
					dataCells[i] = newVal

					if colType != ColUnknown && strings.TrimSpace(v) != "" && dm != nil {
						var headerName string
						if i < len(headers) {
							headerName = headers[i]
						}
						dm.AddTargetHeader(headerName)
						if strategy == domain.StrategyDelete {
							if !shouldShuffle {
								dm.PutCell(sheetName, headerName, rowNum, v)
							}
						} else {
							dm.Put(newVal, v)
						}
					}
				}
				if shouldShuffle {
					pendingDataRows = append(pendingDataRows, pendingXlsRow{dataCells: dataCells, origCells: colValues})
				} else {
					if err := sw.SetRow(cellCoordinate, dataCells); err != nil {
						return nil, fmt.Errorf("failed to write data row %d in sheet %s: %w", rowNum, sheetName, err)
					}
				}
			}

			if r <= headerRowIndex {
				totalProcessedRows++
			} else if !shouldShuffle {
				totalProcessedRows++
				if progressFn != nil && totalProcessedRows%50 == 0 {
					progressFn(totalProcessedRows)
				}
			}
		}

		if shouldShuffle && len(pendingDataRows) > 0 {
			ShuffleRows(h.Anon, pendingDataRows)
			for i, pr := range pendingDataRows {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				default:
				}
				targetRowNum := headerRowIndex + 2 + i
				cellCoordinate, err := excelize.CoordinatesToCellName(1, targetRowNum)
				if err != nil {
					return nil, fmt.Errorf("failed to get cell coordinate for row %d: %w", targetRowNum, err)
				}
				if err := sw.SetRow(cellCoordinate, pr.dataCells); err != nil {
					return nil, fmt.Errorf("failed to write shuffled data row %d in sheet %s: %w", targetRowNum, sheetName, err)
				}
				if dm != nil && strategy == domain.StrategyDelete {
					for colIdx, v := range pr.origCells {
						var colType ColumnType
						col1Based := colIdx + 1
						if colIdx < len(colTypes) && !disabledMap[col1Based] {
							colType = colTypes[colIdx]
						}
						if colType != ColUnknown && strings.TrimSpace(v) != "" {
							var headerName string
							if colIdx < len(headers) {
								headerName = headers[colIdx]
							}
							dm.PutCell(sheetName, headerName, targetRowNum, v)
						}
					}
				}
				totalProcessedRows++
				if progressFn != nil && totalProcessedRows%50 == 0 {
					progressFn(totalProcessedRows)
				}
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
