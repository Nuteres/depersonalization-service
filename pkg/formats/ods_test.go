package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

func createTestODS(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// mimetype (must be uncompressed in standard ODF)
	mimeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mimeHeader)
	if err != nil {
		t.Fatalf("failed to create mimetype header: %v", err)
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	// styles.xml to test archive member preservation
	sw, err := zw.Create("styles.xml")
	if err != nil {
		t.Fatalf("failed to create styles.xml: %v", err)
	}
	sw.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"/>`))

	// content.xml with 1 sheet, headers and data
	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Сотрудники">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>СНИЛС</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Телефон</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Иванов Иван Иванович</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>112-233-445 95</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>+7 916 123-45-67</text:p></table:table-cell>
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

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return buf.Bytes()
}

func TestODSHandler_Lifecycle(t *testing.T) {
	rawODS := createTestODS(t)
	anon := anonymizer.New("test-salt-ods")
	handler := &ODSHandler{Anon: anon}

	// 1. Inspect
	reader := bytes.NewReader(rawODS)
	sheets, err := handler.Inspect(reader, int64(len(rawODS)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if len(sheets) != 1 || sheets[0].Name != "Сотрудники" {
		t.Fatalf("unexpected sheets: %+v", sheets)
	}
	if len(sheets[0].Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(sheets[0].Columns))
	}

	// 2. Process / Anonymize
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{}
	depMap, err := handler.Process(context.Background(), reader, int64(len(rawODS)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if depMap == nil || len(depMap.Entities) == 0 {
		t.Fatalf("expected non-empty depMap, got %+v", depMap)
	}

	anonymizedBytes := outBuf.Bytes()
	if bytes.Contains(anonymizedBytes, []byte("Иванов Иван Иванович")) {
		t.Errorf("anonymized ODS still contains original FIO")
	}

	// Verify styles.xml was preserved
	zr, err := zip.NewReader(bytes.NewReader(anonymizedBytes), int64(len(anonymizedBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}
	hasStyles := false
	for _, f := range zr.File {
		if f.Name == "styles.xml" {
			hasStyles = true
			break
		}
	}
	if !hasStyles {
		t.Errorf("anonymized ODS missing styles.xml")
	}

	// 3. Restore
	restoredBuf := new(bytes.Buffer)
	anonReader := bytes.NewReader(anonymizedBytes)
	count, err := RestoreODS(context.Background(), anonReader, int64(len(anonymizedBytes)), restoredBuf, depMap)
	if err != nil {
		t.Fatalf("RestoreODS failed: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 restored cells, got %d", count)
	}

	rzr, err := zip.NewReader(bytes.NewReader(restoredBuf.Bytes()), int64(restoredBuf.Len()))
	if err != nil {
		t.Fatalf("failed to open restored zip: %v", err)
	}
	restoredContent, err := readZipEntry(rzr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read restored content.xml: %v", err)
	}
	if !strings.Contains(string(restoredContent), "Иванов Иван Иванович") {
		t.Errorf("restored ODS does not contain original FIO")
	}
}

func TestODSHandler_SupportedExtensions(t *testing.T) {
	handler := &ODSHandler{}
	exts := handler.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".ods" {
		t.Fatalf("unexpected supported extensions: %v", exts)
	}
}

func TestODSHandler_DisabledColumns(t *testing.T) {
	rawODS := createTestODS(t)
	anon := anonymizer.New("test-salt-ods")
	handler := &ODSHandler{Anon: anon}

	reader := bytes.NewReader(rawODS)
	outBuf := new(bytes.Buffer)
	// Disable FIO column (column 1, 1-based)
	opts := map[string]interface{}{
		"disabled_columns": []int{1},
	}
	depMap, err := handler.Process(context.Background(), reader, int64(len(rawODS)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process with disabled columns failed: %v", err)
	}

	anonymizedBytes := outBuf.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(anonymizedBytes), int64(len(anonymizedBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}
	content, err := readZipEntry(zr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read content.xml: %v", err)
	}
	contentStr := string(content)

	// FIO should remain unchanged
	if !strings.Contains(contentStr, "Иванов Иван Иванович") {
		t.Errorf("expected FIO to be kept intact when column is disabled")
	}
	// But SNILS (column 2) should be anonymized
	if strings.Contains(contentStr, "112-233-445 95") {
		t.Errorf("expected SNILS to be anonymized")
	}

	if depMap.RecordsCount == 0 {
		t.Errorf("expected some anonymized records in depMap")
	}
}

func TestODSHandler_ContextCancellation(t *testing.T) {
	rawODS := createTestODS(t)
	anon := anonymizer.New("test-salt-ods")
	handler := &ODSHandler{Anon: anon}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	reader := bytes.NewReader(rawODS)
	outBuf := new(bytes.Buffer)
	_, err := handler.Process(ctx, reader, int64(len(rawODS)), outBuf, nil)
	if err == nil {
		t.Fatalf("expected error on cancelled context, got nil")
	}

	_, err = RestoreODS(ctx, reader, int64(len(rawODS)), outBuf, depmap.New("test.ods"))
	if err == nil {
		t.Fatalf("expected error on cancelled context in RestoreODS, got nil")
	}
}

func TestODSHandler_InvalidZip(t *testing.T) {
	anon := anonymizer.New("test-salt-ods")
	handler := &ODSHandler{Anon: anon}

	corrupt := []byte("not a zip file at all")
	reader := bytes.NewReader(corrupt)
	_, err := handler.Inspect(reader, int64(len(corrupt)))
	if err == nil {
		t.Fatalf("expected Inspect error on invalid zip, got nil")
	}

	outBuf := new(bytes.Buffer)
	_, err = handler.Process(context.Background(), reader, int64(len(corrupt)), outBuf, nil)
	if err == nil {
		t.Fatalf("expected Process error on invalid zip, got nil")
	}

	_, err = RestoreODS(context.Background(), reader, int64(len(corrupt)), outBuf, depmap.New("test.ods"))
	if err == nil {
		t.Fatalf("expected RestoreODS error on invalid zip, got nil")
	}
}

func createMultiSheetODS(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	mw, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Лист1">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Email</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Петров Петр Петрович</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>petrov@example.com</text:p></table:table-cell>
    </table:table-row>
   </table:table>
   <table:table table:name="Лист2">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Клиент</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>ИНН</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Сидоров Сидор</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>770123456789</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`

	cw, _ := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Store})
	cw.Write([]byte(contentXML))
	zw.Close()
	return buf.Bytes()
}

func TestODSHandler_MultipleSheets(t *testing.T) {
	raw := createMultiSheetODS(t)
	anon := anonymizer.New("test-salt-multi")
	handler := NewODSHandler(anon)

	reader := bytes.NewReader(raw)
	sheets, err := handler.Inspect(reader, int64(len(raw)))
	if err != nil {
		t.Fatalf("Inspect multi-sheet failed: %v", err)
	}
	if len(sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d", len(sheets))
	}
	if sheets[0].Name != "Лист1" || sheets[1].Name != "Лист2" {
		t.Errorf("unexpected sheet names: %+v", sheets)
	}

	// Anonymize only Sheet2
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"sheet": "Лист2",
	}
	dm, err := handler.Process(context.Background(), reader, int64(len(raw)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	anonBytes := outBuf.Bytes()
	azr, err := zip.NewReader(bytes.NewReader(anonBytes), int64(len(anonBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}
	anonContent, err := readZipEntry(azr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read content.xml: %v", err)
	}
	anonStr := string(anonContent)

	// Sheet 1 should be untouched
	if !strings.Contains(anonStr, "Петров Петр Петрович") {
		t.Errorf("Sheet1 was modified even though target was Sheet2")
	}
	// Sheet 2 should be anonymized
	if strings.Contains(anonStr, "Сидоров Сидор") {
		t.Errorf("Sheet2 client name was not anonymized")
	}

	// Restore
	restoredBuf := new(bytes.Buffer)
	count, err := RestoreODS(context.Background(), bytes.NewReader(anonBytes), int64(len(anonBytes)), restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreODS failed: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 restored cell, got %d", count)
	}

	rzr, err := zip.NewReader(bytes.NewReader(restoredBuf.Bytes()), int64(restoredBuf.Len()))
	if err != nil {
		t.Fatalf("failed to open restored zip: %v", err)
	}
	restoredContent, err := readZipEntry(rzr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read restored content.xml: %v", err)
	}
	if !strings.Contains(string(restoredContent), "Сидоров Сидор") {
		t.Errorf("restored ODS missing original client name")
	}
}

func TestODSHandler_ColumnOverridesAndProgress(t *testing.T) {
	raw := createTestODS(t)
	anon := anonymizer.New("test-salt-overrides")
	handler := &ODSHandler{Anon: anon}

	reader := bytes.NewReader(raw)
	outBuf := new(bytes.Buffer)
	progressCalled := 0
	opts := map[string]interface{}{
		"columns": map[string]string{
			"ФИО": "UNKNOWN", // Override FIO to unknown -> should NOT be anonymized
		},
		"progress_callback": func(row int) {
			progressCalled++
		},
	}

	dm, err := handler.Process(context.Background(), reader, int64(len(raw)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	anonBytes := outBuf.Bytes()
	azr, err := zip.NewReader(bytes.NewReader(anonBytes), int64(len(anonBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}
	anonContent, err := readZipEntry(azr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read content.xml: %v", err)
	}
	anonStr := string(anonContent)

	// FIO should NOT be anonymized because column was overridden to UNKNOWN
	if !strings.Contains(anonStr, "Иванов Иван Иванович") {
		t.Errorf("FIO was anonymized despite being overridden to UNKNOWN")
	}
	// SNILS should still be anonymized
	if strings.Contains(anonStr, "112-233-445 95") {
		t.Errorf("SNILS was not anonymized")
	}
	if dm.RecordsCount == 0 {
		t.Errorf("expected SNILS in depMap")
	}
}

func TestRestoreODS_ValidationErrors(t *testing.T) {
	raw := createTestODS(t)
	reader := bytes.NewReader(raw)
	buf := new(bytes.Buffer)
	dm := depmap.New("test.ods")

	if _, err := RestoreODS(context.Background(), nil, 100, buf, dm); err == nil {
		t.Errorf("expected error for nil reader")
	}
	if _, err := RestoreODS(context.Background(), reader, 100, nil, dm); err == nil {
		t.Errorf("expected error for nil writer")
	}
	if _, err := RestoreODS(context.Background(), reader, 100, buf, nil); err == nil {
		t.Errorf("expected error for nil depmap")
	}
	if _, err := RestoreODS(context.Background(), reader, 0, buf, dm); err == nil {
		t.Errorf("expected error for size <= 0")
	}
}

func createDateTestODS(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	mw, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Персонал">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Дата рождения</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Иванов Иван Иванович</text:p></table:table-cell>
     <table:table-cell office:value-type="date" office:date-value="1985-04-12"><text:p>12.04.1985</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`

	cw, _ := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Deflate})
	cw.Write([]byte(contentXML))
	zw.Close()
	return buf.Bytes()
}

func TestODSHandler_DateAttributeAndText(t *testing.T) {
	raw := createDateTestODS(t)
	anon := anonymizer.New("test-salt-date")
	handler := NewODSHandler(anon)

	reader := bytes.NewReader(raw)
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{}

	dm, err := handler.Process(context.Background(), reader, int64(len(raw)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	anonBytes := outBuf.Bytes()
	zr, err := zip.NewReader(bytes.NewReader(anonBytes), int64(len(anonBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}
	anonContent, err := readZipEntry(zr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read anonymized content.xml: %v", err)
	}

	anonStr := string(anonContent)
	// Verify neither original text nor original attribute is present
	if strings.Contains(anonStr, "12.04.1985") {
		t.Errorf("anonymized ODS content still contains original date text '12.04.1985'")
	}
	if strings.Contains(anonStr, "1985-04-12") {
		t.Errorf("anonymized ODS content still contains original office:date-value '1985-04-12'")
	}

	// Verify date attribute was updated
	if !strings.Contains(anonStr, "office:date-value=") {
		t.Errorf("anonymized ODS content missing office:date-value attribute")
	}

	// Restore
	restoredBuf := new(bytes.Buffer)
	count, err := RestoreODS(context.Background(), bytes.NewReader(anonBytes), int64(len(anonBytes)), restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreODS failed: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 restored cells (FIO and Date), got %d", count)
	}

	restoredZR, err := zip.NewReader(bytes.NewReader(restoredBuf.Bytes()), int64(restoredBuf.Len()))
	if err != nil {
		t.Fatalf("failed to open restored zip: %v", err)
	}
	restoredContent, err := readZipEntry(restoredZR, "content.xml")
	if err != nil {
		t.Fatalf("failed to read restored content.xml: %v", err)
	}
	restoredStr := string(restoredContent)

	if !strings.Contains(restoredStr, "12.04.1985") {
		t.Errorf("restored ODS content missing original date text '12.04.1985'")
	}
	if !strings.Contains(restoredStr, "1985-04-12") {
		t.Errorf("restored ODS content missing original office:date-value '1985-04-12'")
	}
	if !strings.Contains(restoredStr, "Иванов Иван Иванович") {
		t.Errorf("restored ODS content missing original FIO")
	}
}

