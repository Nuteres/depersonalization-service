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
		dim, err := xl.GetSheetDimension(sheet)
		if err == nil && dim != "" {
			parts := strings.Split(dim, ":")
			lastCell := parts[len(parts)-1]
			_, r, err := excelize.SplitCellName(lastCell)
			if err == nil && r > 0 {
				totalRows += r
				continue
			}
		}
		rIter, err := xl.Rows(sheet)
		if err == nil {
			for rIter.Next() {
				totalRows++
			}
			_ = rIter.Close()
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
		pct := 100
		if total > 0 {
			pct = (procRows * 100) / total
			if pct > 100 {
				pct = 100
			}
		}
		if force || pct != lastReportedPct {
			lastReportedPct = pct
			onProgress(pct, procRows, total)
		}
	}

	type cellChange struct {
		cell string
		val  string
	}

	for _, sheetName := range sheetList {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
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

			for colIdx, val := range cols {
				if len(val) == 0 {
					continue
				}

				if orig, ok := dm.Get(val); ok {
					coord, err := excelize.CoordinatesToCellName(colIdx+1, rowNum)
					if err == nil {
						changes = append(changes, cellChange{cell: coord, val: orig})
					}
				} else {
					trimmed := strings.TrimSpace(val)
					trimmed = strings.TrimPrefix(trimmed, "\ufeff")
					if trimmed != val {
						if orig, ok := dm.Get(trimmed); ok {
							coord, err := excelize.CoordinatesToCellName(colIdx+1, rowNum)
							if err == nil {
								changes = append(changes, cellChange{cell: coord, val: orig})
							}
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

	// Final progress callback if not reached 100%
	if onProgress != nil && lastReportedPct < 100 {
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
