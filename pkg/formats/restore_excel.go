package formats

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/depmap"
)

// RestoreExcel reads XLSX from inputPath, replaces cells matching dm.Mapping with original values across all sheets,
// writes the restored workbook to outputPath, and invokes onProgress if non-nil.
// Returns total number of restored cells.
func RestoreExcel(ctx context.Context, inputPath, outputPath string, dm *depmap.DepMap, onProgress func(pct int, proc, total int)) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	if dm == nil {
		return 0, errors.New("depmap cannot be nil")
	}
	if strings.TrimSpace(inputPath) == "" {
		return 0, errors.New("input path cannot be empty")
	}
	if strings.TrimSpace(outputPath) == "" {
		return 0, errors.New("output path cannot be empty")
	}

	xl, err := excelize.OpenFile(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open excel file: %w", err)
	}
	defer xl.Close()

	sheetList := xl.GetSheetList()
	if len(sheetList) == 0 {
		return 0, nil
	}

	// Calculate total rows for progress tracking
	totalRows := 0
	for _, sheet := range sheetList {
		counted := false
		dim, err := xl.GetSheetDimension(sheet)
		if err == nil && strings.Contains(dim, ":") {
			parts := strings.Split(dim, ":")
			if len(parts) >= 2 {
				lastCell := parts[len(parts)-1]
				_, r, err := excelize.SplitCellName(lastCell)
				if err == nil && r > 1 {
					totalRows += r
					counted = true
				}
			}
		}
		if !counted {
			rIter, err := xl.Rows(sheet)
			if err == nil {
				sheetRows := 0
				for rIter.Next() {
					sheetRows++
				}
				_ = rIter.Close()
				totalRows += sheetRows
			}
		}
	}

	restoredCount := 0
	procRows := 0
	lastReportedPct := -1

	reportProgress := func(force bool) {
		if onProgress == nil {
			return
		}
		total := totalRows
		if procRows > total {
			total = procRows
		}
		pct := 0
		if total > 0 {
			pct = (procRows * 100) / total
			if pct > 99 {
				pct = 99
			}
		}
		if force || procRows%50 == 0 || pct != lastReportedPct {
			lastReportedPct = pct
			onProgress(pct, procRows, total)
		}
	}

	// Initial progress report (0%)
	reportProgress(true)

	type cellChange struct {
		cell string
		val  string
	}

	isDeleteMode := dm.Mode == "delete" || len(dm.CellMappings) > 0
	targetHeadersMap := make(map[string]bool)
	if len(dm.TargetHeaders) > 0 {
		for _, h := range dm.TargetHeaders {
			targetHeadersMap[strings.TrimSpace(h)] = true
		}
	}
	hasHeaderAwareness := isDeleteMode || len(dm.TargetHeaders) > 0

	for _, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		// Sample initial rows to detect headers
		sampleRows, err := xl.Rows(sheetName)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
		}
		var sample [][]string
		sampleLimit := 40
		for sampleRows.Next() && len(sample) < sampleLimit {
			cols, err := sampleRows.Columns()
			if err != nil {
				break
			}
			sample = append(sample, cols)
		}
		_ = sampleRows.Close()

		headerRowIndex := -1
		var headers []string
		if len(sample) > 0 {
			headerRowIndex, headers = FindHeaderRow(sample)
		}

		rows, err := xl.Rows(sheetName)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
		}

		var changes []cellChange
		rowNum := 0

		for rows.Next() {
			select {
			case <-ctx.Done():
				_ = rows.Close()
				return restoredCount, ctx.Err()
			default:
			}

			rowNum++
			procRows++

			cols, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return restoredCount, fmt.Errorf("failed to read columns in sheet %s row %d: %w", sheetName, rowNum, err)
			}

			if hasHeaderAwareness && len(sample) > 1 && headerRowIndex >= 0 && rowNum <= headerRowIndex+1 {
				reportProgress(false)
				continue
			}

			for colIdx, val := range cols {
				if len(val) == 0 {
					continue
				}

				var headerName string
				if colIdx < len(headers) {
					headerName = strings.TrimSpace(headers[colIdx])
				}

				if restoredVal, ok := RestoreCellString(val, dm); ok {
					if len(dm.TargetHeaders) == 0 || targetHeadersMap[headerName] || strings.Contains(val, "[") {
						coord, err := excelize.CoordinatesToCellName(colIdx+1, rowNum)
						if err == nil {
							changes = append(changes, cellChange{cell: coord, val: restoredVal})
						}
						continue
					}
				}

				if isDeleteMode {
					if orig, ok := dm.GetCell(sheetName, headerName, rowNum); ok {
						trimmed := strings.TrimSpace(strings.TrimPrefix(val, "\ufeff"))
						if trimmed == DeletionMarker {
							coord, err := excelize.CoordinatesToCellName(colIdx+1, rowNum)
							if err == nil {
								changes = append(changes, cellChange{cell: coord, val: orig})
							}
							continue
						}
					}
				}
			}

			reportProgress(false)
		}

		if err := rows.Error(); err != nil {
			_ = rows.Close()
			return restoredCount, fmt.Errorf("error reading rows in sheet %s: %w", sheetName, err)
		}

		if err := rows.Close(); err != nil {
			return restoredCount, fmt.Errorf("failed to close rows in sheet %s: %w", sheetName, err)
		}

		// Apply cell changes for this sheet
		for _, ch := range changes {
			select {
			case <-ctx.Done():
				return restoredCount, ctx.Err()
			default:
			}
			if err := xl.SetCellValue(sheetName, ch.cell, ch.val); err != nil {
				return restoredCount, fmt.Errorf("failed to set cell %s in sheet %s: %w", ch.cell, sheetName, err)
			}
			restoredCount++
		}
	}

	// Final progress callback reaching 100%
	if onProgress != nil {
		total := totalRows
		if procRows > total {
			total = procRows
		}
		onProgress(100, total, total)
	}

	select {
	case <-ctx.Done():
		return restoredCount, ctx.Err()
	default:
	}

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return restoredCount, fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
	}

	if err := xl.SaveAs(outputPath); err != nil {
		return restoredCount, fmt.Errorf("failed to save restored excel file to %s: %w", outputPath, err)
	}

	return restoredCount, nil
}
