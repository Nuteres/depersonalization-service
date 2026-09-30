package formats

import (
	"bufio"
	"bytes"
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
func RestoreCSV(ctx context.Context, r io.Reader, w io.Writer, dm *depmap.DepMap) (int, error) {
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

	bufReader := bufio.NewReader(r)

	// Delimiter detection and BOM check
	comma := ','
	peekBytes, err := bufReader.Peek(2048)
	if len(peekBytes) == 0 && (err == io.EOF || errors.Is(err, io.EOF)) {
		return 0, nil
	}

	if bytes.HasPrefix(peekBytes, []byte("\xef\xbb\xbf")) {
		_, _ = bufReader.Discard(3)
		peekBytes = peekBytes[3:]
	}

	peekStr := string(peekBytes)
	firstLine := peekStr
	if idx := strings.IndexAny(peekStr, "\r\n"); idx != -1 {
		firstLine = peekStr[:idx]
	}
	semiCount := strings.Count(firstLine, ";")
	tabCount := strings.Count(firstLine, "\t")
	commaCount := strings.Count(firstLine, ",")
	if semiCount > commaCount && semiCount >= tabCount {
		comma = ';'
	} else if tabCount > commaCount && tabCount > semiCount {
		comma = '\t'
	}

	csvReader := csv.NewReader(bufReader)
	csvReader.Comma = comma
	csvReader.FieldsPerRecord = -1
	csvReader.LazyQuotes = true

	bufWriter := bufio.NewWriter(w)
	defer bufWriter.Flush()

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
