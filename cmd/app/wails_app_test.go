package main

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

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
