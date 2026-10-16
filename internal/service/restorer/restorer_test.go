package restorer_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/restorer"
	"dep_service/pkg/audit"
	"dep_service/pkg/depmap"
)

func TestInspectKeyFileInvalid(t *testing.T) {
	km := keymanager.New()
	_, err := km.InspectKey("/nonexistent/key.depmap", "secret")
	if err == nil {
		t.Errorf("expected error for nonexistent key file, got nil")
	}
}

func TestRestorer_New_DefaultKM(t *testing.T) {
	r := restorer.New()
	if r == nil {
		t.Fatalf("expected non-nil Restorer")
	}
	var _ *restorer.RestorerService = r
}

func TestRestorer_Restore_CSV_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	origCSVPath := filepath.Join(tmpDir, "original.csv")
	anonCSVPath := filepath.Join(tmpDir, "original_anonymized.csv")
	keyPath := filepath.Join(tmpDir, "original.depmap")
	restoredCSVPath := filepath.Join(tmpDir, "original_restored.csv")

	origContent := "Name;Phone;City\nИванов Иван;+79991234567;Москва\nПетров Петр;+79997654321;Санкт-Петербург\n"
	anonContent := "Name;Phone;City\n[NAME_1];[PHONE_1];Москва\n[NAME_2];[PHONE_2];Санкт-Петербург\n"

	if err := os.WriteFile(origCSVPath, []byte(origContent), 0644); err != nil {
		t.Fatalf("failed to write original CSV: %v", err)
	}
	if err := os.WriteFile(anonCSVPath, []byte(anonContent), 0644); err != nil {
		t.Fatalf("failed to write anonymized CSV: %v", err)
	}

	mappings := map[string]string{
		"[NAME_1]":  "Иванов Иван",
		"[PHONE_1]": "+79991234567",
		"[NAME_2]":  "Петров Петр",
		"[PHONE_2]": "+79997654321",
	}
	if err := km.CreateContainer(keyPath, "testpass", "original.csv", mappings); err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	var progressUpdates []int
	reporter := func(proc, total, pct int) {
		progressUpdates = append(progressUpdates, pct)
	}

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonCSVPath,
		KeyPath:    keyPath,
		Password:   "testpass",
		OutputPath: restoredCSVPath,
	}, reporter)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected res.Success=true")
	}
	if res.RestoredCells != 4 {
		t.Errorf("expected 4 restored cells, got %d", res.RestoredCells)
	}
	if res.OutputPath != restoredCSVPath {
		t.Errorf("expected OutputPath=%s, got %s", restoredCSVPath, res.OutputPath)
	}

	// Verify reporter reached 100%
	if len(progressUpdates) == 0 || progressUpdates[len(progressUpdates)-1] != 100 {
		t.Errorf("expected progress updates to conclude at 100%%, got %v", progressUpdates)
	}

	// Verify restored file content matches original
	restoredBytes, err := os.ReadFile(restoredCSVPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}

	if strings.TrimSpace(strings.TrimPrefix(string(restoredBytes), "\ufeff")) != strings.TrimSpace(origContent) {
		t.Errorf("restored content mismatch:\nGot:\n%s\nExpected:\n%s", string(restoredBytes), origContent)
	}
}

func TestRestorer_Restore_TSV_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	origTSVPath := filepath.Join(tmpDir, "data.tsv")
	anonTSVPath := filepath.Join(tmpDir, "data_anonymized.tsv")
	keyPath := filepath.Join(tmpDir, "data.depmap")
	restoredTSVPath := filepath.Join(tmpDir, "data_restored.tsv")

	origContent := "Name\tPhone\nИванов\t+79991234567\n"
	anonContent := "Name\tPhone\n[NAME_1]\t[PHONE_1]\n"

	_ = os.WriteFile(origTSVPath, []byte(origContent), 0644)
	_ = os.WriteFile(anonTSVPath, []byte(anonContent), 0644)

	mappings := map[string]string{
		"[NAME_1]":  "Иванов",
		"[PHONE_1]": "+79991234567",
	}
	_ = km.CreateContainer(keyPath, "pass", "data.tsv", mappings)

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonTSVPath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: restoredTSVPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore TSV failed: %v", err)
	}

	if res.RestoredCells != 2 {
		t.Errorf("expected 2 restored cells, got %d", res.RestoredCells)
	}
	content, _ := os.ReadFile(restoredTSVPath)
	if strings.TrimSpace(strings.TrimPrefix(string(content), "\ufeff")) != strings.TrimSpace(origContent) {
		t.Errorf("content mismatch: got %q, want %q", string(content), origContent)
	}
}

func TestRestorer_Restore_Excel_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	anonXLSXPath := filepath.Join(tmpDir, "data_anonymized.xlsx")
	keyPath := filepath.Join(tmpDir, "data.depmap")
	restoredXLSXPath := filepath.Join(tmpDir, "data_restored.xlsx")

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "Имя")
	_ = f.SetCellValue("Sheet1", "B1", "Email")
	_ = f.SetCellValue("Sheet1", "A2", "[ANON_NAME_1]")
	_ = f.SetCellValue("Sheet1", "B2", "[ANON_EMAIL_1]")
	if err := f.SaveAs(anonXLSXPath); err != nil {
		t.Fatalf("failed to save anonymized xlsx: %v", err)
	}
	_ = f.Close()

	mappings := map[string]string{
		"[ANON_NAME_1]":  "Сидоров Сидор",
		"[ANON_EMAIL_1]": "sidorov@mail.ru",
	}
	if err := km.CreateContainer(keyPath, "secret", "data.xlsx", mappings); err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	var progressCalls int
	reporter := func(proc, total, pct int) {
		progressCalls++
	}

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonXLSXPath,
		KeyPath:    keyPath,
		Password:   "secret",
		OutputPath: restoredXLSXPath,
	}, reporter)
	if err != nil {
		t.Fatalf("Restore excel failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success")
	}
	if res.RestoredCells != 2 {
		t.Errorf("expected 2 restored cells, got %d", res.RestoredCells)
	}
	if progressCalls == 0 {
		t.Errorf("expected progress calls, got 0")
	}

	// Verify restored excel content
	fRestored, err := excelize.OpenFile(restoredXLSXPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer fRestored.Close()

	valA2, _ := fRestored.GetCellValue("Sheet1", "A2")
	valB2, _ := fRestored.GetCellValue("Sheet1", "B2")

	if valA2 != "Сидоров Сидор" {
		t.Errorf("expected 'Сидоров Сидор', got '%s'", valA2)
	}
	if valB2 != "sidorov@mail.ru" {
		t.Errorf("expected 'sidorov@mail.ru', got '%s'", valB2)
	}
}

func TestRestorer_Restore_DOCX_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	anonDOCXPath := filepath.Join(tmpDir, "document_anonymized.docx")
	keyPath := filepath.Join(tmpDir, "doc.depmap")
	restoredDOCXPath := filepath.Join(tmpDir, "document_restored.docx")

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>[SECRET_NAME]</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	docxBytes := createTestZipArchive(map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/></Types>`,
		"word/document.xml":   docXML,
	})

	if err := os.WriteFile(anonDOCXPath, docxBytes, 0644); err != nil {
		t.Fatalf("failed to write docx: %v", err)
	}

	mappings := map[string]string{
		"[SECRET_NAME]": "Дмитрий Дмитриев",
	}
	if err := km.CreateContainer(keyPath, "pass", "document.docx", mappings); err != nil {
		t.Fatalf("failed to create key container: %v", err)
	}

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonDOCXPath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: restoredDOCXPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore DOCX failed: %v", err)
	}
	if res.RestoredCells != 1 {
		t.Errorf("expected 1 restored cell, got %d", res.RestoredCells)
	}

	// Verify restored docx
	restoredData, err := os.ReadFile(restoredDOCXPath)
	if err != nil {
		t.Fatalf("failed to read restored docx: %v", err)
	}
	docContent, err := readZipMember(restoredData, "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read document.xml: %v", err)
	}
	if !strings.Contains(docContent, "Дмитрий Дмитриев") {
		t.Errorf("expected restored docx to contain 'Дмитрий Дмитриев', got: %s", docContent)
	}
}

func TestRestorer_Restore_ODS_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	anonODSPath := filepath.Join(tmpDir, "table_anonymized.ods")
	keyPath := filepath.Join(tmpDir, "ods.depmap")
	restoredODSPath := filepath.Join(tmpDir, "table_restored.ods")

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Лист1">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>[ANON_ODS]</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`

	odsBytes := createTestZipArchive(map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.spreadsheet",
		"content.xml": contentXML,
	})
	if err := os.WriteFile(anonODSPath, odsBytes, 0644); err != nil {
		t.Fatalf("failed to write ods: %v", err)
	}

	_ = km.CreateContainer(keyPath, "pass", "table.ods", map[string]string{
		"[ANON_ODS]": "Восстановлено ODS",
	})

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonODSPath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: restoredODSPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore ODS failed: %v", err)
	}
	if res.RestoredCells != 1 {
		t.Errorf("expected 1 restored cell, got %d", res.RestoredCells)
	}
}

func TestRestorer_Restore_ODT_Roundtrip(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	anonODTPath := filepath.Join(tmpDir, "text_anonymized.odt")
	keyPath := filepath.Join(tmpDir, "odt.depmap")
	restoredODTPath := filepath.Join(tmpDir, "text_restored.odt")

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:text>
   <text:p>[ANON_ODT]</text:p>
  </office:text>
 </office:body>
</office:document-content>`

	odtBytes := createTestZipArchive(map[string]string{
		"mimetype":    "application/vnd.oasis.opendocument.text",
		"content.xml": contentXML,
	})
	if err := os.WriteFile(anonODTPath, odtBytes, 0644); err != nil {
		t.Fatalf("failed to write odt: %v", err)
	}

	_ = km.CreateContainer(keyPath, "pass", "text.odt", map[string]string{
		"[ANON_ODT]": "Восстановлено ODT",
	})

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonODTPath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: restoredODTPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore ODT failed: %v", err)
	}
	if res.RestoredCells != 1 {
		t.Errorf("expected 1 restored cell, got %d", res.RestoredCells)
	}
}

func TestRestorer_Restore_InvalidPassword(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	csvPath := filepath.Join(tmpDir, "test.csv")
	keyPath := filepath.Join(tmpDir, "test.depmap")

	_ = os.WriteFile(csvPath, []byte("col\nval\n"), 0644)
	if err := km.CreateContainer(keyPath, "good_password", "test.csv", map[string]string{"val": "orig"}); err != nil {
		t.Fatalf("failed to create container: %v", err)
	}

	_, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath: csvPath,
		KeyPath:   keyPath,
		Password:  "wrong_password",
	}, nil)

	if err == nil {
		t.Fatalf("expected error for wrong password, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidPassword) {
		t.Errorf("expected error to wrap domain.ErrInvalidPassword, got: %v", err)
	}
}

func TestRestorer_Restore_UnsupportedFormat(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	txtPath := filepath.Join(tmpDir, "test.unsupported")
	keyPath := filepath.Join(tmpDir, "test.depmap")

	_ = os.WriteFile(txtPath, []byte("some text"), 0644)
	_ = km.CreateContainer(keyPath, "pass", "test.unsupported", map[string]string{"some": "other"})

	_, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath: txtPath,
		KeyPath:   keyPath,
		Password:  "pass",
	}, nil)

	if err == nil {
		t.Fatalf("expected error for unsupported format, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected error to wrap domain.ErrUnsupportedFormat, got: %v", err)
	}
}

func TestRestorer_Restore_ContextCanceled(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	csvPath := filepath.Join(tmpDir, "test.csv")
	keyPath := filepath.Join(tmpDir, "test.depmap")

	_ = os.WriteFile(csvPath, []byte("col\nval\n"), 0644)
	_ = km.CreateContainer(keyPath, "pass", "test.csv", map[string]string{"val": "orig"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := rest.Restore(ctx, domain.RestoreOptions{
		InputPath: csvPath,
		KeyPath:   keyPath,
		Password:  "pass",
	}, nil)

	if err == nil {
		t.Fatalf("expected error for canceled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestRestorer_Restore_MissingInputOrKey(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	// Missing input
	_, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath: filepath.Join(tmpDir, "missing.csv"),
		KeyPath:   filepath.Join(tmpDir, "key.depmap"),
	}, nil)
	if err == nil {
		t.Errorf("expected error for missing input, got nil")
	}

	// Empty input
	_, err = rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath: "",
		KeyPath:   filepath.Join(tmpDir, "key.depmap"),
	}, nil)
	if err == nil {
		t.Errorf("expected error for empty input path, got nil")
	}

	// Missing key
	existingFile := filepath.Join(tmpDir, "file.csv")
	_ = os.WriteFile(existingFile, []byte("a,b"), 0644)
	_, err = rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath: existingFile,
		KeyPath:   filepath.Join(tmpDir, "missing.depmap"),
	}, nil)
	if err == nil {
		t.Errorf("expected error for missing key file, got nil")
	}
}

func TestRestorer_Restore_DefaultOutputPath(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	anonPath := filepath.Join(tmpDir, "sample_anonymized.csv")
	keyPath := filepath.Join(tmpDir, "sample.depmap")

	_ = os.WriteFile(anonPath, []byte("header\nanon_val\n"), 0644)
	_ = km.CreateContainer(keyPath, "pass", "sample.csv", map[string]string{"anon_val": "orig_val"})

	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  anonPath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: "", // should auto-generate
	}, nil)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	expectedDest := filepath.Join(tmpDir, "sample_restored.csv")
	if res.OutputPath != expectedDest {
		t.Errorf("expected default OutputPath %s, got %s", expectedDest, res.OutputPath)
	}
	if _, err := os.Stat(expectedDest); err != nil {
		t.Errorf("expected default restored file to exist at %s: %v", expectedDest, err)
	}

	// Verify content
	content, _ := os.ReadFile(expectedDest)
	r := csv.NewReader(strings.NewReader(string(content)))
	rows, _ := r.ReadAll()
	if len(rows) < 2 || rows[1][0] != "orig_val" {
		t.Errorf("expected orig_val restored, got %v", rows)
	}
}

func TestRestore_SameInputAndOutputPath(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	samePath := filepath.Join(tmpDir, "file.csv")
	keyPath := filepath.Join(tmpDir, "file.depmap")

	_ = os.WriteFile(samePath, []byte("col\nval\n"), 0644)
	_ = km.CreateContainer(keyPath, "pass", "file.csv", map[string]string{"val": "orig"})

	_, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  samePath,
		KeyPath:    keyPath,
		Password:   "pass",
		OutputPath: samePath,
	}, nil)

	if err == nil {
		t.Fatalf("expected error when InputPath == OutputPath, got nil")
	}
	if !strings.Contains(err.Error(), "output path cannot be the same as input path") {
		t.Errorf("expected error message to mention 'output path cannot be the same as input path', got %v", err)
	}

	// Verify input file was not truncated or modified
	content, _ := os.ReadFile(samePath)
	if string(content) != "col\nval\n" {
		t.Errorf("expected input file content to be preserved, got %q", string(content))
	}
}

func createTestZipArchive(files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err == nil {
			_, _ = w.Write([]byte(content))
		}
	}
	_ = zw.Close()
	return buf.Bytes()
}

func readZipMember(data []byte, name string) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()
			var b bytes.Buffer
			_, err = b.ReadFrom(rc)
			return b.String(), err
		}
	}
	return "", errors.New("file not found in zip archive")
}

func TestRestorer_AuditLogging(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.jsonl")
	al, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	km := keymanager.New()
	rest := restorer.New(km)
	rest.SetAuditLogger(al)

	// Create test file and key
	inPath := filepath.Join(tmpDir, "test_anon.csv")
	restPath := filepath.Join(tmpDir, "test_restored.csv")
	keyPath := filepath.Join(tmpDir, "test.depmap")

	if err := os.WriteFile(inPath, []byte("ФИО\nАноним1\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	dm := depmap.New("test.csv")
	dm.Put("Аноним1", "Иванов Иван")
	if err := dm.SaveFile(keyPath, "secret123"); err != nil {
		t.Fatalf("failed to save key: %v", err)
	}

	// 1. Success case
	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  inPath,
		KeyPath:    keyPath,
		Password:   "secret123",
		OutputPath: restPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected restore success")
	}

	records, err := al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	r0 := records[0]
	if r0.Operation != "restore" {
		t.Errorf("expected operation 'restore', got %q", r0.Operation)
	}
	if r0.Status != "success" {
		t.Errorf("expected status 'success', got %q", r0.Status)
	}
	if r0.SourceHash == "" || r0.ResultHash == "" {
		t.Errorf("expected non-empty source and result hashes")
	}
	if r0.RecordsCount != 1 {
		t.Errorf("expected RecordsCount=1, got %d", r0.RecordsCount)
	}

	// 2. Error case (wrong password)
	_, _ = rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  inPath,
		KeyPath:    keyPath,
		Password:   "wrong-password",
		OutputPath: restPath,
	}, nil)

	records, err = al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(records))
	}
	if records[0].Status != "error" || records[0].Error == "" {
		t.Errorf("expected newest record to be error, got %+v", records[0])
	}

	// 3. Cancel case
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = rest.Restore(ctx, domain.RestoreOptions{
		InputPath:  inPath,
		KeyPath:    keyPath,
		Password:   "secret123",
		OutputPath: restPath,
	}, nil)

	records, err = al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 audit records, got %d", len(records))
	}
	if records[0].Status != "canceled" {
		t.Errorf("expected newest record to be canceled, got %+v", records[0])
	}
}

func TestRestorer_Restore_XLSX_MislabeledAsCSV(t *testing.T) {
	km := keymanager.New()
	rest := restorer.New(km)
	tmpDir := t.TempDir()

	// 1. Create an anonymized XLSX file, then rename it to mislabeled.csv
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ФИО")
	_ = f.SetCellValue(sheet, "A2", "Петров Петр Петрович") // Anonymized synthetic value
	tempXLSX := filepath.Join(tmpDir, "temp.xlsx")
	if err := f.SaveAs(tempXLSX); err != nil {
		t.Fatalf("failed to save temp excel: %v", err)
	}
	_ = f.Close()
	mislabeledCSV := filepath.Join(tmpDir, "mislabeled.csv")
	if err := os.Rename(tempXLSX, mislabeledCSV); err != nil {
		t.Fatalf("failed to rename temp excel to csv: %v", err)
	}

	// 2. Create a depmap key referencing original.xlsx
	dm := depmap.New("original.xlsx")
	dm.Put("Петров Петр Петрович", "Иванов Иван Иванович")
	keyPath := filepath.Join(tmpDir, "key.depmap")
	if _, err := dm.SaveFileWithAutoPassword(keyPath, "ValidPassword123!"); err != nil {
		t.Fatalf("failed to save depmap: %v", err)
	}

	// 3. Restore the mislabeled file
	destPath := filepath.Join(tmpDir, "restored.csv")
	res, err := rest.Restore(context.Background(), domain.RestoreOptions{
		InputPath:  mislabeledCSV,
		KeyPath:    keyPath,
		Password:   "ValidPassword123!",
		OutputPath: destPath,
	}, nil)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// 4. Verify cells were restored and count > 0
	if res.RestoredCells != 1 {
		t.Errorf("expected RestoredCells=1, got %d", res.RestoredCells)
	}

	// 5. Verify the restored file is a valid XLSX file and contains original value
	restoredXLSXPath := filepath.Join(tmpDir, "restored.xlsx")
	rf, err := excelize.OpenFile(restoredXLSXPath)
	if err != nil {
		t.Fatalf("failed to open restored file as XLSX: %v", err)
	}
	defer rf.Close()

	val, err := rf.GetCellValue(sheet, "A2")
	if err != nil {
		t.Fatalf("failed to read cell value: %v", err)
	}
	if val != "Иванов Иван Иванович" {
		t.Errorf("expected restored value 'Иванов Иван Иванович', got %q", val)
	}
}

