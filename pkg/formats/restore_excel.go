package formats

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"runtime/debug"
	"strings"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/depmap"
)

// RestoreExcel reads XLSX from inputPath, replaces cells matching dm.Mapping with original values across all sheets,
// streams the restored workbook to outputPath using StreamWriter for minimal memory footprint,
// and invokes onProgress if non-nil.
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

	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	sheetList := xl.GetSheetList()
	if len(sheetList) == 0 {
		return 0, nil
	}

	// Calculate total rows across all sheets for progress tracking
	totalRows := 0
	for _, sheet := range sheetList {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
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
					select {
					case <-ctx.Done():
						_ = rIter.Close()
						return 0, ctx.Err()
					default:
					}
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

	isDeleteMode := dm.Mode == "delete" || len(dm.CellMappings) > 0
	targetHeadersMap := make(map[string]bool)
	if len(dm.TargetHeaders) > 0 {
		for _, h := range dm.TargetHeaders {
			targetHeadersMap[strings.TrimSpace(h)] = true
		}
	}
	hasHeaderAwareness := isDeleteMode || len(dm.TargetHeaders) > 0

	outFile := excelize.NewFile()
	defer outFile.Close()

	defer func() {
		goruntime.GC()
		debug.FreeOSMemory()
	}()

	for sheetIdx, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
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
				return restoredCount, fmt.Errorf("failed to create sheet %s: %w", sheetName, err)
			}
		}

		sw, err := outFile.NewStreamWriter(sheetName)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to create stream writer for sheet %s: %w", sheetName, err)
		}

		// Sample initial rows to detect headers
		sampleRows, err := xl.Rows(sheetName)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
		}
		var sample [][]string
		sampleLimit := 40
		for sampleRows.Next() && len(sample) < sampleLimit {
			select {
			case <-ctx.Done():
				_ = sampleRows.Close()
				return restoredCount, ctx.Err()
			default:
			}
			cols, err := sampleRows.Columns()
			if err != nil {
				break
			}
			sample = append(sample, cols)
		}
		_ = sampleRows.Close()

		headerBounds := HeaderBounds{HeaderStartIdx: 0, LeafHeaderIdx: 0, DataStartIdx: 1}
		var headers []string
		if len(sample) > 0 {
			merges := ExtractMergeRanges(xl, sheetName)
			headerBounds = DetectHeaderBounds(sample, merges)
			headers = BuildHierarchicalHeaders(sample, headerBounds.LeafHeaderIdx, merges)
		}

		rows, err := xl.Rows(sheetName)
		if err != nil {
			return restoredCount, fmt.Errorf("failed to read rows from sheet %s: %w", sheetName, err)
		}

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

			cellCoordinate, err := excelize.CoordinatesToCellName(1, rowNum)
			if err != nil {
				_ = rows.Close()
				return restoredCount, fmt.Errorf("failed to get cell coordinate for row %d: %w", rowNum, err)
			}

			if hasHeaderAwareness && len(sample) > 1 && rowNum < headerBounds.DataStartIdx+1 {
				headerCells := make([]interface{}, len(cols))
				for i, v := range cols {
					headerCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, headerCells); err != nil {
					_ = rows.Close()
					return restoredCount, fmt.Errorf("failed to write header row %d: %w", rowNum, err)
				}
				reportProgress(false)
				continue
			}

			rowCells := make([]interface{}, len(cols))
			for colIdx, val := range cols {
				if len(val) == 0 {
					rowCells[colIdx] = ""
					continue
				}

				var headerName string
				if colIdx < len(headers) {
					headerName = strings.TrimSpace(headers[colIdx])
				}

				if restoredVal, ok := RestoreCellString(val, dm); ok {
					isTarget := len(dm.TargetHeaders) == 0 || targetHeadersMap[headerName] || strings.Contains(val, "[")
					if !isTarget && strings.Contains(headerName, " / ") {
						parts := strings.Split(headerName, " / ")
						leaf := strings.TrimSpace(parts[len(parts)-1])
						if idx := strings.LastIndex(leaf, " ("); idx != -1 && strings.HasSuffix(leaf, ")") {
							leaf = strings.TrimSpace(leaf[:idx])
						}
						isTarget = targetHeadersMap[leaf]
					}
					if isTarget {
						rowCells[colIdx] = restoredVal
						restoredCount++
						continue
					}
				}

				if isDeleteMode {
					orig, ok := dm.GetCell(sheetName, headerName, rowNum)
					if !ok && strings.Contains(headerName, " / ") {
						parts := strings.Split(headerName, " / ")
						leaf := strings.TrimSpace(parts[len(parts)-1])
						if idx := strings.LastIndex(leaf, " ("); idx != -1 && strings.HasSuffix(leaf, ")") {
							leaf = strings.TrimSpace(leaf[:idx])
						}
						orig, ok = dm.GetCell(sheetName, leaf, rowNum)
					}
					if ok {
						trimmed := strings.TrimSpace(strings.TrimPrefix(val, "\ufeff"))
						if trimmed == DeletionMarker {
							rowCells[colIdx] = orig
							restoredCount++
							continue
						}
					}
				}

				rowCells[colIdx] = val
			}

			if err := sw.SetRow(cellCoordinate, rowCells); err != nil {
				_ = rows.Close()
				return restoredCount, fmt.Errorf("failed to write row %d in sheet %s: %w", rowNum, sheetName, err)
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

		// Preserve merged cells from input file
		if srcMerges, err := xl.GetMergeCells(sheetName); err == nil {
			for _, m := range srcMerges {
				_ = sw.MergeCell(m.GetStartAxis(), m.GetEndAxis())
			}
		}

		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		if err := sw.Flush(); err != nil {
			return restoredCount, fmt.Errorf("failed to flush stream writer for sheet %s: %w", sheetName, err)
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

	outFile.SetActiveSheet(0)

	dir := filepath.Dir(outputPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return restoredCount, fmt.Errorf("failed to create output directory %s: %w", dir, err)
		}
	}

	if err := outFile.SaveAs(outputPath); err != nil {
		return restoredCount, fmt.Errorf("failed to save restored excel file to %s: %w", outputPath, err)
	}

	return restoredCount, nil
}
