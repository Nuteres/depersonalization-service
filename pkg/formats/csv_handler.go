package formats

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"dep_service/pkg/anonymizer"
)

// CSVHandler implements FormatHandler for CSV and TSV files using buffered I/O.
type CSVHandler struct {
	Anon *anonymizer.Anonymizer
}

// NewCSVHandler creates a new CSVHandler with the given Anonymizer.
func NewCSVHandler(anon *anonymizer.Anonymizer) *CSVHandler {
	return &CSVHandler{Anon: anon}
}

// SupportedExtensions returns file extensions supported by this handler.
func (h *CSVHandler) SupportedExtensions() []string {
	return []string{".csv", ".tsv"}
}

// Process reads CSV rows from r, detects PII columns from the header row,
// anonymizes identified columns using streaming, and writes the result to w.
func (h *CSVHandler) Process(ctx context.Context, r io.Reader, w io.Writer, options map[string]interface{}) error {
	if h.Anon == nil {
		return fmt.Errorf("anonymizer is not initialized")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	bufReader := bufio.NewReader(r)

	// Determine delimiter
	comma := ','
	hasExplicitDelimiter := false
	if options != nil {
		if delim, ok := options["delimiter"].(rune); ok {
			comma = delim
			hasExplicitDelimiter = true
		} else if delimStr, ok := options["delimiter"].(string); ok && len(delimStr) > 0 {
			comma = []rune(delimStr)[0]
			hasExplicitDelimiter = true
		} else if c, ok := options["comma"].(rune); ok {
			comma = c
			hasExplicitDelimiter = true
		}
	}

	// Auto-detect delimiter if not explicitly specified (e.g. ';' or '\t')
	if !hasExplicitDelimiter {
		peekBytes, err := bufReader.Peek(2048)
		if err == nil || errors.Is(err, io.EOF) {
			peekStr := strings.TrimPrefix(string(peekBytes), "\ufeff")
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
		}
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

	dateRange := getDateRange(options)

	var progressFn func(int)
	if cb, ok := options["progress_callback"].(func(int)); ok {
		progressFn = cb
	}

	// Buffer up to 40 leading rows to locate the header row (supporting leading metadata/preambles)
	var initialRows [][]string
	for len(initialRows) < 40 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rec, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("failed to read csv rows: %w", err)
		}
		initialRows = append(initialRows, rec)
	}

	if len(initialRows) == 0 {
		return nil
	}

	headerIdx, headers := FindHeaderRow(initialRows)
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\ufeff")
	}
	colTypes := ResolveColumnTypes(headers, options)

	// Write any preamble rows before the header
	for i := 0; i < headerIdx; i++ {
		if err := csvWriter.Write(initialRows[i]); err != nil {
			return fmt.Errorf("failed to write preamble row %d: %w", i, err)
		}
	}

	// Write header row
	if err := csvWriter.Write(headers); err != nil {
		return fmt.Errorf("failed to write header row: %w", err)
	}

	rowCount := 0

	// Write already-buffered data rows
	for i := headerIdx + 1; i < len(initialRows); i++ {
		record := initialRows[i]
		outRow := make([]string, len(record))
		for j, val := range record {
			var ct ColumnType
			if j < len(colTypes) {
				ct = colTypes[j]
			}
			outRow[j] = AnonymizeCell(h.Anon, ct, val, dateRange)
		}
		if err := csvWriter.Write(outRow); err != nil {
			return fmt.Errorf("failed writing csv row: %w", err)
		}
		rowCount++
		if progressFn != nil && rowCount%50 == 0 {
			progressFn(rowCount)
		}
	}

	// Stream remaining data rows from file
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		record, err := csvReader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("failed reading csv row: %w", err)
		}

		outRow := make([]string, len(record))
		for i, val := range record {
			var colType ColumnType
			if i < len(colTypes) {
				colType = colTypes[i]
			}
			outRow[i] = AnonymizeCell(h.Anon, colType, val, dateRange)
		}

		if err := csvWriter.Write(outRow); err != nil {
			return fmt.Errorf("failed writing csv row: %w", err)
		}

		rowCount++
		if progressFn != nil && rowCount%50 == 0 {
			progressFn(rowCount)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("csv writer flush error: %w", err)
	}
	if err := bufWriter.Flush(); err != nil {
		return fmt.Errorf("bufio writer flush error: %w", err)
	}

	return nil
}
