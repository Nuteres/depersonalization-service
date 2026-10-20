package formats_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
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

func TestRestoreExcel_HeaderAwareColumnShift(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "original.xlsx")
	anonPath := filepath.Join(tmpDir, "anonymized.xlsx")
	aiModifiedPath := filepath.Join(tmpDir, "ai_modified.xlsx")
	restoredPath := filepath.Join(tmpDir, "restored.xlsx")

	// 1. Create original workbook with headers: ID, FIO, Phone
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ID")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "C1", "Телефон")
	_ = f.SetCellValue(sheet, "A2", "1")
	_ = f.SetCellValue(sheet, "B2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet, "C2", "+7 999 123-45-67")
	if err := f.SaveAs(origPath); err != nil {
		t.Fatalf("failed to save original xlsx: %v", err)
	}
	f.Close()

	// 2. Anonymize with ModeDelete and CreateKey=true -> yields anonymized.xlsx and key.depmap
	anon := anonymizer.New("salt-test")
	handler := formats.NewExcelHandler(anon)
	dm := depmap.New("original.xlsx")

	var anonBuf bytes.Buffer
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read original xlsx: %v", err)
	}
	err = handler.Process(context.Background(), bytes.NewReader(origBytes), &anonBuf, map[string]interface{}{
		"mode":   domain.ModeDelete,
		"depmap": dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	if err := os.WriteFile(anonPath, anonBuf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write anonymized xlsx: %v", err)
	}

	// 3. Simulate AI model: Insert 2 new columns between ID and FIO (e.g. AI_Score, AI_Category)
	aiFile := excelize.NewFile()
	aiFile.SetSheetName("Sheet1", sheet)
	_ = aiFile.SetCellValue(sheet, "A1", "ID")
	_ = aiFile.SetCellValue(sheet, "B1", "AI_Score")
	_ = aiFile.SetCellValue(sheet, "C1", "AI_Category")
	_ = aiFile.SetCellValue(sheet, "D1", "ФИО")
	_ = aiFile.SetCellValue(sheet, "E1", "Телефон")

	_ = aiFile.SetCellValue(sheet, "A2", "1")
	_ = aiFile.SetCellValue(sheet, "B2", "0.95")
	_ = aiFile.SetCellValue(sheet, "C2", "VIP")
	_ = aiFile.SetCellValue(sheet, "D2", formats.DeletionMarker)
	_ = aiFile.SetCellValue(sheet, "E2", formats.DeletionMarker)

	if err := aiFile.SaveAs(aiModifiedPath); err != nil {
		t.Fatalf("failed to save ai modified xlsx: %v", err)
	}
	aiFile.Close()

	// 4. Run RestoreExcel on the modified file using key.depmap
	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, aiModifiedPath, restoredPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	// 5. Verify:
	//    - Restore count matches expected (2)
	//    - FIO and Phone are restored to original values
	//    - AI_Score and AI_Category remain intact with AI values
	if restoredCount != 2 {
		t.Errorf("expected 2 restored cells, got %d", restoredCount)
	}

	rf, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	idVal, _ := rf.GetCellValue(sheet, "A2")
	if idVal != "1" {
		t.Errorf("expected ID '1', got %q", idVal)
	}

	aiScoreVal, _ := rf.GetCellValue(sheet, "B2")
	if aiScoreVal != "0.95" {
		t.Errorf("expected AI_Score untouched '0.95', got %q", aiScoreVal)
	}

	aiCatVal, _ := rf.GetCellValue(sheet, "C2")
	if aiCatVal != "VIP" {
		t.Errorf("expected AI_Category untouched 'VIP', got %q", aiCatVal)
	}

	fioVal, _ := rf.GetCellValue(sheet, "D2")
	if fioVal != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", fioVal)
	}

	phoneVal, _ := rf.GetCellValue(sheet, "E2")
	if phoneVal != "+7 999 123-45-67" {
		t.Errorf("expected Phone restored '+7 999 123-45-67', got %q", phoneVal)
	}
}

func TestRestoreCSV_HeaderAwareColumnShift(t *testing.T) {
	origCSV := "ID,ФИО,Телефон\n1,Иванов Иван Иванович,+7 999 123-45-67\n"

	anon := anonymizer.New("salt-test-csv")
	handler := formats.NewCSVHandler(anon)
	dm := depmap.New("clients.csv")

	var anonBuf bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader(origCSV), &anonBuf, map[string]interface{}{
		"mode":   domain.ModeDelete,
		"depmap": dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	// Simulate AI model inserting columns between ID and ФИО
	aiCSV := "ID,AI_Score,AI_Category,ФИО,Телефон\n" +
		"1,0.95,VIP," + formats.DeletionMarker + "," + formats.DeletionMarker + "\n"

	var restoredBuf bytes.Buffer
	ctx := context.Background()
	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(aiCSV), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 2 {
		t.Errorf("expected 2 restored cells, got %d", restoredCount)
	}

	reader := csv.NewReader(&restoredBuf)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored csv: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[1][0] != "1" {
		t.Errorf("expected ID '1', got %q", rows[1][0])
	}
	if rows[1][1] != "0.95" {
		t.Errorf("expected AI_Score untouched '0.95', got %q", rows[1][1])
	}
	if rows[1][2] != "VIP" {
		t.Errorf("expected AI_Category untouched 'VIP', got %q", rows[1][2])
	}
	if rows[1][3] != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", rows[1][3])
	}
	if rows[1][4] != "+7 999 123-45-67" {
		t.Errorf("expected Phone restored '+7 999 123-45-67', got %q", rows[1][4])
	}
}

func TestRestoreExcel_ModeReplace_HeaderAwareAIColumnProtected(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "orig_replace.xlsx")
	aiModifiedPath := filepath.Join(tmpDir, "ai_replace.xlsx")
	restoredPath := filepath.Join(tmpDir, "restored_replace.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ID")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "A2", "1")
	_ = f.SetCellValue(sheet, "B2", "Иванов Иван Иванович")
	if err := f.SaveAs(origPath); err != nil {
		t.Fatalf("failed to save original xlsx: %v", err)
	}
	f.Close()

	anon := anonymizer.New("salt-replace")
	handler := formats.NewExcelHandler(anon)
	dm := depmap.New("orig_replace.xlsx")

	var anonBuf bytes.Buffer
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read original xlsx: %v", err)
	}
	err = handler.Process(context.Background(), bytes.NewReader(origBytes), &anonBuf, map[string]interface{}{
		"mode":   domain.ModeReplace,
		"depmap": dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	anonExcel, err := excelize.OpenReader(&anonBuf)
	if err != nil {
		t.Fatalf("failed to open anon excel: %v", err)
	}
	synthFIO, _ := anonExcel.GetCellValue(sheet, "B2")
	anonExcel.Close()

	// Simulate AI adding column "AI_Comment" that happens to contain the exact synthetic FIO string!
	aiFile := excelize.NewFile()
	aiFile.SetSheetName("Sheet1", sheet)
	_ = aiFile.SetCellValue(sheet, "A1", "ID")
	_ = aiFile.SetCellValue(sheet, "B1", "AI_Comment")
	_ = aiFile.SetCellValue(sheet, "C1", "ФИО")

	_ = aiFile.SetCellValue(sheet, "A2", "1")
	_ = aiFile.SetCellValue(sheet, "B2", synthFIO) // Should NOT be replaced!
	_ = aiFile.SetCellValue(sheet, "C2", synthFIO) // Should be replaced!

	if err := aiFile.SaveAs(aiModifiedPath); err != nil {
		t.Fatalf("failed to save ai modified xlsx: %v", err)
	}
	aiFile.Close()

	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, aiModifiedPath, restoredPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	if restoredCount != 1 {
		t.Errorf("expected exactly 1 restored cell, got %d", restoredCount)
	}

	rf, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	aiCommentVal, _ := rf.GetCellValue(sheet, "B2")
	if aiCommentVal != synthFIO {
		t.Errorf("expected AI_Comment untouched %q, got %q", synthFIO, aiCommentVal)
	}

	fioVal, _ := rf.GetCellValue(sheet, "C2")
	if fioVal != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", fioVal)
	}
}

func TestRestoreExcel_ShuffledRowsAndTokensInSubstrings(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input_shuffled.xlsx")
	outputPath := filepath.Join(tempDir, "output_restored.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Комментарий_LLM")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "C1", "Паспорт")

	_ = f.SetCellValue(sheet, "A2", "Рекомендация: повысить сотрудника [ФИО_0002] на 1 грейд")
	_ = f.SetCellValue(sheet, "B2", "[ФИО_0002]")
	_ = f.SetCellValue(sheet, "C2", "[Паспорт_0002]")

	_ = f.SetCellValue(sheet, "A3", "Рекомендация: премировать [ФИО_0001]")
	_ = f.SetCellValue(sheet, "B3", "[ФИО_0001]")
	_ = f.SetCellValue(sheet, "C3", "[Паспорт_0001]")

	if err := f.SaveAs(inputPath); err != nil {
		t.Fatalf("failed to save input xlsx: %v", err)
	}
	f.Close()

	dm := depmap.New("test.xlsx")
	dm.Mode = "tokens"
	dm.Put("[ФИО_0001]", "Иванов Иван Иванович")
	dm.Put("[Паспорт_0001]", "4508 123456")
	dm.Put("[ФИО_0002]", "Петров Петр Петрович")
	dm.Put("[Паспорт_0002]", "4510 654321")

	count, err := formats.RestoreExcel(context.Background(), inputPath, outputPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if count < 6 {
		t.Errorf("expected at least 6 replacements, got %d", count)
	}

	rf, err := excelize.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	// Row 2 checks (Petrov)
	comment2, _ := rf.GetCellValue(sheet, "A2")
	if comment2 != "Рекомендация: повысить сотрудника Петров Петр Петрович на 1 грейд" {
		t.Errorf("expected substring restored in A2, got %q", comment2)
	}
	fio2, _ := rf.GetCellValue(sheet, "B2")
	if fio2 != "Петров Петр Петрович" {
		t.Errorf("expected B2 restored to Петров, got %q", fio2)
	}
	pass2, _ := rf.GetCellValue(sheet, "C2")
	if pass2 != "4510 654321" {
		t.Errorf("expected C2 restored to 4510 654321, got %q", pass2)
	}

	// Row 3 checks (Ivanov)
	comment3, _ := rf.GetCellValue(sheet, "A3")
	if comment3 != "Рекомендация: премировать Иванов Иван Иванович" {
		t.Errorf("expected substring restored in A3, got %q", comment3)
	}
	fio3, _ := rf.GetCellValue(sheet, "B3")
	if fio3 != "Иванов Иван Иванович" {
		t.Errorf("expected B3 restored to Иванов, got %q", fio3)
	}
	pass3, _ := rf.GetCellValue(sheet, "C3")
	if pass3 != "4508 123456" {
		t.Errorf("expected C3 restored to 4508 123456, got %q", pass3)
	}
}

func TestRestoreExcel_ProgressTracking_StreamWriterDimensionA1(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "stream_writer_dim_a1.xlsx")
	outPath := filepath.Join(tmpDir, "stream_writer_dim_a1_restored.xlsx")

	dm := depmap.New("original.xlsx")
	dm.Put("Синтетика", "Оригинал")

	// Create file using StreamWriter (which leaves Sheet1 dimension as default "A1")
	f := excelize.NewFile()
	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		t.Fatalf("failed to create stream writer: %v", err)
	}
	// Write header + 120 rows
	_ = sw.SetRow("A1", []interface{}{"Колонка1", "Колонка2"})
	for r := 2; r <= 121; r++ {
		coord, _ := excelize.CoordinatesToCellName(1, r)
		_ = sw.SetRow(coord, []interface{}{"Синтетика", r})
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("failed to flush stream writer: %v", err)
	}
	if err := f.SaveAs(inPath); err != nil {
		t.Fatalf("failed to save stream writer file: %v", err)
	}
	_ = f.Close()

	// Verify that Excelize reports dimension without multi-row range (e.g. "A1")
	checkFile, _ := excelize.OpenFile(inPath)
	dim, _ := checkFile.GetSheetDimension("Sheet1")
	_ = checkFile.Close()
	if strings.Contains(dim, ":") {
		t.Logf("note: dimension unexpectedly contains colon: %s", dim)
	}

	var progressEvents []struct{ Pct, Proc, Total int }
	progressFn := func(pct, proc, total int) {
		progressEvents = append(progressEvents, struct{ Pct, Proc, Total int }{pct, proc, total})
	}

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, progressFn)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 120 {
		t.Errorf("expected 120 restored cells, got %d", restored)
	}

	if len(progressEvents) == 0 {
		t.Fatalf("expected progress events to be emitted, got none")
	}

	first := progressEvents[0]
	if first.Total != 121 {
		t.Errorf("expected total rows to be detected as 121, got %d", first.Total)
	}

	last := progressEvents[len(progressEvents)-1]
	if last.Pct != 100 {
		t.Errorf("expected final percentage 100, got %d", last.Pct)
	}
	if last.Total != 121 {
		t.Errorf("expected final total to be 121, got %d", last.Total)
	}
	if last.Proc != 121 {
		t.Errorf("expected final proc to be 121, got %d", last.Proc)
	}
}

func TestRestoreExcel_MultiTierHeaderAndMergesPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_merges.xlsx")
	outPath := filepath.Join(tmpDir, "output_merges.xlsx")

	f := excelize.NewFile()
	sheet := "DataSheet"
	f.SetSheetName("Sheet1", sheet)

	// Merged category header in row 1
	_ = f.SetCellValue(sheet, "B1", "Категория 1")
	_ = f.MergeCell(sheet, "B1", "C1")

	// Leaf headers in row 2
	_ = f.SetCellValue(sheet, "A2", "Участник")
	_ = f.SetCellValue(sheet, "B2", "Балл 1")
	_ = f.SetCellValue(sheet, "C2", "Балл 2")

	// Data in row 3
	_ = f.SetCellValue(sheet, "A3", "[ФИО_0001]")
	_ = f.SetCellValue(sheet, "B3", "5")
	_ = f.SetCellValue(sheet, "C3", "9")

	if err := f.SaveAs(inPath); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dm := depmap.New(inPath)
	dm.Put("[ФИО_0001]", "Сидоров Сидор Сидорович")

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored cell, got %d", restored)
	}

	resFile, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resFile.Close()

	// Verify merged cells were preserved
	merges, err := resFile.GetMergeCells(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(merges) != 1 {
		t.Fatalf("expected 1 merge range, got %d", len(merges))
	}
	if merges[0].GetStartAxis() != "B1" || merges[0].GetEndAxis() != "C1" {
		t.Errorf("expected B1:C1 merge, got %s:%s", merges[0].GetStartAxis(), merges[0].GetEndAxis())
	}

	valA3, _ := resFile.GetCellValue(sheet, "A3")
	if valA3 != "Сидоров Сидор Сидорович" {
		t.Errorf("expected restored FIO, got %q", valA3)
	}
}

func TestRestoreExcel_DeleteMode_LegacyLeafHeaderFallback(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_del.xlsx")
	outPath := filepath.Join(tmpDir, "output_del.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"

	// Merged category header in row 1
	_ = f.SetCellValue(sheet, "A1", "Личные данные")
	_ = f.SetCellValue(sheet, "B1", "Личные данные")
	_ = f.MergeCell(sheet, "A1", "B1")

	// Leaf headers in row 2
	_ = f.SetCellValue(sheet, "A2", "Участник")
	_ = f.SetCellValue(sheet, "B2", "Город")

	// Data in row 3
	_ = f.SetCellValue(sheet, "A3", formats.DeletionMarker)
	_ = f.SetCellValue(sheet, "B3", "Москва")

	if err := f.SaveAs(inPath); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Depmap with legacy leaf header "Участник" instead of "Личные данные / Участник"
	dm := depmap.New(inPath)
	dm.Mode = "delete"
	dm.PutCell(sheet, "Участник", 3, "Иванов Иван Иванович")

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored cell, got %d", restored)
	}

	resFile, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resFile.Close()

	valA3, _ := resFile.GetCellValue(sheet, "A3")
	if valA3 != "Иванов Иван Иванович" {
		t.Errorf("expected restored FIO from legacy leaf header, got %q", valA3)
	}
}




