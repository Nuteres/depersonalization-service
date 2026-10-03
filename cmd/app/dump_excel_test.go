package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	"dep_service/internal/service/inspector"
	"github.com/xuri/excelize/v2"
)

func TestDumpExcel(t *testing.T) {
	path := "../../ШК_ПАО РТК_В2С.xlsx"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample excel file not found in root")
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("Error opening excel: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	fmt.Printf("Found %d sheets: %v\n", len(sheets), sheets)

	for _, sheet := range sheets {
		rows, err := f.GetRows(sheet)
		if err != nil {
			fmt.Printf("Error reading rows: %v\n", err)
			continue
		}
		fmt.Printf("\nTotal rows in %s: %d\n", sheet, len(rows))
		for r := 0; r < len(rows) && r < 12; r++ {
			nonEmpty := 0
			for _, c := range rows[r] {
				if c != "" {
					nonEmpty++
				}
			}
			fmt.Printf("Row %2d (%2d non-empty): %v\n", r+1, nonEmpty, rows[r][:min(5, len(rows[r]))])
		}
	}

	// Now run Inspector.Inspect
	insp := inspector.New()
	info, err := insp.Inspect(context.Background(), path)
	if err != nil {
		t.Fatalf("Inspector error: %v", err)
	}
	fmt.Printf("\n=== Inspector Results ===\n")
	fmt.Printf("File: %s, Sheets: %d, RowCount: %d, Entities: %d\n", info.Name, len(info.Sheets), info.RowCount, len(info.Entities))
	for _, sheet := range info.Sheets {
		fmt.Printf("\nSheet [%s]: %d rows, %d headers, %d entities\n", sheet.Name, sheet.RowCount, len(sheet.Headers), len(sheet.Entities))
		for _, e := range sheet.Entities {
			fmt.Printf("  Col %d [%s] -> Type: %s, Action: %s (%s)\n", e.ColumnIndex, e.ColumnName, e.Type, e.Action, e.Description)
		}
		fmt.Printf("\nHeaders and sample values from Row 11 (index 10) and Row 12 (index 11):\n")
		rows, _ := f.GetRows(sheet.Name)
		headerRow := 9 // row 10 in 1-based is index 9
		for i, h := range sheet.Headers {
			s1, s2 := "", ""
			if len(rows) > headerRow+1 && i < len(rows[headerRow+1]) {
				s1 = rows[headerRow+1][i]
			}
			if len(rows) > headerRow+2 && i < len(rows[headerRow+2]) {
				s2 = rows[headerRow+2][i]
			}
			fmt.Printf("  Col %2d | Header: %-45s | R11: [%s] | R12: [%s]\n", i+1, h, s1, s2)
		}
	}
}
