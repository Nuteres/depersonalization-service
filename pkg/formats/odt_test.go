package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

func createTestODTArchive(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// 1. mimetype (must be first and stored uncompressed)
	mh := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mh)
	if err != nil {
		t.Fatalf("failed to create mimetype header: %v", err)
	}
	if _, err := mw.Write([]byte("application/vnd.oasis.opendocument.text")); err != nil {
		t.Fatalf("failed to write mimetype: %v", err)
	}

	// 2. content.xml with paragraphs, spans, tables, headers
	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0">
 <office:body>
  <office:text>
   <text:h text:outline-level="1">Трудовой договор</text:h>
   <text:p text:style-name="P1">Настоящий договор заключен с гражданином РФ Иванов Иван Иванович.</text:p>
   <text:p text:style-name="P2">Паспорт гражданина РФ: <text:span text:style-name="T1">4510</text:span> 123890, выдан ОВД г. Москвы.</text:p>
   <text:p text:style-name="P3">Страховое свидетельство (СНИЛС): 112-233-445 95, дата рождения: 15.06.1982.</text:p>
   <table:table table:name="Контакты">
    <table:table-row>
     <table:table-cell><text:p>Контактный телефон:</text:p></table:table-cell>
     <table:table-cell><text:p>+7 916 123-45-67</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:text>
 </office:body>
</office:document-content>`

	cw, err := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Deflate})
	if err != nil {
		t.Fatalf("failed to create content.xml: %v", err)
	}
	if _, err := cw.Write([]byte(contentXML)); err != nil {
		t.Fatalf("failed to write content.xml: %v", err)
	}

	// 3. styles.xml (header/footer)
	stylesXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:master-styles>
  <text:p>Колонтитул: ответственный сотрудник Петров Петр Петрович, тел. +7 925 555-44-33</text:p>
 </office:master-styles>
</office:document-styles>`

	sw, err := zw.CreateHeader(&zip.FileHeader{Name: "styles.xml", Method: zip.Deflate})
	if err != nil {
		t.Fatalf("failed to create styles.xml: %v", err)
	}
	if _, err := sw.Write([]byte(stylesXML)); err != nil {
		t.Fatalf("failed to write styles.xml: %v", err)
	}

	// 4. Dummy media file
	mw2, err := zw.CreateHeader(&zip.FileHeader{Name: "Pictures/100000000000010000000100ABCD.png", Method: zip.Store})
	if err != nil {
		t.Fatalf("failed to create media: %v", err)
	}
	if _, err := mw2.Write([]byte("FAKE_PNG_BYTES")); err != nil {
		t.Fatalf("failed to write media: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	return buf.Bytes()
}

func readODTEntry(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(rc)
			_ = rc.Close()
			return data, err
		}
	}
	return nil, fmt.Errorf("file %s not found in zip", name)
}

func TestODT_SupportedExtensions(t *testing.T) {
	h := NewODTHandler(nil)
	exts := h.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".odt" {
		t.Fatalf("expected [.odt], got %v", exts)
	}
}

func TestODT_Inspect(t *testing.T) {
	raw := createTestODTArchive(t)
	anon := anonymizer.New("test-salt-odt")
	h := NewODTHandler(anon)

	paraCount, entities, snippets, previews, err := h.Inspect(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(previews) == 0 {
		t.Errorf("expected paragraphPreviews, got 0")
	}

	if paraCount < 5 {
		t.Errorf("expected at least 5 paragraphs, got %d", paraCount)
	}

	if len(entities) == 0 {
		t.Errorf("expected detected entities, got none")
	}

	foundFIO := false
	foundPhone := false
	for _, e := range entities {
		if e.Type == "FIO" && e.Count > 0 {
			foundFIO = true
		}
		if e.Type == "PHONE" && e.Count > 0 {
			foundPhone = true
		}
	}
	if !foundFIO {
		t.Errorf("expected FIO in entity counts: %+v", entities)
	}
	if !foundPhone {
		t.Errorf("expected PHONE in entity counts: %+v", entities)
	}

	if len(snippets) == 0 {
		t.Errorf("expected non-empty snippets")
	}
}

func TestODT_Process_And_Restore(t *testing.T) {
	raw := createTestODTArchive(t)
	anon := anonymizer.New("test-salt-odt-roundtrip")
	h := NewODTHandler(anon)

	// Process / Anonymize
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{}
	dm, err := h.Process(context.Background(), bytes.NewReader(raw), int64(len(raw)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if dm == nil || dm.RecordsCount == 0 {
		t.Fatalf("expected non-empty depmap, got %+v", dm)
	}

	anonBytes := outBuf.Bytes()
	azr, err := zip.NewReader(bytes.NewReader(anonBytes), int64(len(anonBytes)))
	if err != nil {
		t.Fatalf("failed to open anonymized zip: %v", err)
	}

	// Verify mimetype is first and uncompressed
	if len(azr.File) == 0 || azr.File[0].Name != "mimetype" || azr.File[0].Method != zip.Store {
		t.Errorf("mimetype must be the first uncompressed entry")
	}

	// Verify content.xml anonymization
	contentData, err := readODTEntry(azr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read content.xml: %v", err)
	}
	contentStr := string(contentData)

	if strings.Contains(contentStr, "Иванов Иван Иванович") {
		t.Errorf("content.xml still contains original FIO")
	}
	if strings.Contains(contentStr, "112-233-445 95") {
		t.Errorf("content.xml still contains original SNILS")
	}
	if strings.Contains(contentStr, "+7 916 123-45-67") {
		t.Errorf("content.xml still contains original Phone")
	}

	// Verify styles.xml was also anonymized
	stylesData, err := readODTEntry(azr, "styles.xml")
	if err != nil {
		t.Fatalf("failed to read styles.xml: %v", err)
	}
	if strings.Contains(string(stylesData), "Петров Петр Петрович") {
		t.Errorf("styles.xml still contains original header FIO")
	}

	// Verify media bytes untouched
	mediaData, err := readODTEntry(azr, "Pictures/100000000000010000000100ABCD.png")
	if err != nil {
		t.Fatalf("failed to read media entry: %v", err)
	}
	if string(mediaData) != "FAKE_PNG_BYTES" {
		t.Errorf("media entry was altered: %q", string(mediaData))
	}

	// Restore
	restoredBuf := new(bytes.Buffer)
	count, err := RestoreODT(context.Background(), bytes.NewReader(anonBytes), int64(len(anonBytes)), restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreODT failed: %v", err)
	}
	if count < 3 {
		t.Errorf("expected at least 3 restored entities, got %d", count)
	}

	rzr, err := zip.NewReader(bytes.NewReader(restoredBuf.Bytes()), int64(restoredBuf.Len()))
	if err != nil {
		t.Fatalf("failed to open restored zip: %v", err)
	}

	restoredContent, err := readODTEntry(rzr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read restored content.xml: %v", err)
	}
	restoredContentStr := string(restoredContent)

	if !strings.Contains(restoredContentStr, "Иванов Иван Иванович") {
		t.Errorf("restored content.xml missing original FIO")
	}
	if !strings.Contains(restoredContentStr, "112-233-445 95") {
		t.Errorf("restored content.xml missing original SNILS")
	}
	if !strings.Contains(restoredContentStr, "+7 916 123-45-67") {
		t.Errorf("restored content.xml missing original Phone")
	}

	restoredStyles, err := readODTEntry(rzr, "styles.xml")
	if err != nil {
		t.Fatalf("failed to read restored styles.xml: %v", err)
	}
	if !strings.Contains(string(restoredStyles), "Петров Петр Петрович") {
		t.Errorf("restored styles.xml missing original header FIO")
	}
}

func TestODT_DisabledEntityTypes(t *testing.T) {
	raw := createTestODTArchive(t)
	anon := anonymizer.New("test-salt-odt-disabled")
	h := NewODTHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"disabled_entities": []string{"PHONE"},
	}

	_, err := h.Process(context.Background(), bytes.NewReader(raw), int64(len(raw)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	azr, err := zip.NewReader(bytes.NewReader(outBuf.Bytes()), int64(outBuf.Len()))
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	contentData, err := readODTEntry(azr, "content.xml")
	if err != nil {
		t.Fatalf("failed to read content: %v", err)
	}
	contentStr := string(contentData)

	// Phone should remain intact
	if !strings.Contains(contentStr, "+7 916 123-45-67") {
		t.Errorf("phone was anonymized despite being in disabled_entities")
	}
	// FIO should still be anonymized
	if strings.Contains(contentStr, "Иванов Иван Иванович") {
		t.Errorf("FIO was not anonymized")
	}
}

func TestODT_EdgeCases(t *testing.T) {
	raw := createTestODTArchive(t)
	anon := anonymizer.New("test-salt-odt-edge")
	h := NewODTHandler(anon)

	// Nil reader
	if _, _, _, _, err := h.Inspect(nil, 100); err == nil {
		t.Errorf("expected error on nil reader")
	}
	if _, err := h.Process(context.Background(), nil, 100, new(bytes.Buffer), nil); err == nil {
		t.Errorf("expected error on nil reader")
	}
	if _, err := RestoreODT(context.Background(), nil, 100, new(bytes.Buffer), depmap.New("t.odt")); err == nil {
		t.Errorf("expected error on nil reader in restore")
	}

	// Invalid size
	if _, _, _, _, err := h.Inspect(bytes.NewReader(raw), 0); err == nil {
		t.Errorf("expected error on 0 size")
	}

	// Nil anonymizer
	nilH := NewODTHandler(nil)
	if _, err := nilH.Process(context.Background(), bytes.NewReader(raw), int64(len(raw)), new(bytes.Buffer), nil); err == nil {
		t.Errorf("expected error on nil anonymizer")
	}

	// Corrupt zip
	corrupt := []byte("not a zip at all")
	if _, _, _, _, err := h.Inspect(bytes.NewReader(corrupt), int64(len(corrupt))); err == nil {
		t.Errorf("expected error on corrupt zip")
	}

	// Context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Process(ctx, bytes.NewReader(raw), int64(len(raw)), new(bytes.Buffer), nil); err == nil {
		t.Errorf("expected error on cancelled context")
	}
	if _, err := RestoreODT(ctx, bytes.NewReader(raw), int64(len(raw)), new(bytes.Buffer), depmap.New("t.odt")); err == nil {
		t.Errorf("expected error on cancelled context in restore")
	}
}
