package formats

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
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

	var explicitDelim rune
	if options != nil {
		if delim, ok := options["delimiter"].(rune); ok {
			explicitDelim = delim
		} else if delimStr, ok := options["delimiter"].(string); ok && len(delimStr) > 0 {
			explicitDelim = []rune(delimStr)[0]
		} else if c, ok := options["comma"].(rune); ok {
			explicitDelim = c
		}
	}

	csvReader, comma, err := NewEncodingAwareCSVReader(r, explicitDelim)
	if err != nil {
		return fmt.Errorf("failed to initialize csv reader: %w", err)
	}

	bufWriter := bufio.NewWriter(w)
	defer bufWriter.Flush()

	// Write UTF-8 BOM if requested (e.g. by desktop processor for spreadsheet compatibility)
	if options != nil {
		if writeBOM, ok := options["write_bom"].(bool); ok && writeBOM {
			if err := WriteUTF8BOM(bufWriter); err != nil {
				return fmt.Errorf("failed to write UTF-8 BOM: %w", err)
			}
		}
	}

	csvWriter := csv.NewWriter(bufWriter)
	csvWriter.Comma = comma
	defer csvWriter.Flush()

	dateRange := getDateRange(options)
	mode := getProcessingMode(options)
	var depMap *depmap.DepMap
	if dm, ok := options["depmap"].(*depmap.DepMap); ok {
		depMap = dm
	} else if dm, ok := options["depMap"].(*depmap.DepMap); ok {
		depMap = dm
	}
	if depMap != nil {
		depMap.Mode = string(mode)
	}

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
	colTypes := ResolveColumnTypesWithSamples(headers, initialRows[headerIdx+1:], options)

	// Build map of disabled columns (1-based index)
	disabledMap := make(map[int]bool)
	if options != nil {
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
	shouldShuffle := isShuffleRows(options)

	if shouldShuffle {
		type pendingCSVRow struct {
			outRow  []string
			origRow []string
		}
		var allDataRows []pendingCSVRow

		// 1. Buffered data rows
		for i := headerIdx + 1; i < len(initialRows); i++ {
			record := initialRows[i]
			outRow := make([]string, len(record))
			for j, val := range record {
				var ct ColumnType
				col1Based := j + 1
				if j < len(colTypes) && !disabledMap[col1Based] {
					ct = colTypes[j]
				}
				newVal := ProcessCell(h.Anon, ct, val, mode, dateRange)
				outRow[j] = newVal

				if ct != ColUnknown && strings.TrimSpace(val) != "" && depMap != nil {
					var headerName string
					if j < len(headers) {
						headerName = headers[j]
					}
					depMap.AddTargetHeader(headerName)
					if mode != domain.ModeDelete {
						depMap.Put(newVal, val)
					}
				}
			}
			allDataRows = append(allDataRows, pendingCSVRow{outRow: outRow, origRow: record})
		}

		// 2. Stream remaining data rows into buffer
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
				col1Based := i + 1
				if i < len(colTypes) && !disabledMap[col1Based] {
					colType = colTypes[i]
				}
				newVal := ProcessCell(h.Anon, colType, val, mode, dateRange)
				outRow[i] = newVal

				if colType != ColUnknown && strings.TrimSpace(val) != "" && depMap != nil {
					var headerName string
					if i < len(headers) {
						headerName = headers[i]
					}
					depMap.AddTargetHeader(headerName)
					if mode != domain.ModeDelete {
						depMap.Put(newVal, val)
					}
				}
			}
			allDataRows = append(allDataRows, pendingCSVRow{outRow: outRow, origRow: record})
		}

		// Shuffle data rows using Fisher-Yates algorithm seeded by salt
		ShuffleRows(h.Anon, allDataRows)

		// Write shuffled data rows
		for idx, pr := range allDataRows {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if err := csvWriter.Write(pr.outRow); err != nil {
				return fmt.Errorf("failed writing csv row: %w", err)
			}

			targetRowNum := headerIdx + 2 + idx
			if depMap != nil && mode == domain.ModeDelete {
				for j, val := range pr.origRow {
					var ct ColumnType
					col1Based := j + 1
					if j < len(colTypes) && !disabledMap[col1Based] {
						ct = colTypes[j]
					}
					if ct != ColUnknown && strings.TrimSpace(val) != "" {
						var headerName string
						if j < len(headers) {
							headerName = headers[j]
						}
						depMap.PutCell("", headerName, targetRowNum, val)
					}
				}
			}

			rowCount++
			if progressFn != nil && rowCount%50 == 0 {
				progressFn(rowCount)
			}
		}

		if progressFn != nil && rowCount%50 != 0 {
			progressFn(rowCount)
		}
	} else {
		// Write already-buffered data rows directly (streaming mode)
		for i := headerIdx + 1; i < len(initialRows); i++ {
			record := initialRows[i]
			physicalRowNum := i + 1
			outRow := make([]string, len(record))
			for j, val := range record {
				var ct ColumnType
				col1Based := j + 1
				if j < len(colTypes) && !disabledMap[col1Based] {
					ct = colTypes[j]
				}
				newVal := ProcessCell(h.Anon, ct, val, mode, dateRange)
				outRow[j] = newVal

				if ct != ColUnknown && strings.TrimSpace(val) != "" && depMap != nil {
					var headerName string
					if j < len(headers) {
						headerName = headers[j]
					}
					depMap.AddTargetHeader(headerName)
					if mode == domain.ModeDelete {
						depMap.PutCell("", headerName, physicalRowNum, val)
					} else {
						depMap.Put(newVal, val)
					}
				}
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
		physicalRowNum := len(initialRows) + 1
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
				col1Based := i + 1
				if i < len(colTypes) && !disabledMap[col1Based] {
					colType = colTypes[i]
				}
				newVal := ProcessCell(h.Anon, colType, val, mode, dateRange)
				outRow[i] = newVal

				if colType != ColUnknown && strings.TrimSpace(val) != "" && depMap != nil {
					var headerName string
					if i < len(headers) {
						headerName = headers[i]
					}
					depMap.AddTargetHeader(headerName)
					if mode == domain.ModeDelete {
						depMap.PutCell("", headerName, physicalRowNum, val)
					} else {
						depMap.Put(newVal, val)
					}
				}
			}

			if err := csvWriter.Write(outRow); err != nil {
				return fmt.Errorf("failed writing csv row: %w", err)
			}

			physicalRowNum++
			rowCount++
			if progressFn != nil && rowCount%50 == 0 {
				progressFn(rowCount)
			}
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
