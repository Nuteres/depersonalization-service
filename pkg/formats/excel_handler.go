package formats

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

// ExcelHandler implements FormatHandler for Microsoft Excel (.xlsx) files using streaming.
type ExcelHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewExcelHandler creates a new ExcelHandler with the given Anonymizer.
func NewExcelHandler(anon *anonymizer.Anonymizer) *ExcelHandler {
	return &ExcelHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *ExcelHandler) SupportedExtensions() []string {
	return []string{".xlsx", ".xlsm", ".xltx", ".xltm"}
}

// Process reads sheets and rows using streaming row iterator and StreamWriter,
// anonymizes detected PII columns, and writes the output workbook to w.
func (h *ExcelHandler) Process(ctx context.Context, r io.Reader, w io.Writer, options map[string]interface{}) error {
	if h.Anon == nil {
		return fmt.Errorf("anonymizer is not initialized")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	srcFile, err := excelize.OpenReader(r)
	if err != nil {
		return fmt.Errorf("failed to open source excel: %w", err)
	}
	defer srcFile.Close()

	sheetList := srcFile.GetSheetList()
	if len(sheetList) == 0 {
		return nil
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
		if found {
			sheetList = []string{targetSheet}
		}
	}

	outFile := excelize.NewFile()
	defer outFile.Close()

	dateRange := getDateRange(options)
	strategy := getProcessingStrategy(options)
	salt := ""
	if s, ok := options["salt"].(string); ok {
		salt = s
	}
	ts := anonymizer.NewTokenSession(salt)

	var depMap *depmap.DepMap
	if dm, ok := options["depmap"].(*depmap.DepMap); ok {
		depMap = dm
	} else if dm, ok := options["depMap"].(*depmap.DepMap); ok {
		depMap = dm
	}
	if depMap != nil {
		depMap.Mode = string(strategy)
	}

	for sheetIdx, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return ctx.Err()
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
				return fmt.Errorf("failed to create sheet %s: %w", sheetName, err)
			}
		}

		sw, err := outFile.NewStreamWriter(sheetName)
		if err != nil {
			return fmt.Errorf("failed to create stream writer for sheet %s: %w", sheetName, err)
		}

		// Sample leading rows to locate the header row (skipping preamble metadata if present)
		sampleIter, err := srcFile.Rows(sheetName)
		if err != nil {
			return fmt.Errorf("failed to sample rows for sheet %s: %w", sheetName, err)
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

		srcMerges, _ := srcFile.GetMergeCells(sheetName)
		merges := ExtractMergeRanges(srcFile, sheetName)
		headerRowIndex, _ := FindHeaderRow(sample)
		headers := BuildHierarchicalHeaders(sample, headerRowIndex, merges)
		colTypes := ResolveColumnTypesWithSamples(headers, sample[headerRowIndex+1:], options)

		// Build map of disabled columns (1-based index)
		disabledMap := make(map[int]bool)
		hasSheetSpecific := false
		if options != nil {
			if bySheet, ok := options["disabled_columns_by_sheet"].(map[string][]int); ok && len(bySheet) > 0 {
				if dList, found := bySheet[sheetName]; found {
					hasSheetSpecific = true
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				}
			} else if bySheetIntf, ok := options["disabled_columns_by_sheet"].(map[string]interface{}); ok && len(bySheetIntf) > 0 {
				if sheetVal, found := bySheetIntf[sheetName]; found {
					hasSheetSpecific = true
					if dList, ok := sheetVal.([]int); ok {
						for _, colIdx := range dList {
							disabledMap[colIdx] = true
						}
					} else if dInterface, ok := sheetVal.([]interface{}); ok {
						for _, itm := range dInterface {
							if n, ok := itm.(int); ok {
								disabledMap[n] = true
							} else if f, ok := itm.(float64); ok {
								disabledMap[int(f)] = true
							}
						}
					}
				}
			}

			if !hasSheetSpecific {
				if dList, ok := options["disabled_columns"].([]int); ok {
					for _, colIdx := range dList {
						disabledMap[colIdx] = true
					}
				} else if dInterface, ok := options["disabled_columns"].([]interface{}); ok {
					for _, itm := range dInterface {
						if n, ok := itm.(int); ok {
							disabledMap[n] = true
						} else if f, ok := itm.(float64); ok {
							disabledMap[int(f)] = true
						}
					}
				}
			}
		}

		// Check for optional progress callback
		var progressFn func(int)
		if cb, ok := options["progress_callback"].(func(int)); ok {
			progressFn = cb
		}

		rows, err := srcFile.Rows(sheetName)
		if err != nil {
			return fmt.Errorf("failed to read rows for sheet %s: %w", sheetName, err)
		}

		shouldShuffle := isShuffleRows(options)
		type pendingExcelRow struct {
			dataCells []interface{}
			origCells []string
		}
		var pendingDataRows []pendingExcelRow

		rowNum := 1

		for rows.Next() {
			select {
			case <-ctx.Done():
				_ = rows.Close()
				return ctx.Err()
			default:
			}

			colValues, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("failed to read columns in sheet %s row %d: %w", sheetName, rowNum, err)
			}

			cellCoordinate, err := excelize.CoordinatesToCellName(1, rowNum)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("failed to get cell coordinate for row %d: %w", rowNum, err)
			}

			if rowNum < headerRowIndex+1 {
				// Preamble row before table header: write unaltered
				preambleCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					preambleCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, preambleCells); err != nil {
					_ = rows.Close()
					return fmt.Errorf("failed to write preamble row %d: %w", rowNum, err)
				}
			} else if rowNum == headerRowIndex+1 {
				// Header row: write unaltered
				headerCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					headerCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, headerCells); err != nil {
					_ = rows.Close()
					return fmt.Errorf("failed to write header row in sheet %s: %w", sheetName, err)
				}
			} else {
				// Data row: anonymize according to detected column types (skipping disabled columns)
				dataCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					var colType ColumnType
					col1Based := i + 1
					if i < len(colTypes) && !disabledMap[col1Based] {
						colType = colTypes[i]
					}
					newVal := ProcessCellWithStrategy(h.Anon, ts, colType, v, strategy, dateRange)
					dataCells[i] = newVal

					if colType != ColUnknown && strings.TrimSpace(v) != "" && depMap != nil {
						var headerName string
						if i < len(headers) {
							headerName = headers[i]
						}
						depMap.AddTargetHeader(headerName)
						if strategy == domain.StrategyDelete {
							if !shouldShuffle {
								depMap.PutCell(sheetName, headerName, rowNum, v)
							}
						} else {
							depMap.Put(newVal, v)
						}
					}
				}
				if shouldShuffle {
					pendingDataRows = append(pendingDataRows, pendingExcelRow{dataCells: dataCells, origCells: colValues})
				} else {
					if err := sw.SetRow(cellCoordinate, dataCells); err != nil {
						_ = rows.Close()
						return fmt.Errorf("failed to write data row %d in sheet %s: %w", rowNum, sheetName, err)
					}
				}
			}

			if progressFn != nil && !shouldShuffle && rowNum%50 == 0 {
				progressFn(rowNum)
			}

			rowNum++
		}

		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("error during row iteration in sheet %s: %w", sheetName, err)
		}

		if err := rows.Close(); err != nil {
			return fmt.Errorf("failed to close rows iterator for sheet %s: %w", sheetName, err)
		}

		if shouldShuffle && len(pendingDataRows) > 0 {
			ShuffleRows(h.Anon, pendingDataRows)
			for i, pr := range pendingDataRows {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				targetRowNum := headerRowIndex + 2 + i
				cellCoordinate, err := excelize.CoordinatesToCellName(1, targetRowNum)
				if err != nil {
					return fmt.Errorf("failed to get cell coordinate for row %d: %w", targetRowNum, err)
				}
				if err := sw.SetRow(cellCoordinate, pr.dataCells); err != nil {
					return fmt.Errorf("failed to write shuffled data row %d in sheet %s: %w", targetRowNum, sheetName, err)
				}
				if depMap != nil && strategy == domain.StrategyDelete {
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
							depMap.PutCell(sheetName, headerName, targetRowNum, v)
						}
					}
				}
				if progressFn != nil && (targetRowNum)%50 == 0 {
					progressFn(targetRowNum)
				}
			}
			if progressFn != nil {
				progressFn(headerRowIndex + 1 + len(pendingDataRows))
			}
		}

		// Preserve merged cells from source sheet
		for _, m := range srcMerges {
			_ = sw.MergeCell(m.GetStartAxis(), m.GetEndAxis())
		}

		if err := sw.Flush(); err != nil {
			return fmt.Errorf("failed to flush stream writer for sheet %s: %w", sheetName, err)
		}
	}

	outFile.SetActiveSheet(0)

	if err := outFile.Write(w); err != nil {
		return fmt.Errorf("failed to write output excel: %w", err)
	}

	return nil
}
