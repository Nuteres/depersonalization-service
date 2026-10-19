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

	"dep_service/internal/domain"
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

// InspectRows returns sheet metadata and up to sampleLimit rows per sheet.
func (h *XLSBHandler) InspectRows(path string, sampleLimit int) ([]SheetInfo, map[string][][]string, error) {
	wb, err := xlsb.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open xlsb: %w", err)
	}
	defer wb.Close()

	if sampleLimit <= 0 {
		sampleLimit = 40
	}

	sheetNames := wb.Sheets()
	var sheets []SheetInfo
	samples := make(map[string][][]string)

	for i, name := range sheetNames {
		sheet, err := wb.SheetByName(name)
		if err != nil {
			continue
		}

		var sample [][]string
		rowCount := 0

		for row := range sheet.Rows(false) {
			rowCount++
			if len(sample) < sampleLimit {
				cols := make([]string, len(row))
				for cIdx, cell := range row {
					cols[cIdx] = sheet.FormatCell(cell)
				}
				sample = append(sample, cols)
			}
		}

		if sheet.Err != nil {
			return nil, nil, fmt.Errorf("error reading sheet %s: %w", name, sheet.Err)
		}

		samples[name] = sample
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

	return sheets, samples, nil
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
	type pendingXlsbRow struct {
		dataCells []interface{}
		origCells []string
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
		colTypes := ResolveColumnTypesWithSamples(headers, sample[headerRowIndex+1:], options)

		// Build map of disabled columns (1-based index)
		disabledMap := BuildDisabledMap(sheetName, options)

		var pendingDataRows []pendingXlsbRow

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
					pendingDataRows = append(pendingDataRows, pendingXlsbRow{dataCells: dataCells, origCells: colValues})
				} else {
					if err := sw.SetRow(cellCoordinate, dataCells); err != nil {
						return nil, fmt.Errorf("failed to write data row %d in sheet %s: %w", rowNum, sheetName, err)
					}
				}
			}

			if rowNum <= headerRowIndex+1 {
				totalProcessedRows++
			} else if !shouldShuffle {
				totalProcessedRows++
				if progressFn != nil && totalProcessedRows%50 == 0 {
					progressFn(totalProcessedRows)
				}
			}

			rowNum++
		}

		if sheet.Err != nil {
			return nil, fmt.Errorf("error reading sheet %s: %w", sheetName, sheet.Err)
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
