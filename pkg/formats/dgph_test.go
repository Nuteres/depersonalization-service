package formats

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"github.com/xuri/excelize/v2"
)

func TestInspectDGPH_HeaderBounds(t *testing.T) {
	filePath := "../../ДГПХ_ПАО РТК_015.xlsx"
	if _, err := os.Stat(filePath); err != nil {
		filePath = "../ДГПХ_ПАО РТК_015.xlsx"
		if _, err := os.Stat(filePath); err != nil {
			filePath = "ДГПХ_ПАО РТК_015.xlsx"
			if _, err := os.Stat(filePath); err != nil {
				t.Skip("user file ДГПХ_ПАО РТК_015.xlsx not found")
			}
		}
	}

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer f.Close()

	sheet := "Лист1"
	merges := ExtractMergeRanges(f, sheet)

	rows, err := f.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}

	sampleLimit := 40
	if len(rows) < sampleLimit {
		sampleLimit = len(rows)
	}
	sample := rows[:sampleLimit]

	bounds := DetectHeaderBounds(sample, merges)
	if bounds.LeafHeaderIdx != 21 {
		t.Errorf("expected LeafHeaderIdx = 21, got %d", bounds.LeafHeaderIdx)
	}
	if bounds.DataStartIdx != 23 {
		t.Errorf("expected DataStartIdx = 23, got %d", bounds.DataStartIdx)
	}

	headers := BuildHierarchicalHeaders(sample, bounds.LeafHeaderIdx, merges)
	if len(headers) != 27 {
		t.Fatalf("expected 27 columns, got %d", len(headers))
	}

	sampleData := sample[bounds.DataStartIdx:]
	colTypes := ResolveColumnTypesWithSamples(headers, sampleData, nil)

	// FIO columns
	if colTypes[1] != ColFIO {
		t.Errorf("expected Col 1 [%s] to be ColFIO, got %s", headers[1], colTypes[1])
	}
	if colTypes[21] != ColFIO {
		t.Errorf("expected Col 21 [%s] to be ColFIO, got %s", headers[21], colTypes[21])
	}
	if colTypes[25] != ColFIO {
		t.Errorf("expected Col 25 [%s] to be ColFIO, got %s", headers[25], colTypes[25])
	}

	// Email column
	if colTypes[20] != ColEmail {
		t.Errorf("expected Col 20 [%s] to be ColEmail, got %s", headers[20], colTypes[20])
	}

	// Dates
	dateCols := []int{4, 5, 6, 7, 15, 16, 19}
	for _, cIdx := range dateCols {
		if colTypes[cIdx] != ColDate {
			t.Errorf("expected Col %d [%s] to be ColDate, got %s", cIdx, headers[cIdx], colTypes[cIdx])
		}
	}

	// Columns that must NOT be FIO
	nonFioCols := []int{2, 3, 23, 24, 26}
	for _, cIdx := range nonFioCols {
		if colTypes[cIdx] == ColFIO {
			t.Errorf("expected Col %d [%s] NOT to be ColFIO, got %s", cIdx, headers[cIdx], colTypes[cIdx])
		}
	}
}

func TestDetectHeaderBounds_SyntheticNumberedTable(t *testing.T) {
	// Build a synthetic workbook replicating the GOST / 1C / SAP multi-tier structure:
	// Row 1 (idx 0): Preamble
	// Row 2 (idx 1): Top Category header with horizontal merge B2:D2
	// Row 3 (idx 2): Leaf headers with vertical merge A2:A3
	// Row 4 (idx 3): Sequential numbering row 1..4
	// Row 5 (idx 4): First data row
	fx := excelize.NewFile()
	sheet := "Sheet1"

	_ = fx.SetCellValue(sheet, "A1", "Отчёт по договорам ГПХ")
	_ = fx.SetCellValue(sheet, "A2", "Исполнитель")
	_ = fx.SetCellValue(sheet, "B2", "Сведения о договоре")
	_ = fx.MergeCell(sheet, "B2", "D2")
	_ = fx.MergeCell(sheet, "A2", "A3")

	_ = fx.SetCellValue(sheet, "B3", "Дата начала")
	_ = fx.SetCellValue(sheet, "C3", "Дата окончания")
	_ = fx.SetCellValue(sheet, "D3", "Номер договора")

	// Row 4: sequential numbering 1..4
	for c := 1; c <= 4; c++ {
		cell, _ := excelize.CoordinatesToCellName(c, 4)
		_ = fx.SetCellValue(sheet, cell, fmt.Sprintf("%d", c))
	}

	// Row 5: data
	_ = fx.SetCellValue(sheet, "A5", "Иванова Ирина Анатольевна")
	_ = fx.SetCellValue(sheet, "B5", "01.01.2026")
	_ = fx.SetCellValue(sheet, "C5", "31.12.2026")
	_ = fx.SetCellValue(sheet, "D5", "587684-4")

	merges := ExtractMergeRanges(fx, sheet)
	rows, err := fx.GetRows(sheet)
	if err != nil {
		t.Fatal(err)
	}

	bounds := DetectHeaderBounds(rows, merges)
	if bounds.HeaderStartIdx != 1 {
		t.Errorf("expected HeaderStartIdx = 1, got %d", bounds.HeaderStartIdx)
	}
	if bounds.LeafHeaderIdx != 2 {
		t.Errorf("expected LeafHeaderIdx = 2, got %d", bounds.LeafHeaderIdx)
	}
	if bounds.DataStartIdx != 4 {
		t.Errorf("expected DataStartIdx = 4 (skipping row 3 numbering), got %d", bounds.DataStartIdx)
	}

	headers := BuildHierarchicalHeaders(rows, bounds.LeafHeaderIdx, merges)
	expectedHeaders := []string{
		"Исполнитель",
		"Сведения о договоре / Дата начала",
		"Сведения о договоре / Дата окончания",
		"Сведения о договоре / Номер договора",
	}
	for i, exp := range expectedHeaders {
		if i >= len(headers) || headers[i] != exp {
			t.Errorf("col %d: expected %q, got %q", i, exp, headers[i])
		}
	}

	colTypes := ResolveColumnTypesWithSamples(headers, rows[bounds.DataStartIdx:], nil)
	if colTypes[0] != ColFIO {
		t.Errorf("expected Col 0 to be ColFIO, got %s", colTypes[0])
	}
	if colTypes[1] != ColDate {
		t.Errorf("expected Col 1 to be ColDate, got %s", colTypes[1])
	}
	if colTypes[2] != ColDate {
		t.Errorf("expected Col 2 to be ColDate, got %s", colTypes[2])
	}
	if colTypes[3] != ColUnknown {
		t.Errorf("expected Col 3 to be ColUnknown, got %s", colTypes[3])
	}

	// Anonymization test
	anon := anonymizer.New("test-salt-dgph")
	handler := NewExcelHandler(anon)
	buf, _ := fx.WriteToBuffer()
	fx.Close()

	var out bytes.Buffer
	dm := depmap.New("test-salt-dgph")
	err = handler.Process(context.Background(), bytes.NewReader(buf.Bytes()), &out, map[string]interface{}{
		"depmap": dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	resFile, err := excelize.OpenReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	defer resFile.Close()

	// Preamble preserved
	vA1, _ := resFile.GetCellValue(sheet, "A1")
	if vA1 != "Отчёт по договорам ГПХ" {
		t.Errorf("expected preamble A1 preserved, got %q", vA1)
	}
	// Category preserved
	vB2, _ := resFile.GetCellValue(sheet, "B2")
	if vB2 != "Сведения о договоре" {
		t.Errorf("expected category B2 preserved, got %q", vB2)
	}
	// Numbering row preserved
	vA4, _ := resFile.GetCellValue(sheet, "A4")
	if vA4 != "1" {
		t.Errorf("expected numbering row cell A4='1' preserved, got %q", vA4)
	}
	// Data anonymized
	vA5, _ := resFile.GetCellValue(sheet, "A5")
	if vA5 == "Иванова Ирина Анатольевна" || vA5 == "" {
		t.Errorf("expected anonymized FIO in A5, got %q", vA5)
	}
}
