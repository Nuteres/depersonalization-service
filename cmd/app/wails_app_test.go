package main

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/desktop"
	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
	"dep_service/internal/version"
	"dep_service/pkg/depmap"
)

type (
	FileInfo      = domain.FileInfo
	SheetInfo     = domain.SheetInfo
	EntityInfo    = domain.EntityInfo
	PreviewRow    = domain.PreviewRow
	ProcessResult = domain.ProcessResult
	KeyFileInfo   = domain.KeyFileInfo
	RestoreResult = domain.RestoreResult
)

func NewApp() *desktop.DesktopApp {
	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km, insp)
	rest := restorer.New(km)
	return desktop.New(insp, prev, proc, rest, km)
}

func TestApp_InspectCSV(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_clients.csv")

	content := "ФИО,СНИЛС,Паспорт,Телефон,Сумма\n" +
		"Иванов Иван Иванович,112-233-445 95,4510 123890,+7 (916) 123-45-67,1000\n" +
		"Смирнова Ольга Алексеевна,001-234-567 89,4608 554312,+7 (903) 765-43-21,2500\n"

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	app := NewApp()
	info, err := app.InspectFile(csvPath)
	if err != nil {
		t.Fatalf("InspectFile failed: %v", err)
	}

	if info.FileType != "table" {
		t.Errorf("expected FileType table, got %s", info.FileType)
	}
	if info.RowCount != 2 {
		t.Errorf("expected RowCount 2, got %d", info.RowCount)
	}
	if len(info.Headers) != 5 {
		t.Errorf("expected 5 headers, got %d", len(info.Headers))
	}
	if len(info.Entities) < 4 {
		t.Errorf("expected at least 4 entities detected, got %d", len(info.Entities))
	}
	if len(info.PreviewRows) != 2 {
		t.Fatalf("expected 2 preview rows, got %d", len(info.PreviewRows))
	}

	// Verify before vs after values differ for FIO
	row0 := info.PreviewRows[0]
	if row0.Before[0] == row0.After[0] {
		t.Errorf("expected FIO before and after to differ: %s vs %s", row0.Before[0], row0.After[0])
	}
	// Verify sum column is untouched
	if row0.Before[4] != row0.After[4] {
		t.Errorf("expected sum column to be identical, got %s vs %s", row0.Before[4], row0.After[4])
	}
}

func TestApp_ProcessCSV(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "clients.csv")

	content := "ФИО,СНИЛС,Телефон\n" +
		"Иванов Иван Иванович,112-233-445 95,+7 (916) 123-45-67\n"

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	app := NewApp()
	res, err := app.ProcessFile(csvPath, "salt-unit-test")
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected Success=true, got false: %s", res.Error)
	}
	if !strings.HasSuffix(res.OutputPath, "clients_anonymized.csv") {
		t.Errorf("unexpected output path: %s", res.OutputPath)
	}

	outBytes, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	outStr := string(outBytes)
	if strings.Contains(outStr, "Иванов Иван Иванович") {
		t.Errorf("output still contains original FIO")
	}
	if strings.Contains(outStr, "112-233-445 95") {
		t.Errorf("output still contains original SNILS")
	}
}

func TestApp_InspectAndProcessXLSX(t *testing.T) {
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "data.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ФИО")
	_ = f.SetCellValue(sheet, "B1", "СНИЛС")
	_ = f.SetCellValue(sheet, "C1", "Паспорт")

	_ = f.SetCellValue(sheet, "A2", "Петров Петр Петрович")
	_ = f.SetCellValue(sheet, "B2", "112-233-445 95")
	_ = f.SetCellValue(sheet, "C2", "4510 123890")

	if err := f.SaveAs(xlsxPath); err != nil {
		t.Fatalf("failed to create xlsx: %v", err)
	}

	app := NewApp()
	info, err := app.InspectFile(xlsxPath)
	if err != nil {
		t.Fatalf("InspectFile on xlsx failed: %v", err)
	}

	if info.FileType != "table" {
		t.Errorf("expected table, got %s", info.FileType)
	}
	if info.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", info.RowCount)
	}
	if len(info.Entities) < 3 {
		t.Errorf("expected at least 3 entities, got %d", len(info.Entities))
	}

	res, err := app.ProcessFile(xlsxPath, "xlsx-salt")
	if err != nil {
		t.Fatalf("ProcessFile on xlsx failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected ProcessFile success")
	}

	// Verify output xlsx
	outXl, err := excelize.OpenFile(res.OutputPath)
	if err != nil {
		t.Fatalf("failed to open output xlsx: %v", err)
	}
	defer outXl.Close()

	valA2, _ := outXl.GetCellValue(sheet, "A2")
	if valA2 == "Петров Петр Петрович" || valA2 == "" {
		t.Errorf("unexpected anonymized FIO: %s", valA2)
	}
}

func TestApp_InspectAndProcessPDF(t *testing.T) {
	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "doc.pdf")

	// Create a minimal valid uncompressed PDF with sensitive text
	streamContent := "BT /F1 12 Tf 72 712 Td (СНИЛС 112-233-445 95 Паспорт 4510 123890) Tj ET"
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n")
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(streamContent), streamContent))
	buf.WriteString("xref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000210 00000 n \n")
	buf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n350\n%%EOF\n")

	if err := os.WriteFile(pdfPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test pdf: %v", err)
	}

	app := NewApp()
	info, err := app.InspectFile(pdfPath)
	if err != nil {
		t.Fatalf("InspectFile on pdf failed: %v", err)
	}

	if info.FileType != "pdf" {
		t.Errorf("expected FileType pdf, got %s", info.FileType)
	}
	if len(info.Entities) < 2 {
		t.Errorf("expected at least 2 entities detected in pdf, got %d", len(info.Entities))
	}
	if !strings.Contains(info.PDFRedactedSnippet, "[ ПДн СКРЫТО ]") {
		t.Errorf("expected redacted marker in snippet, got: %s", info.PDFRedactedSnippet)
	}

	res, err := app.ProcessFile(pdfPath, "pdf-salt")
	if err != nil {
		t.Fatalf("ProcessFile on pdf failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected ProcessFile success on pdf")
	}

	outPdf, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("failed to read output pdf: %v", err)
	}
	if strings.Contains(string(outPdf), "112-233-445 95") {
		t.Errorf("output pdf still contains SNILS")
	}
	if !strings.Contains(string(outPdf), "[ ПДн СКРЫТО ]") {
		t.Errorf("output pdf missing blackout marker")
	}
}

func TestApp_FlatePDF_Inspection(t *testing.T) {
	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "flate.pdf")

	rawContent := "BT /F1 12 Tf 72 712 Td (+7 916 123-45-67 112-233-445 95) Tj ET"
	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	_, _ = zw.Write([]byte(rawContent))
	_ = zw.Close()

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	buf.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n")
	buf.WriteString(fmt.Sprintf("4 0 obj\n<< /Length %d /Filter /FlateDecode >>\nstream\n", compressed.Len()))
	buf.Write(compressed.Bytes())
	buf.WriteString("\nendstream\nendobj\n")
	buf.WriteString("trailer\n<< /Size 5 /Root 1 0 R >>\n%%EOF\n")

	if err := os.WriteFile(pdfPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write flate pdf: %v", err)
	}

	app := NewApp()
	info, err := app.InspectFile(pdfPath)
	if err != nil {
		t.Fatalf("failed to inspect flate pdf: %v", err)
	}

	if len(info.Entities) < 2 {
		t.Errorf("expected at least 2 entities in flate pdf, got %d", len(info.Entities))
	}
}

func TestApp_EdgeCases(t *testing.T) {
	app := NewApp()

	// Empty path
	_, err := app.InspectFile("")
	if err == nil {
		t.Errorf("expected error on empty inspect path")
	}

	_, err = app.ProcessFile("", "salt")
	if err == nil {
		t.Errorf("expected error on empty process path")
	}

	// Non-existent file
	_, err = app.InspectFile("/path/does/not/exist.csv")
	if err == nil {
		t.Errorf("expected error on non-existent file")
	}

	// Unsupported extension
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "file.txt")
	_ = os.WriteFile(txtPath, []byte("hello"), 0644)
	_, err = app.InspectFile(txtPath)
	if err == nil {
		t.Errorf("expected error on unsupported extension .txt")
	}
}

func TestApp_GetAppInfo(t *testing.T) {
	app := NewApp()
	info := app.GetAppInfo()
	expectedVersion := "v" + version.Version
	if info["version"] != expectedVersion {
		t.Errorf("expected version %s, got %s", expectedVersion, info["version"])
	}
	if info["os"] == "" {
		t.Errorf("expected non-empty OS name")
	}
	if info["arch"] == "" {
		t.Errorf("expected non-empty Arch")
	}
}

func TestApp_InspectExcelWithPreamble(t *testing.T) {
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "preamble.xlsx")

	f := excelize.NewFile()
	defer f.Close()

	// Write title rows
	_ = f.SetCellValue("Sheet1", "A1", "Отчет по курсам")
	_ = f.SetCellValue("Sheet1", "A2", "Период: 2026")
	// Header at row 4
	_ = f.SetCellValue("Sheet1", "A4", "ID")
	_ = f.SetCellValue("Sheet1", "B4", "ФИО сотрудника")
	_ = f.SetCellValue("Sheet1", "C4", "Рабочая почта")
	_ = f.SetCellValue("Sheet1", "D4", "Дата приема")
	// Data at row 5
	_ = f.SetCellValue("Sheet1", "A5", "1")
	_ = f.SetCellValue("Sheet1", "B5", "Иванов Иван")
	_ = f.SetCellValue("Sheet1", "C5", "i.ivanov@corp.local")
	_ = f.SetCellValue("Sheet1", "D5", "01.01.2024")

	if err := f.SaveAs(xlsxPath); err != nil {
		t.Fatalf("failed to save xlsx: %v", err)
	}

	app := NewApp()
	info, err := app.InspectFile(xlsxPath)
	if err != nil {
		t.Fatalf("InspectFile failed: %v", err)
	}

	if len(info.Headers) != 4 || info.Headers[1] != "ФИО сотрудника" {
		t.Errorf("expected header row to be detected at row 4, got headers: %v", info.Headers)
	}

	if len(info.Entities) != 3 {
		t.Errorf("expected 3 detected entities (FIO, Email, Date), got %d: %v", len(info.Entities), info.Entities)
	}

	if len(info.PreviewRows) != 1 {
		t.Fatalf("expected 1 data preview row, got %d", len(info.PreviewRows))
	}

	// Verify anonymization in preview
	row := info.PreviewRows[0]
	if row.Before[1] == row.After[1] {
		t.Errorf("expected FIO to be anonymized in preview")
	}
	if row.Before[2] == row.After[2] {
		t.Errorf("expected Email to be anonymized in preview")
	}
}

func TestApp_GetTableRows(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "many_clients.csv")

	var sb strings.Builder
	sb.WriteString("ФИО,СНИЛС,Телефон\n")
	for i := 1; i <= 35; i++ {
		sb.WriteString(fmt.Sprintf("Иванов Иван %d,112-233-445 95,+7 916 111-22-%02d\n", i, i))
	}
	if err := os.WriteFile(csvPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	app := NewApp()

	// Window 1: offset 0, limit 15
	rows1, err := app.GetTableRows(csvPath, 0, 15, "test-salt", nil)
	if err != nil {
		t.Fatalf("GetTableRows offset 0 failed: %v", err)
	}
	if len(rows1) != 15 {
		t.Fatalf("expected 15 rows, got %d", len(rows1))
	}
	if rows1[0].Index != 1 || rows1[14].Index != 15 {
		t.Errorf("unexpected row indices in window 1: first=%d, last=%d", rows1[0].Index, rows1[14].Index)
	}
	// Verify FIO is anonymized
	if rows1[0].Before[0] == rows1[0].After[0] {
		t.Errorf("expected FIO to be anonymized in window 1")
	}

	// Window 2: offset 15, limit 15
	rows2, err := app.GetTableRows(csvPath, 15, 15, "test-salt", nil)
	if err != nil {
		t.Fatalf("GetTableRows offset 15 failed: %v", err)
	}
	if len(rows2) != 15 {
		t.Fatalf("expected 15 rows in window 2, got %d", len(rows2))
	}
	if rows2[0].Index != 16 || rows2[14].Index != 30 {
		t.Errorf("unexpected row indices in window 2: first=%d, last=%d", rows2[0].Index, rows2[14].Index)
	}

	// Test with disabled column 2 (СНИЛС) -> 1-based index 2
	rowsDisabled, err := app.GetTableRows(csvPath, 0, 5, "test-salt", []int{2})
	if err != nil {
		t.Fatalf("GetTableRows with disabled col failed: %v", err)
	}
	for i, r := range rowsDisabled {
		// FIO (col 1) should be anonymized
		if r.Before[0] == r.After[0] {
			t.Errorf("row %d: col 1 (FIO) expected anonymized", i)
		}
		// SNILS (col 2) should NOT be anonymized because disabled
		if r.Before[1] != r.After[1] {
			t.Errorf("row %d: col 2 (SNILS) should remain original when disabled, got before=%s after=%s", i, r.Before[1], r.After[1])
		}
	}
}

func TestApp_ProcessFileTo_DisabledColumns(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "clients_to_disable.csv")
	outPath := filepath.Join(tmpDir, "clients_disabled_out.csv")

	content := "ФИО,СНИЛС,Телефон\n" +
		"Иванов Иван Иванович,112-233-445 95,+7 916 123-45-67\n"

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	app := NewApp()
	// Disable column 2 (СНИЛС)
	res, err := app.ProcessFileTo(csvPath, outPath, "test-salt", []int{2}, false, "", "", false)
	if err != nil {
		t.Fatalf("ProcessFileTo failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("ProcessFileTo reported failure: %s", res.Error)
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outStr := string(outBytes)

	// FIO must be anonymized
	if strings.Contains(outStr, "Иванов Иван Иванович") {
		t.Errorf("expected FIO to be anonymized in output")
	}
	// SNILS was disabled, so it must be preserved
	if !strings.Contains(outStr, "112-233-445 95") {
		t.Errorf("expected disabled SNILS to be preserved in output")
	}
}

func TestApp_ProcessWithKeyGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "clients_keygen.csv")
	outPath := filepath.Join(tmpDir, "clients_keygen_anon.csv")
	keyPath := filepath.Join(tmpDir, "custom.depmap")

	content := "ФИО,СНИЛС,Телефон\n" +
		"Иванов Иван Иванович,112-233-445 95,+7 (916) 123-45-67\n" +
		"Петров Петр Петрович,223-344-556 06,+7 (903) 765-43-21\n"

	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	app := NewApp()

	// 1. Process with custom keyPath and password
	res, err := app.ProcessFileTo(csvPath, outPath, "test-salt", nil, true, "secret123", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileTo with key gen failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("ProcessFileTo reported failure: %s", res.Error)
	}
	if res.KeyPath != keyPath {
		t.Fatalf("expected KeyPath=%s, got %s", keyPath, res.KeyPath)
	}

	// Verify key file was created on disk
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key file to exist at %s: %v", keyPath, err)
	}

	// Verify key can be loaded and decrypted
	dm, err := depmap.LoadFile(keyPath, "secret123")
	if err != nil {
		t.Fatalf("failed to load generated key file: %v", err)
	}
	if dm.RecordsCount < 4 {
		t.Errorf("expected at least 4 mappings in depmap, got %d", dm.RecordsCount)
	}
	if dm.OriginalFileName != filepath.Base(csvPath) {
		t.Errorf("expected OriginalFileName=%s, got %s", filepath.Base(csvPath), dm.OriginalFileName)
	}

	// Check mapping contains original names
	foundFIO := false
	for _, orig := range dm.Mapping {
		if orig == "Иванов Иван Иванович" {
			foundFIO = true
			break
		}
	}
	if !foundFIO {
		t.Errorf("expected depmap to map synthetic FIO back to Ivanov")
	}

	// 2. Process with empty keyPath and empty password (default key path & default master key)
	outPath2 := filepath.Join(tmpDir, "auto_key_out.csv")
	res2, err := app.ProcessFileTo(csvPath, outPath2, "test-salt", nil, true, "", "", false)
	if err != nil {
		t.Fatalf("ProcessFileTo with default key gen failed: %v", err)
	}
	expectedDefaultKey := outPath2 + ".depmap"
	if !res2.KeyPasswordGenerated || res2.EffectivePassword == "" {
		t.Fatalf("expected auto-generated password, got generated=%v, pwd=%q", res2.KeyPasswordGenerated, res2.EffectivePassword)
	}
	dm2, err := depmap.LoadFile(expectedDefaultKey, res2.EffectivePassword)
	if err != nil {
		t.Fatalf("failed to load default key file with effective password: %v", err)
	}
	if dm2.RecordsCount == 0 {
		t.Errorf("expected non-zero mappings in default key file")
	}
}

func TestApp_RestoreWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp()

	// Part A: CSV workflow
	t.Run("CSV_Workflow", func(t *testing.T) {
		csvPath := filepath.Join(tmpDir, "source.csv")
		anonPath := filepath.Join(tmpDir, "source_anonymized.csv")
		keyPath := filepath.Join(tmpDir, "source.depmap")
		scoredPath := filepath.Join(tmpDir, "source_scored.csv")
		restoredPath := filepath.Join(tmpDir, "source_restored.csv")

		csvContent := "ФИО,СНИЛС,Телефон,Сумма\n" +
			"Иванов Иван Иванович,112-233-445 95,+7 (916) 123-45-67,1000\n" +
			"Петров Петр Петрович,223-344-556 06,+7 (903) 765-43-21,2500\n"

		if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
			t.Fatalf("failed to write csv: %v", err)
		}

		// Process with key generation
		procRes, err := app.ProcessFileTo(csvPath, anonPath, "salt-workflow", nil, true, "pass123", keyPath, false)
		if err != nil {
			t.Fatalf("ProcessFileTo failed: %v", err)
		}
		if !procRes.Success {
			t.Fatalf("ProcessFileTo failed: %s", procRes.Error)
		}

		// Read anon file
		anonBytes, err := os.ReadFile(anonPath)
		if err != nil {
			t.Fatalf("failed to read anon file: %v", err)
		}
		anonLines := strings.Split(strings.TrimSpace(string(anonBytes)), "\n")
		if len(anonLines) != 3 {
			t.Fatalf("expected 3 lines in anon csv, got %d", len(anonLines))
		}

		// Simulate contractor adding "Score" and "Category" columns
		var scoredBuf strings.Builder
		scoredBuf.WriteString(anonLines[0] + ",Score,Category\n")
		scoredBuf.WriteString(anonLines[1] + ",0.95,High\n")
		scoredBuf.WriteString(anonLines[2] + ",0.42,Low\n")

		if err := os.WriteFile(scoredPath, []byte(scoredBuf.String()), 0644); err != nil {
			t.Fatalf("failed to write scored csv: %v", err)
		}

		// Restore file
		restoreRes, err := app.RestoreFileTo(scoredPath, keyPath, "pass123", restoredPath)
		if err != nil {
			t.Fatalf("RestoreFileTo failed: %v", err)
		}
		if !restoreRes.Success {
			t.Fatalf("RestoreFileTo reported failure: %s", restoreRes.Error)
		}
		if restoreRes.RestoredCells < 6 {
			t.Errorf("expected at least 6 restored cells, got %d", restoreRes.RestoredCells)
		}

		restoredBytes, err := os.ReadFile(restoredPath)
		if err != nil {
			t.Fatalf("failed to read restored file: %v", err)
		}
		restoredStr := string(restoredBytes)

		// Verify original PII restored
		if !strings.Contains(restoredStr, "Иванов Иван Иванович") {
			t.Errorf("restored file missing original FIO Ivanov")
		}
		if !strings.Contains(restoredStr, "112-233-445 95") {
			t.Errorf("restored file missing original SNILS Ivanov")
		}
		if !strings.Contains(restoredStr, "Петров Петр Петрович") {
			t.Errorf("restored file missing original FIO Petrov")
		}
		// Verify contractor columns preserved
		if !strings.Contains(restoredStr, "0.95,High") || !strings.Contains(restoredStr, "0.42,Low") {
			t.Errorf("restored file did not preserve contractor extra columns")
		}
		// Verify original unmapped Sum preserved
		if !strings.Contains(restoredStr, "1000") || !strings.Contains(restoredStr, "2500") {
			t.Errorf("restored file lost original sum column")
		}
	})

	// Part B: Excel workflow
	t.Run("XLSX_Workflow", func(t *testing.T) {
		xlsxPath := filepath.Join(tmpDir, "source.xlsx")
		anonPath := filepath.Join(tmpDir, "source_anon.xlsx")
		scoredPath := filepath.Join(tmpDir, "source_scored.xlsx")
		restoredPath := filepath.Join(tmpDir, "source_restored.xlsx")

		xl := excelize.NewFile()
		sheet := "Sheet1"
		_ = xl.SetCellValue(sheet, "A1", "ФИО")
		_ = xl.SetCellValue(sheet, "B1", "СНИЛС")
		_ = xl.SetCellValue(sheet, "C1", "Паспорт")
		_ = xl.SetCellValue(sheet, "D1", "Департамент")

		_ = xl.SetCellValue(sheet, "A2", "Сидоров Сидор Сидорович")
		_ = xl.SetCellValue(sheet, "B2", "334-455-667 17")
		_ = xl.SetCellValue(sheet, "C2", "4510 123890")
		_ = xl.SetCellValue(sheet, "D2", "IT")

		if err := xl.SaveAs(xlsxPath); err != nil {
			t.Fatalf("failed to save source xlsx: %v", err)
		}
		_ = xl.Close()

		// Process with key generation (default key path & password)
		procRes, err := app.ProcessFileTo(xlsxPath, anonPath, "salt-excel", nil, true, "", "", false)
		if err != nil {
			t.Fatalf("ProcessFileTo on xlsx failed: %v", err)
		}
		if !procRes.Success {
			t.Fatalf("ProcessFileTo on xlsx failed: %s", procRes.Error)
		}

		// Contractor opens anon file, adds column E: Risk_Segment
		anonXl, err := excelize.OpenFile(anonPath)
		if err != nil {
			t.Fatalf("failed to open anon xlsx: %v", err)
		}
		_ = anonXl.SetCellValue(sheet, "E1", "Risk_Segment")
		_ = anonXl.SetCellValue(sheet, "E2", "Tier-1")
		if err := anonXl.SaveAs(scoredPath); err != nil {
			t.Fatalf("failed to save scored xlsx: %v", err)
		}
		_ = anonXl.Close()

		// Restore file
		restoreRes, err := app.RestoreFileTo(scoredPath, procRes.KeyPath, procRes.EffectivePassword, restoredPath)
		if err != nil {
			t.Fatalf("RestoreFileTo on xlsx failed: %v", err)
		}
		if !restoreRes.Success {
			t.Fatalf("RestoreFileTo on xlsx failed: %s", restoreRes.Error)
		}
		if restoreRes.RestoredCells < 3 {
			t.Errorf("expected at least 3 restored cells in xlsx, got %d", restoreRes.RestoredCells)
		}

		resXl, err := excelize.OpenFile(restoredPath)
		if err != nil {
			t.Fatalf("failed to open restored xlsx: %v", err)
		}
		defer resXl.Close()

		fio, _ := resXl.GetCellValue(sheet, "A2")
		snils, _ := resXl.GetCellValue(sheet, "B2")
		pass, _ := resXl.GetCellValue(sheet, "C2")
		dept, _ := resXl.GetCellValue(sheet, "D2")
		risk, _ := resXl.GetCellValue(sheet, "E2")

		if fio != "Сидоров Сидор Сидорович" {
			t.Errorf("expected restored FIO 'Сидоров Сидор Сидорович', got '%s'", fio)
		}
		if snils != "334-455-667 17" {
			t.Errorf("expected restored SNILS '334-455-667 17', got '%s'", snils)
		}
		if pass != "4510 123890" {
			t.Errorf("expected restored Passport '4510 123890', got '%s'", pass)
		}
		if dept != "IT" {
			t.Errorf("expected untouched Dept 'IT', got '%s'", dept)
		}
		if risk != "Tier-1" {
			t.Errorf("expected preserved contractor column Risk_Segment 'Tier-1', got '%s'", risk)
		}
	})
}

func TestApp_InspectKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp()

	// 1. Create a valid key
	keyPath := filepath.Join(tmpDir, "valid.depmap")
	dm := depmap.New("original_data.csv")
	dm.Put("Синтетическое ФИО", "Иванов Иван Иванович")
	dm.Put("111-222-333 44", "222-333-444 55")
	if err := dm.SaveFile(keyPath, "mypassword"); err != nil {
		t.Fatalf("failed to save depmap: %v", err)
	}

	// Test 1: Inspect valid key with correct password
	info, err := app.InspectKeyFile(keyPath, "mypassword")
	if err != nil {
		t.Fatalf("InspectKeyFile failed: %v", err)
	}
	if !info.Valid {
		t.Errorf("expected info.Valid=true, got false: %s", info.Error)
	}
	if info.RecordsCount != 2 {
		t.Errorf("expected RecordsCount=2, got %d", info.RecordsCount)
	}
	if info.OriginalFileName != "original_data.csv" {
		t.Errorf("expected OriginalFileName='original_data.csv', got '%s'", info.OriginalFileName)
	}
	if info.CreatedAt == "" {
		t.Errorf("expected non-empty CreatedAt")
	}

	// Test 2: Inspect valid key with wrong password
	infoWrong, err := app.InspectKeyFile(keyPath, "wrong_password")
	if err != nil {
		t.Fatalf("InspectKeyFile with wrong password returned error instead of invalid info: %v", err)
	}
	if infoWrong.Valid {
		t.Errorf("expected infoWrong.Valid=false for wrong password")
	}
	if infoWrong.Error == "" {
		t.Errorf("expected non-empty error message for wrong password")
	}

	// Test 3: Inspect invalid file (not a depmap container)
	badFile := filepath.Join(tmpDir, "corrupted.depmap")
	if err := os.WriteFile(badFile, []byte("NOT_A_VALID_DEPMAP_CONTAINER_DATA"), 0644); err != nil {
		t.Fatalf("failed to write bad file: %v", err)
	}
	infoBad, err := app.InspectKeyFile(badFile, "")
	if err != nil {
		t.Fatalf("InspectKeyFile with bad file returned error instead of invalid info: %v", err)
	}
	if infoBad.Valid {
		t.Errorf("expected infoBad.Valid=false for bad file")
	}
	if infoBad.Error == "" {
		t.Errorf("expected non-empty error message for bad file")
	}

	// Test 4: Inspect non-existent file
	_, err = app.InspectKeyFile(filepath.Join(tmpDir, "missing.depmap"), "")
	if err == nil {
		t.Errorf("expected error for non-existent file")
	}

	// Test 5: Inspect empty path
	_, err = app.InspectKeyFile("", "")
	if err == nil {
		t.Errorf("expected error for empty path")
	}
}

func TestApp_ContextNotInitialized(t *testing.T) {
	app := NewApp()
	// When app.ctx is nil:
	_, err := app.SelectKeyFile()
	if err == nil {
		t.Errorf("expected error when app.ctx is nil for SelectKeyFile")
	}
	_, err = app.SaveKeyFileDialog("test.depmap")
	if err == nil {
		t.Errorf("expected error when app.ctx is nil for SaveKeyFileDialog")
	}
}

func TestApp_MultiSheetExcel(t *testing.T) {
	tmpDir := t.TempDir()
	app := NewApp()

	xlsxPath := filepath.Join(tmpDir, "multisheet.xlsx")
	anonPath := filepath.Join(tmpDir, "multisheet_anon.xlsx")
	keyPath := filepath.Join(tmpDir, "multisheet.depmap")
	restoredPath := filepath.Join(tmpDir, "multisheet_restored.xlsx")

	xl := excelize.NewFile()
	defer xl.Close()

	// Sheet 1: Сотрудники
	sheet1 := "Сотрудники"
	_ = xl.SetSheetName("Sheet1", sheet1)
	_ = xl.SetCellValue(sheet1, "A1", "ФИО")
	_ = xl.SetCellValue(sheet1, "B1", "СНИЛС")
	_ = xl.SetCellValue(sheet1, "A2", "Иванов Иван Иванович")
	_ = xl.SetCellValue(sheet1, "B2", "112-233-445 95")
	_ = xl.SetCellValue(sheet1, "A3", "Петров Петр Петрович")
	_ = xl.SetCellValue(sheet1, "B3", "223-344-556 06")

	// Sheet 2: Клиенты
	sheet2 := "Клиенты"
	_, _ = xl.NewSheet(sheet2)
	_ = xl.SetCellValue(sheet2, "A1", "Клиент ФИО")
	_ = xl.SetCellValue(sheet2, "B1", "Телефон")
	_ = xl.SetCellValue(sheet2, "C1", "Сумма")
	_ = xl.SetCellValue(sheet2, "A2", "Сидорова Анна Сергеевна")
	_ = xl.SetCellValue(sheet2, "B2", "+7 (999) 111-22-33")
	_ = xl.SetCellValue(sheet2, "C2", "50000")
	_ = xl.SetCellValue(sheet2, "A3", "Кузнецов Дмитрий Павлович")
	_ = xl.SetCellValue(sheet2, "B3", "+7 (999) 444-55-66")
	_ = xl.SetCellValue(sheet2, "C3", "75000")

	if err := xl.SaveAs(xlsxPath); err != nil {
		t.Fatalf("failed to save multisheet xlsx: %v", err)
	}

	// 1. InspectFile
	info, err := app.InspectFile(xlsxPath)
	if err != nil {
		t.Fatalf("InspectFile on multi-sheet excel failed: %v", err)
	}

	if len(info.Sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d", len(info.Sheets))
	}
	if info.Sheets[0].Name != sheet1 {
		t.Errorf("expected sheet 0 name %s, got %s", sheet1, info.Sheets[0].Name)
	}
	if info.Sheets[1].Name != sheet2 {
		t.Errorf("expected sheet 1 name %s, got %s", sheet2, info.Sheets[1].Name)
	}
	if info.ActiveSheet != sheet1 {
		t.Errorf("expected activeSheet %s, got %s", sheet1, info.ActiveSheet)
	}
	if info.Sheets[0].RowCount != 2 {
		t.Errorf("expected sheet 0 rowCount 2, got %d", info.Sheets[0].RowCount)
	}
	if info.Sheets[1].RowCount != 2 {
		t.Errorf("expected sheet 1 rowCount 2, got %d", info.Sheets[1].RowCount)
	}
	if len(info.Sheets[0].Entities) != 2 {
		t.Errorf("expected 2 entities in sheet 0 (FIO, SNILS), got %d", len(info.Sheets[0].Entities))
	}
	if len(info.Sheets[1].Entities) != 2 {
		t.Errorf("expected 2 entities in sheet 1 (FIO, Phone), got %d", len(info.Sheets[1].Entities))
	}

	// 2. GetSheetRows for sheet 1 vs sheet 2
	rows1, err := app.GetSheetRows(xlsxPath, sheet1, 0, 10, "test-salt", nil, "")
	if err != nil {
		t.Fatalf("GetSheetRows sheet 1 failed: %v", err)
	}
	if len(rows1) != 2 {
		t.Fatalf("expected 2 rows for sheet 1, got %d", len(rows1))
	}
	if rows1[0].Before[0] != "Иванов Иван Иванович" {
		t.Errorf("expected sheet 1 row 0 before[0] 'Иванов Иван Иванович', got %s", rows1[0].Before[0])
	}

	rows2, err := app.GetSheetRows(xlsxPath, sheet2, 0, 10, "test-salt", nil, "")
	if err != nil {
		t.Fatalf("GetSheetRows sheet 2 failed: %v", err)
	}
	if len(rows2) != 2 {
		t.Fatalf("expected 2 rows for sheet 2, got %d", len(rows2))
	}
	if rows2[0].Before[0] != "Сидорова Анна Сергеевна" {
		t.Errorf("expected sheet 2 row 0 before[0] 'Сидорова Анна Сергеевна', got %s", rows2[0].Before[0])
	}

	// 3. ProcessFileWithSheets: disable column 2 (СНИЛС) on sheet1 only!
	disabledBySheet := map[string][]int{
		sheet1: {2}, // column 2: СНИЛС
	}

	procRes, err := app.ProcessFileWithSheets(xlsxPath, anonPath, "multi-salt", nil, disabledBySheet, true, "pass-multi", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileWithSheets failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileWithSheets reported failure: %s", procRes.Error)
	}
	if procRes.RowsCount != 4 {
		t.Errorf("expected total 4 rows across both sheets, got %d", procRes.RowsCount)
	}

	// Verify anonymized file
	anonXl, err := excelize.OpenFile(anonPath)
	if err != nil {
		t.Fatalf("failed to open anon xlsx: %v", err)
	}
	defer anonXl.Close()

	// Sheet 1: FIO must be anonymized, SNILS must be preserved (column 2 was disabled)
	fio1, _ := anonXl.GetCellValue(sheet1, "A2")
	snils1, _ := anonXl.GetCellValue(sheet1, "B2")
	if fio1 == "Иванов Иван Иванович" || fio1 == "" {
		t.Errorf("expected Sheet 1 FIO to be anonymized, got: %s", fio1)
	}
	if snils1 != "112-233-445 95" {
		t.Errorf("expected Sheet 1 SNILS to be preserved unaltered, got: %s", snils1)
	}

	// Sheet 2: FIO and Phone must be anonymized, Sum preserved
	client2, _ := anonXl.GetCellValue(sheet2, "A2")
	phone2, _ := anonXl.GetCellValue(sheet2, "B2")
	sum2, _ := anonXl.GetCellValue(sheet2, "C2")
	if client2 == "Сидорова Анна Сергеевна" || client2 == "" {
		t.Errorf("expected Sheet 2 Client FIO to be anonymized, got: %s", client2)
	}
	if phone2 == "+7 (999) 111-22-33" || phone2 == "" {
		t.Errorf("expected Sheet 2 Phone to be anonymized, got: %s", phone2)
	}
	if sum2 != "50000" {
		t.Errorf("expected Sheet 2 Sum to be preserved 50000, got: %s", sum2)
	}

	// 4. RestoreFileTo across multi-sheet workbook
	restoreRes, err := app.RestoreFileTo(anonPath, keyPath, "pass-multi", restoredPath)
	if err != nil {
		t.Fatalf("RestoreFileTo on multisheet workbook failed: %v", err)
	}
	if !restoreRes.Success {
		t.Fatalf("RestoreFileTo failed: %s", restoreRes.Error)
	}

	resXl, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored multisheet xlsx: %v", err)
	}
	defer resXl.Close()

	// Verify restored Sheet 1
	restoredFio1, _ := resXl.GetCellValue(sheet1, "A2")
	if restoredFio1 != "Иванов Иван Иванович" {
		t.Errorf("expected restored Sheet 1 FIO 'Иванов Иван Иванович', got: %s", restoredFio1)
	}
	// Verify restored Sheet 2
	restoredClient2, _ := resXl.GetCellValue(sheet2, "A2")
	restoredPhone2, _ := resXl.GetCellValue(sheet2, "B2")
	if restoredClient2 != "Сидорова Анна Сергеевна" {
		t.Errorf("expected restored Sheet 2 Client 'Сидорова Анна Сергеевна', got: %s", restoredClient2)
	}
	if restoredPhone2 != "+7 (999) 111-22-33" {
		t.Errorf("expected restored Sheet 2 Phone '+7 (999) 111-22-33', got: %s", restoredPhone2)
	}
}

func createTestODSFile(t *testing.T, path string) {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		t.Fatalf("failed to create mimetype header: %v", err)
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Clients">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>СНИЛС</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Телефон</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Иванов Иван Иванович</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>112-233-445 95</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>+7 (916) 123-45-67</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`

	cw, err := zw.Create("content.xml")
	if err != nil {
		t.Fatalf("failed to create content.xml: %v", err)
	}
	cw.Write([]byte(contentXML))
	zw.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test ods: %v", err)
	}
}

func createTestDocxFile(t *testing.T, path string, textContent string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	ct, _ := zw.Create("[Content_Types].xml")
	ct.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`))

	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>%s</w:t></w:r></w:p>
  </w:body>
</w:document>`, textContent)

	docW, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("failed to create word/document.xml: %v", err)
	}
	docW.Write([]byte(docXML))
	zw.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test docx: %v", err)
	}
}

func createTestODTFile(t *testing.T, path string, textContent string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		t.Fatalf("failed to create mimetype header: %v", err)
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.text"))

	contentXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:text>
   <text:p>%s</text:p>
  </office:text>
 </office:body>
</office:document-content>`, textContent)

	cw, err := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Deflate})
	if err != nil {
		t.Fatalf("failed to create content.xml header: %v", err)
	}
	cw.Write([]byte(contentXML))
	zw.Close()

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write test odt: %v", err)
	}
}

func readZipFileContent(t *testing.T, archivePath string, targetFile string) string {
	t.Helper()
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("failed to open zip reader on %s: %v", archivePath, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name == targetFile {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open file %s in zip: %v", targetFile, err)
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("failed to read file %s in zip: %v", targetFile, err)
			}
			return string(data)
		}
	}
	t.Fatalf("file %s not found in zip %s", targetFile, archivePath)
	return ""
}

func TestApp_ExpandedFormats_ODS(t *testing.T) {
	tmpDir := t.TempDir()
	odsPath := filepath.Join(tmpDir, "clients.ods")
	anonODSPath := filepath.Join(tmpDir, "clients_anon.ods")
	restoredODSPath := filepath.Join(tmpDir, "clients_restored.ods")
	keyPath := filepath.Join(tmpDir, "clients.depmap")
	exportXLSXPath := filepath.Join(tmpDir, "clients_export.xlsx")

	createTestODSFile(t, odsPath)

	app := NewApp()

	// 1. InspectFile
	info, err := app.InspectFile(odsPath)
	if err != nil {
		t.Fatalf("InspectFile on ods failed: %v", err)
	}
	if info.Format != "ods" {
		t.Errorf("expected Format ods, got %s", info.Format)
	}
	if info.FileType != "table" {
		t.Errorf("expected FileType table, got %s", info.FileType)
	}
	if len(info.Sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(info.Sheets))
	}
	if len(info.Entities) < 3 {
		t.Errorf("expected at least 3 detected entities, got %d", len(info.Entities))
	}

	// 2. ProcessFileTo (ODS -> ODS) with key generation
	procRes, err := app.ProcessFileTo(odsPath, anonODSPath, "test-salt", nil, true, "pass123", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileTo on ods failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileTo failed: %s", procRes.Error)
	}

	anonContent := readZipFileContent(t, anonODSPath, "content.xml")
	if strings.Contains(anonContent, "Иванов Иван Иванович") {
		t.Errorf("anonymized ods still contains original FIO")
	}

	// 3. RestoreFileTo (ODS -> ODS)
	restRes, err := app.RestoreFileTo(anonODSPath, keyPath, "pass123", restoredODSPath)
	if err != nil {
		t.Fatalf("RestoreFileTo on ods failed: %v", err)
	}
	if !restRes.Success || restRes.RestoredCells < 1 {
		t.Fatalf("RestoreFileTo failed or 0 restored cells: %+v", restRes)
	}

	restoredContent := readZipFileContent(t, restoredODSPath, "content.xml")
	if !strings.Contains(restoredContent, "Иванов Иван Иванович") {
		t.Errorf("restored ods missing original FIO")
	}
	if !strings.Contains(restoredContent, "112-233-445 95") {
		t.Errorf("restored ods missing original SNILS")
	}

	// 4. Export ODS to XLSX
	exportRes, err := app.ProcessFileTo(odsPath, exportXLSXPath, "test-salt", nil, false, "", "", false)
	if err != nil {
		t.Fatalf("export ODS to XLSX failed: %v", err)
	}
	if !exportRes.Success {
		t.Fatalf("export ODS to XLSX failed: %s", exportRes.Error)
	}

	xl, err := excelize.OpenFile(exportXLSXPath)
	if err != nil {
		t.Fatalf("failed to open exported xlsx: %v", err)
	}
	defer xl.Close()

	val, _ := xl.GetCellValue("Clients", "A2")
	if val == "Иванов Иван Иванович" || val == "" {
		t.Errorf("expected anonymized FIO in exported xlsx, got: %s", val)
	}
}

func TestApp_ExpandedFormats_DOCX(t *testing.T) {
	tmpDir := t.TempDir()
	docxPath := filepath.Join(tmpDir, "contract.docx")
	anonDocxPath := filepath.Join(tmpDir, "contract_anon.docx")
	restoredDocxPath := filepath.Join(tmpDir, "contract_restored.docx")
	keyPath := filepath.Join(tmpDir, "contract.depmap")

	rawText := "Договор с гражданином РФ Иванов Иван Иванович, паспорт 4510 123890, телефон +7 (916) 123-45-67."
	createTestDocxFile(t, docxPath, rawText)

	app := NewApp()

	// 1. InspectFile
	info, err := app.InspectFile(docxPath)
	if err != nil {
		t.Fatalf("InspectFile on docx failed: %v", err)
	}
	if info.Format != "docx" {
		t.Errorf("expected Format docx, got %s", info.Format)
	}
	if info.FileType != "text" {
		t.Errorf("expected FileType text, got %s", info.FileType)
	}
	if info.ParagraphCount != 1 {
		t.Errorf("expected 1 paragraph, got %d", info.ParagraphCount)
	}
	if len(info.Entities) < 2 {
		t.Errorf("expected at least 2 entities detected in docx, got %d", len(info.Entities))
	}

	// 2. ProcessFileTo with key generation
	procRes, err := app.ProcessFileTo(docxPath, anonDocxPath, "docx-salt", nil, true, "secret", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileTo on docx failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileTo failed: %s", procRes.Error)
	}

	anonXML := readZipFileContent(t, anonDocxPath, "word/document.xml")
	if strings.Contains(anonXML, "Иванов Иван Иванович") {
		t.Errorf("anonymized docx still contains original FIO")
	}
	if strings.Contains(anonXML, "4510 123890") {
		t.Errorf("anonymized docx still contains original passport")
	}

	// 3. RestoreFileTo
	restRes, err := app.RestoreFileTo(anonDocxPath, keyPath, "secret", restoredDocxPath)
	if err != nil {
		t.Fatalf("RestoreFileTo on docx failed: %v", err)
	}
	if !restRes.Success || restRes.RestoredCells < 1 {
		t.Fatalf("RestoreFileTo failed or 0 restored cells: %+v", restRes)
	}

	restoredXML := readZipFileContent(t, restoredDocxPath, "word/document.xml")
	if !strings.Contains(restoredXML, "Иванов Иван Иванович") {
		t.Errorf("restored docx missing original FIO")
	}
	if !strings.Contains(restoredXML, "4510 123890") {
		t.Errorf("restored docx missing original passport")
	}
}

func TestApp_ExpandedFormats_ODT(t *testing.T) {
	tmpDir := t.TempDir()
	odtPath := filepath.Join(tmpDir, "contract.odt")
	anonOdtPath := filepath.Join(tmpDir, "contract_anon.odt")
	restoredOdtPath := filepath.Join(tmpDir, "contract_restored.odt")
	keyPath := filepath.Join(tmpDir, "contract.depmap")

	rawText := "Договор с гражданином Сидоров Алексей Викторович, СНИЛС 112-233-445 95."
	createTestODTFile(t, odtPath, rawText)

	app := NewApp()

	// 1. InspectFile
	info, err := app.InspectFile(odtPath)
	if err != nil {
		t.Fatalf("InspectFile on odt failed: %v", err)
	}
	if info.Format != "odt" {
		t.Errorf("expected Format odt, got %s", info.Format)
	}
	if info.FileType != "text" {
		t.Errorf("expected FileType text, got %s", info.FileType)
	}
	if len(info.Entities) < 2 {
		t.Errorf("expected at least 2 entities detected in odt, got %d", len(info.Entities))
	}

	// 2. ProcessFileTo with key generation
	procRes, err := app.ProcessFileTo(odtPath, anonOdtPath, "odt-salt", nil, true, "pass", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileTo on odt failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileTo failed: %s", procRes.Error)
	}

	anonXML := readZipFileContent(t, anonOdtPath, "content.xml")
	if strings.Contains(anonXML, "Сидоров Алексей Викторович") {
		t.Errorf("anonymized odt still contains original FIO")
	}
	if strings.Contains(anonXML, "112-233-445 95") {
		t.Errorf("anonymized odt still contains original SNILS")
	}

	// 3. RestoreFileTo
	restRes, err := app.RestoreFileTo(anonOdtPath, keyPath, "pass", restoredOdtPath)
	if err != nil {
		t.Fatalf("RestoreFileTo on odt failed: %v", err)
	}
	if !restRes.Success || restRes.RestoredCells < 1 {
		t.Fatalf("RestoreFileTo failed or 0 restored cells: %+v", restRes)
	}

	restoredXML := readZipFileContent(t, restoredOdtPath, "content.xml")
	if !strings.Contains(restoredXML, "Сидоров Алексей Викторович") {
		t.Errorf("restored odt missing original FIO")
	}
	if !strings.Contains(restoredXML, "112-233-445 95") {
		t.Errorf("restored odt missing original SNILS")
	}
}

func TestApp_ExpandedFormats_TextDisabledEntities(t *testing.T) {
	tmpDir := t.TempDir()
	docxPath := filepath.Join(tmpDir, "contract.docx")
	anonDocxPath := filepath.Join(tmpDir, "contract_anon.docx")

	rawText := "Договор с гражданином РФ Иванов Иван Иванович, телефон +7 (916) 123-45-67."
	createTestDocxFile(t, docxPath, rawText)

	app := NewApp()
	info, err := app.InspectFile(docxPath)
	if err != nil {
		t.Fatalf("InspectFile failed: %v", err)
	}

	// Find which entity is FIO
	fioColIdx := -1
	for _, ent := range info.Entities {
		if ent.Type == "FIO" {
			fioColIdx = ent.ColumnIndex
			break
		}
	}
	if fioColIdx == -1 {
		t.Fatalf("FIO entity not found in inspected entities: %+v", info.Entities)
	}

	// Disable FIO entity
	procRes, err := app.ProcessFileTo(docxPath, anonDocxPath, "salt", []int{fioColIdx}, false, "", "", false)
	if err != nil {
		t.Fatalf("ProcessFileTo with disabled entity failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileTo failed: %s", procRes.Error)
	}

	anonXML := readZipFileContent(t, anonDocxPath, "word/document.xml")
	// FIO should be preserved because it was disabled!
	if !strings.Contains(anonXML, "Иванов Иван Иванович") {
		t.Errorf("expected disabled FIO 'Иванов Иван Иванович' to be preserved in docx")
	}
	// Phone should be anonymized
	if strings.Contains(anonXML, "+7 (916) 123-45-67") {
		t.Errorf("expected phone to be anonymized in docx")
	}
}




