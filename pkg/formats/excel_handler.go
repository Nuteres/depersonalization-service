package formats

import (
	"context"
	"fmt"
	"io"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
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

		rows, err := srcFile.Rows(sheetName)
		if err != nil {
			return fmt.Errorf("failed to read rows for sheet %s: %w", sheetName, err)
		}

		var colTypes []ColumnType
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

			if rowNum == 1 {
				// Header row
				colTypes = ResolveColumnTypes(colValues, options)
				headerCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					headerCells[i] = v
				}
				if err := sw.SetRow(cellCoordinate, headerCells); err != nil {
					_ = rows.Close()
					return fmt.Errorf("failed to write header row in sheet %s: %w", sheetName, err)
				}
			} else {
				// Data row
				dataCells := make([]interface{}, len(colValues))
				for i, v := range colValues {
					var colType ColumnType
					if i < len(colTypes) {
						colType = colTypes[i]
					}
					dataCells[i] = AnonymizeCell(h.Anon, colType, v, dateRange)
				}
				if err := sw.SetRow(cellCoordinate, dataCells); err != nil {
					_ = rows.Close()
					return fmt.Errorf("failed to write data row %d in sheet %s: %w", rowNum, sheetName, err)
				}
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
