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

	// Read header row
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	headerRow, err := csvReader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("failed to read csv header: %w", err)
	}

	// Strip optional UTF-8 BOM from first header
	if len(headerRow) > 0 {
		headerRow[0] = strings.TrimPrefix(headerRow[0], "\ufeff")
	}

	// Write header unchanged
	if err := csvWriter.Write(headerRow); err != nil {
		return fmt.Errorf("failed to write csv header: %w", err)
	}

	colTypes := ResolveColumnTypes(headerRow, options)

	// Stream data rows
	outRow := make([]string, len(headerRow))
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

		if len(outRow) != len(record) {
			outRow = make([]string, len(record))
		}

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
