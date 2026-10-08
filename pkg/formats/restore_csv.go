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

		outRecord := make([]string, len(record))
		for i, cell := range record {
			if len(cell) == 0 {
				outRecord[i] = cell
				continue
			}

			if orig, ok := dm.Get(cell); ok {
				outRecord[i] = orig
				restoredCount++
			} else {
				trimmed := strings.TrimSpace(cell)
				trimmed = strings.TrimPrefix(trimmed, "\ufeff")
				if trimmed != cell {
					if orig, ok := dm.Get(trimmed); ok {
						outRecord[i] = orig
						restoredCount++
						continue
					}
				}
				outRecord[i] = cell
			}
		}

		if err := csvWriter.Write(outRecord); err != nil {
			return restoredCount, fmt.Errorf("failed writing csv row: %w", err)
		}
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
