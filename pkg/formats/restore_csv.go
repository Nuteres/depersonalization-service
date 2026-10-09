package formats

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"dep_service/pkg/depmap"
)

// RestoreCSV reads CSV/TSV from r, replaces any cell matching dm.Mapping with its original value, and writes to w.
// Returns total number of restored cells.
func RestoreCSV(ctx context.Context, r io.Reader, w io.Writer, dm *depmap.DepMap, writeBOM ...bool) (int, error) {
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
	if r == nil {
		return 0, errors.New("reader cannot be nil")
	}
	if w == nil {
		return 0, errors.New("writer cannot be nil")
	}

	csvReader, comma, err := NewEncodingAwareCSVReader(r, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to initialize csv reader: %w", err)
	}

	bufWriter := bufio.NewWriter(w)
	defer bufWriter.Flush()

	// Write UTF-8 BOM if requested (e.g. by restorer service for spreadsheet compatibility)
	if len(writeBOM) > 0 && writeBOM[0] {
		if err := WriteUTF8BOM(bufWriter); err != nil {
			return 0, fmt.Errorf("failed to write UTF-8 BOM: %w", err)
		}
	}

	csvWriter := csv.NewWriter(bufWriter)
	csvWriter.Comma = comma
	defer csvWriter.Flush()

	restoredCount := 0

	var initialRows [][]string
	for len(initialRows) < 40 {
		rec, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, fmt.Errorf("failed reading csv row: %w", err)
		}
		initialRows = append(initialRows, rec)
	}

	if len(initialRows) == 0 {
		return 0, nil
	}

	headerIdx, headers := FindHeaderRow(initialRows)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}

	isDeleteMode := dm.Mode == "delete" || len(dm.CellMappings) > 0
	targetHeadersMap := make(map[string]bool)
	if len(dm.TargetHeaders) > 0 {
		for _, h := range dm.TargetHeaders {
			targetHeadersMap[strings.TrimSpace(h)] = true
		}
	}
	hasHeaderAwareness := isDeleteMode || len(dm.TargetHeaders) > 0

	restoreRecord := func(record []string, physicalRowNum int) []string {
		outRecord := make([]string, len(record))
		for colIdx, cell := range record {
			if len(cell) == 0 {
				outRecord[colIdx] = cell
				continue
			}

			var headerName string
			if colIdx < len(headers) {
				headerName = strings.TrimSpace(headers[colIdx])
			}

			if isDeleteMode {
				if orig, ok := dm.GetCell("", headerName, physicalRowNum); ok {
					trimmed := strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff"))
					if trimmed == DeletionMarker {
						outRecord[colIdx] = orig
						restoredCount++
						continue
					}
				}
				if len(dm.Mapping) > 0 && (len(dm.TargetHeaders) == 0 || targetHeadersMap[headerName]) {
					if orig, ok := dm.Get(cell); ok {
						outRecord[colIdx] = orig
						restoredCount++
						continue
					} else {
						trimmed := strings.TrimSpace(cell)
						trimmed = strings.TrimPrefix(trimmed, "\ufeff")
						if trimmed != cell {
							if orig, ok := dm.Get(trimmed); ok {
								outRecord[colIdx] = orig
								restoredCount++
								continue
							}
						}
					}
				}
				outRecord[colIdx] = cell
			} else if len(dm.TargetHeaders) > 0 {
				if targetHeadersMap[headerName] {
					if orig, ok := dm.Get(cell); ok {
						outRecord[colIdx] = orig
						restoredCount++
					} else {
						trimmed := strings.TrimSpace(cell)
						trimmed = strings.TrimPrefix(trimmed, "\ufeff")
						if trimmed != cell {
							if orig, ok := dm.Get(trimmed); ok {
								outRecord[colIdx] = orig
								restoredCount++
								continue
							}
						}
						outRecord[colIdx] = cell
					}
				} else {
					outRecord[colIdx] = cell
				}
			} else {
				if orig, ok := dm.Get(cell); ok {
					outRecord[colIdx] = orig
					restoredCount++
				} else {
					trimmed := strings.TrimSpace(cell)
					trimmed = strings.TrimPrefix(trimmed, "\ufeff")
					if trimmed != cell {
						if orig, ok := dm.Get(trimmed); ok {
							outRecord[colIdx] = orig
							restoredCount++
							continue
						}
					}
					outRecord[colIdx] = cell
				}
			}
		}
		return outRecord
	}

	for i, record := range initialRows {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		physicalRowNum := i + 1
		if hasHeaderAwareness && len(initialRows) > 1 && headerIdx >= 0 && physicalRowNum <= headerIdx+1 {
			if err := csvWriter.Write(record); err != nil {
				return restoredCount, fmt.Errorf("failed writing csv header row: %w", err)
			}
			continue
		}

		outRecord := restoreRecord(record, physicalRowNum)
		if err := csvWriter.Write(outRecord); err != nil {
			return restoredCount, fmt.Errorf("failed writing csv row: %w", err)
		}
	}

	physicalRowNum := len(initialRows) + 1
	for {
		select {
		case <-ctx.Done():
			return restoredCount, ctx.Err()
		default:
		}

		record, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return restoredCount, fmt.Errorf("failed reading csv row: %w", err)
		}

		outRecord := restoreRecord(record, physicalRowNum)
		if err := csvWriter.Write(outRecord); err != nil {
			return restoredCount, fmt.Errorf("failed writing csv row: %w", err)
		}
		physicalRowNum++
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return restoredCount, fmt.Errorf("csv writer flush error: %w", err)
	}

	if err := bufWriter.Flush(); err != nil {
		return restoredCount, fmt.Errorf("bufio writer flush error: %w", err)
	}

	return restoredCount, nil
}
