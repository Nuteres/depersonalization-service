package formats_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
)

func TestRestoreCSV(t *testing.T) {
	dm := depmap.New("clients.csv")
	dm.Put("Смирнов Алексей Петрович", "Иванов Иван Иванович")
	dm.Put("452-190-883 12", "112-233-445 95")
	dm.Put("Кузнецова Ольга Ивановна", "Петрова Анна Сергеевна")
	dm.Put("987-654-321 00", "223-344-556 01")

	inputCSV := "ФИО,СНИЛС,Score_ML,Category\n" +
		"Смирнов Алексей Петрович,452-190-883 12,0.95,VIP\n" +
		"Кузнецова Ольга Ивановна,987-654-321 00,0.42,Standard\n" +
		"Сидоров Петр Алексеевич,000-111-222 33,0.15,New\n"

	var out bytes.Buffer
	ctx := context.Background()

	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(inputCSV), &out, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 4 {
		t.Errorf("expected 4 restored cells, got %d", restoredCount)
	}

	reader := csv.NewReader(&out)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored csv: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Verify headers preserved intact
	expectedHeaders := []string{"ФИО", "СНИЛС", "Score_ML", "Category"}
	for i, h := range expectedHeaders {
		if rows[0][i] != h {
			t.Errorf("header col %d mismatch: expected %q, got %q", i, h, rows[0][i])
		}
	}

	// Row 1: restored FIO and SNILS, preserved Score_ML and Category
	if rows[1][0] != "Иванов Иван Иванович" {
		t.Errorf("row 1 FIO mismatch: expected 'Иванов Иван Иванович', got %q", rows[1][0])
	}
	if rows[1][1] != "112-233-445 95" {
		t.Errorf("row 1 SNILS mismatch: expected '112-233-445 95', got %q", rows[1][1])
	}
	if rows[1][2] != "0.95" {
		t.Errorf("row 1 Score_ML mismatch: expected '0.95', got %q", rows[1][2])
	}
	if rows[1][3] != "VIP" {
		t.Errorf("row 1 Category mismatch: expected 'VIP', got %q", rows[1][3])
	}

	// Row 2: restored FIO and SNILS
	if rows[2][0] != "Петрова Анна Сергеевна" {
		t.Errorf("row 2 FIO mismatch: expected 'Петрова Анна Сергеевна', got %q", rows[2][0])
	}
	if rows[2][1] != "223-344-556 01" {
		t.Errorf("row 2 SNILS mismatch: expected '223-344-556 01', got %q", rows[2][1])
	}
	if rows[2][2] != "0.42" {
		t.Errorf("row 2 Score_ML mismatch: expected '0.42', got %q", rows[2][2])
	}
	if rows[2][3] != "Standard" {
		t.Errorf("row 2 Category mismatch: expected 'Standard', got %q", rows[2][3])
	}

	// Row 3: unmapped values remain unchanged
	if rows[3][0] != "Сидоров Петр Алексеевич" {
		t.Errorf("row 3 FIO unmapped mismatch: expected 'Сидоров Петр Алексеевич', got %q", rows[3][0])
	}
	if rows[3][1] != "000-111-222 33" {
		t.Errorf("row 3 SNILS unmapped mismatch: expected '000-111-222 33', got %q", rows[3][1])
	}
	if rows[3][2] != "0.15" {
		t.Errorf("row 3 Score_ML mismatch: expected '0.15', got %q", rows[3][2])
	}
	if rows[3][3] != "New" {
		t.Errorf("row 3 Category mismatch: expected 'New', got %q", rows[3][3])
	}
}

func TestRestoreCSV_SemicolonAndBOM(t *testing.T) {
	dm := depmap.New("semicolon.csv")
	dm.Put("Синтетика Один", "Оригинал Один")

	// UTF-8 BOM + semicolon-separated CSV
	inputCSV := "\ufeffФИО;Метрика\nСинтетика Один;100\n"

	var out bytes.Buffer
	ctx := context.Background()

	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(inputCSV), &out, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 1 {
		t.Errorf("expected 1 restored cell, got %d", restoredCount)
	}

	reader := csv.NewReader(&out)
	reader.Comma = ';'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored semicolon csv: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[1][0] != "Оригинал Один" {
		t.Errorf("expected 'Оригинал Один', got %q", rows[1][0])
	}
	if rows[1][1] != "100" {
		t.Errorf("expected '100', got %q", rows[1][1])
	}
}

func TestRestoreExcel(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "scored_pipeline.xlsx")
	outputPath := filepath.Join(tmpDir, "restored_pipeline.xlsx")

	dm := depmap.New("original.xlsx")
	dm.Put("Васильев Василий Васильевич", "Иванов Иван Иванович")
	dm.Put("452-190-883 12", "112-233-445 95")
	dm.Put("+7 (999) 000-11-22", "+7 (916) 123-45-67")

	// Create test workbook with 2 sheets
	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Clients"
	f.SetSheetName("Sheet1", sheet1)

	// Sheet 1: has synthetic FIO and SNILS, plus ML pipeline added columns: Score, Segment, Status
	headers1 := []string{"ФИО", "СНИЛС", "ML_Score", "Segment", "Model_Status"}
	for i, h := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet1, cell, h)
	}

	row1 := []interface{}{"Васильев Василий Васильевич", "452-190-883 12", 0.88, "A", "APPROVED"}
	for i, v := range row1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	row2 := []interface{}{"Неизвестный Кандидат", "111-222-333 44", 0.12, "C", "REJECTED"}
	for i, v := range row2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	// Sheet 2: "Contacts"
	sheet2 := "Contacts"
	_, _ = f.NewSheet(sheet2)
	headers2 := []string{"Телефон", "Email_Verified", "Priority"}
	for i, h := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet2, cell, h)
	}

	sheet2Row := []interface{}{"+7 (999) 000-11-22", true, 10}
	for i, v := range sheet2Row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet2, cell, v)
	}

	if err := f.SaveAs(inputPath); err != nil {
		t.Fatalf("failed to save initial xlsx: %v", err)
	}

	var progressCalled int32
	var lastPercent int
	progressFn := func(pct int, proc, total int) {
		atomic.AddInt32(&progressCalled, 1)
		lastPercent = pct
		if proc > total && total > 0 {
			t.Errorf("progress proc (%d) > total (%d)", proc, total)
		}
	}

	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, inputPath, outputPath, dm, progressFn)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	// Expect 3 restored cells: 1 FIO, 1 SNILS on Sheet1, 1 Phone on Sheet2
	if restoredCount != 3 {
		t.Errorf("expected 3 restored cells, got %d", restoredCount)
	}

	if atomic.LoadInt32(&progressCalled) == 0 {
		t.Errorf("expected progress callback to be invoked at least once")
	}
	if lastPercent != 100 {
		t.Errorf("expected final progress percentage to be 100, got %d", lastPercent)
	}

	// Read and verify restored workbook
	rf, err := excelize.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx file: %v", err)
	}
	defer rf.Close()

	// Verify Sheet 1
	rows1, err := rf.GetRows(sheet1)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet1, err)
	}
	if len(rows1) < 3 {
		t.Fatalf("expected at least 3 rows in %s, got %d", sheet1, len(rows1))
	}

	// Header check
	for i, h := range headers1 {
		if rows1[0][i] != h {
			t.Errorf("sheet1 header col %d mismatch: expected %q, got %q", i, h, rows1[0][i])
		}
	}

	// Row 1 checks
	if rows1[1][0] != "Иванов Иван Иванович" {
		t.Errorf("row 1 FIO expected 'Иванов Иван Иванович', got %q", rows1[1][0])
	}
	if rows1[1][1] != "112-233-445 95" {
		t.Errorf("row 1 SNILS expected '112-233-445 95', got %q", rows1[1][1])
	}
	if rows1[1][2] != "0.88" {
		t.Errorf("row 1 ML_Score expected '0.88', got %q", rows1[1][2])
	}
	if rows1[1][3] != "A" {
		t.Errorf("row 1 Segment expected 'A', got %q", rows1[1][3])
	}
	if rows1[1][4] != "APPROVED" {
		t.Errorf("row 1 Model_Status expected 'APPROVED', got %q", rows1[1][4])
	}

	// Row 2 checks (unmapped values intact)
	if rows1[2][0] != "Неизвестный Кандидат" {
		t.Errorf("row 2 unmapped FIO expected 'Неизвестный Кандидат', got %q", rows1[2][0])
	}
	if rows1[2][1] != "111-222-333 44" {
		t.Errorf("row 2 unmapped SNILS expected '111-222-333 44', got %q", rows1[2][1])
	}

	// Verify Sheet 2
	rows2, err := rf.GetRows(sheet2)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet2, err)
	}
	if len(rows2) < 2 {
		t.Fatalf("expected at least 2 rows in %s, got %d", sheet2, len(rows2))
	}
	if rows2[1][0] != "+7 (916) 123-45-67" {
		t.Errorf("sheet2 row 1 Phone expected '+7 (916) 123-45-67', got %q", rows2[1][0])
	}
	if rows2[1][1] != "TRUE" && rows2[1][1] != "true" && rows2[1][1] != "1" {
		t.Errorf("sheet2 row 1 Email_Verified preserved boolean, got %q", rows2[1][1])
	}
	if rows2[1][2] != "10" {
		t.Errorf("sheet2 row 1 Priority expected '10', got %q", rows2[1][2])
	}
}

func TestRestore_ContextCancelled(t *testing.T) {
	dm := depmap.New("test.csv")
	dm.Put("A", "B")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// CSV context cancellation
	var out bytes.Buffer
	_, err := formats.RestoreCSV(ctx, strings.NewReader("A,C\nA,C\n"), &out, dm)
	if err == nil {
		t.Fatal("expected error on cancelled context in RestoreCSV, got nil")
	}

	// Excel context cancellation
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "cancel.xlsx")
	outXlsx := filepath.Join(tmpDir, "out_cancel.xlsx")

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "A")
	_ = f.SaveAs(xlsxPath)
	f.Close()

	_, err = formats.RestoreExcel(ctx, xlsxPath, outXlsx, dm, nil)
	if err == nil {
		t.Fatal("expected error on cancelled context in RestoreExcel, got nil")
	}
}

func TestRestore_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// Nil depmap
	var buf bytes.Buffer
	_, err := formats.RestoreCSV(ctx, strings.NewReader("A,B"), &buf, nil)
	if err == nil {
		t.Error("expected error for nil depmap in RestoreCSV")
	}

	_, err = formats.RestoreExcel(ctx, "a.xlsx", "b.xlsx", nil, nil)
	if err == nil {
		t.Error("expected error for nil depmap in RestoreExcel")
	}

	dm := depmap.New("empty.csv")

	// Empty input CSV
	buf.Reset()
	count, err := formats.RestoreCSV(ctx, strings.NewReader(""), &buf, dm)
	if err != nil {
		t.Errorf("unexpected error for empty CSV: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 restored cells for empty CSV, got %d", count)
	}

	// Non-existent Excel file
	_, err = formats.RestoreExcel(ctx, "/path/does/not/exist/at/all.xlsx", "out.xlsx", dm, nil)
	if err == nil {
		t.Error("expected error for non-existent excel file")
	}
}
