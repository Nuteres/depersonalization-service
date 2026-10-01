package formats_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

func createTestDocx(docXML string) []byte {
	return createTestDocxWithParts(map[string]string{
		"word/document.xml": docXML,
	})
}

func createTestDocxWithParts(parts map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	w, err := zw.Create("[Content_Types].xml")
	if err == nil {
		_, _ = w.Write([]byte(contentTypes))
	}

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	w, err = zw.Create("_rels/.rels")
	if err == nil {
		_, _ = w.Write([]byte(rels))
	}

	for name, content := range parts {
		w, err = zw.Create(name)
		if err == nil {
			_, _ = w.Write([]byte(content))
		}
	}

	_ = zw.Close()
	return buf.Bytes()
}

func readZipFile(data []byte, name string) (string, error) {
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
			b, err := io.ReadAll(rc)
			return string(b), err
		}
	}
	return "", io.EOF
}

const standardBriefDocXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:t>Договор подписал Иванов Иван Иванович</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:t>Паспорт: 45</w:t>
      </w:r>
      <w:r>
        <w:t>10 123890</w:t>
      </w:r>
    </w:p>
    <w:tbl>
      <w:tr>
        <w:tc>
          <w:p>
            <w:r>
              <w:t>+7 916 123-45-67</w:t>
            </w:r>
          </w:p>
        </w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`

func TestDOCX_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)
	exts := h.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".docx" {
		t.Fatalf("expected [.docx], got %v", exts)
	}
}

func TestDOCX_Inspect(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	data := createTestDocx(standardBriefDocXML)
	r := bytes.NewReader(data)

	pCount, entities, snippets, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if pCount != 3 {
		t.Errorf("expected 3 paragraphs, got %d", pCount)
	}

	// Verify detected entities
	entityMap := make(map[string]int)
	for _, ec := range entities {
		entityMap[ec.Type] = ec.Count
	}

	if entityMap[string(detector.EntityFIO)] < 1 && entityMap["FIO"] < 1 {
		t.Errorf("expected at least 1 FIO entity, got map: %v", entityMap)
	}
	if entityMap[string(detector.EntityPassport)] < 1 && entityMap["PASSPORT"] < 1 {
		t.Errorf("expected at least 1 PASSPORT entity, got map: %v", entityMap)
	}
	if entityMap[string(detector.EntityPhone)] < 1 && entityMap["PHONE"] < 1 {
		t.Errorf("expected at least 1 PHONE entity, got map: %v", entityMap)
	}

	// Verify snippets
	if len(snippets) < 3 {
		t.Errorf("expected at least 3 snippets, got %d", len(snippets))
	}

	foundFIO, foundPassport, foundPhone := false, false, false
	for _, s := range snippets {
		if strings.Contains(s.Original, "Иванов Иван Иванович") {
			foundFIO = true
			if s.Anonymized == "" || s.Anonymized == s.Original {
				t.Errorf("snippet Anonymized should differ from original for FIO: %+v", s)
			}
		}
		if strings.Contains(s.Original, "4510 123890") || strings.Contains(s.Original, "45 10 123890") {
			foundPassport = true
		}
		if strings.Contains(s.Original, "+7 916 123-45-67") {
			foundPhone = true
		}
	}

	if !foundFIO {
		t.Errorf("FIO snippet not found: %+v", snippets)
	}
	if !foundPassport {
		t.Errorf("Passport snippet not found: %+v", snippets)
	}
	if !foundPhone {
		t.Errorf("Phone snippet not found: %+v", snippets)
	}
}

func TestDOCX_Process_And_Restore(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	data := createTestDocx(standardBriefDocXML)
	r := bytes.NewReader(data)
	var outBuf bytes.Buffer

	dm, err := h.Process(context.Background(), r, int64(len(data)), &outBuf, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if dm == nil {
		t.Fatalf("expected non-nil DepMap")
	}
	if dm.RecordsCount < 3 {
		t.Errorf("expected at least 3 records in DepMap, got %d", dm.RecordsCount)
	}

	anonData := outBuf.Bytes()

	// Verify PII is gone from the anonymized document
	anonDocXML, err := readZipFile(anonData, "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read word/document.xml from output: %v", err)
	}

	if strings.Contains(anonDocXML, "Иванов Иван Иванович") {
		t.Errorf("anonymized XML still contains FIO")
	}
	if strings.Contains(anonDocXML, "4510 123890") {
		t.Errorf("anonymized XML still contains Passport")
	}
	if strings.Contains(anonDocXML, "+7 916 123-45-67") {
		t.Errorf("anonymized XML still contains Phone")
	}

	// Verify XML is well-formed
	var docStruct interface{}
	if err := xml.Unmarshal([]byte(anonDocXML), &docStruct); err != nil {
		t.Fatalf("anonymized document.xml is not valid XML: %v\nXML:\n%s", err, anonDocXML)
	}

	// Verify split run handling:
	// Second run of paragraph 2 should have empty text node <w:t></w:t> or <w:t/>
	// while the synthetic passport is in the first run!
	if !strings.Contains(anonDocXML, "Паспорт:") {
		t.Errorf("label 'Паспорт:' should be preserved in anonymized XML: %s", anonDocXML)
	}

	// Now Restore
	anonReader := bytes.NewReader(anonData)
	var restoredBuf bytes.Buffer

	restoredCount, err := formats.RestoreDOCX(context.Background(), anonReader, int64(len(anonData)), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}

	if restoredCount < 3 {
		t.Errorf("expected at least 3 restored entities, got %d", restoredCount)
	}

	restoredData := restoredBuf.Bytes()
	restoredDocXML, err := readZipFile(restoredData, "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read word/document.xml from restored doc: %v", err)
	}

	// Verify verbatim restoration
	if !strings.Contains(restoredDocXML, "Иванов Иван Иванович") {
		t.Errorf("restored XML missing original FIO: %s", restoredDocXML)
	}
	if !strings.Contains(restoredDocXML, "4510 123890") && !strings.Contains(restoredDocXML, "45") {
		t.Errorf("restored XML missing original Passport: %s", restoredDocXML)
	}
	if !strings.Contains(restoredDocXML, "+7 916 123-45-67") {
		t.Errorf("restored XML missing original Phone: %s", restoredDocXML)
	}

	// Verify restored XML is well-formed
	if err := xml.Unmarshal([]byte(restoredDocXML), &docStruct); err != nil {
		t.Fatalf("restored document.xml is not valid XML: %v\nXML:\n%s", err, restoredDocXML)
	}
}

func TestDOCX_Headers_Footers_And_Media(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	headerXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:r><w:t>СНИЛС сотрудника: 123-456-789 01</w:t></w:r>
  </w:p>
</w:hdr>`

	footerXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:r><w:t>Связь: director@company.ru</w:t></w:r>
  </w:p>
</w:ftr>`

	imageContent := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x01, 0x02}

	parts := map[string]string{
		"word/document.xml":    standardBriefDocXML,
		"word/header1.xml":     headerXML,
		"word/footer1.xml":     footerXML,
		"word/media/image.png": string(imageContent),
	}

	data := createTestDocxWithParts(parts)
	r := bytes.NewReader(data)
	var outBuf bytes.Buffer

	dm, err := h.Process(context.Background(), r, int64(len(data)), &outBuf, nil)
	if err != nil {
		t.Fatalf("Process with headers/footers failed: %v", err)
	}

	anonData := outBuf.Bytes()

	// Check header anonymization
	anonHdr, err := readZipFile(anonData, "word/header1.xml")
	if err != nil {
		t.Fatalf("failed to read word/header1.xml: %v", err)
	}
	if strings.Contains(anonHdr, "123-456-789 01") {
		t.Errorf("header still contains original SNILS")
	}

	// Check footer anonymization
	anonFtr, err := readZipFile(anonData, "word/footer1.xml")
	if err != nil {
		t.Fatalf("failed to read word/footer1.xml: %v", err)
	}
	if strings.Contains(anonFtr, "director@company.ru") {
		t.Errorf("footer still contains original Email")
	}

	// Check media preserved intact
	anonImg, err := readZipFile(anonData, "word/media/image.png")
	if err != nil {
		t.Fatalf("failed to read word/media/image.png: %v", err)
	}
	if !bytes.Equal([]byte(anonImg), imageContent) {
		t.Errorf("media image was modified or corrupted")
	}

	// Restore and verify
	var restoredBuf bytes.Buffer
	_, err = formats.RestoreDOCX(context.Background(), bytes.NewReader(anonData), int64(len(anonData)), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}

	restoredHdr, _ := readZipFile(restoredBuf.Bytes(), "word/header1.xml")
	if !strings.Contains(restoredHdr, "123-456-789 01") {
		t.Errorf("restored header missing original SNILS: %s", restoredHdr)
	}

	restoredFtr, _ := readZipFile(restoredBuf.Bytes(), "word/footer1.xml")
	if !strings.Contains(restoredFtr, "director@company.ru") {
		t.Errorf("restored footer missing original Email: %s", restoredFtr)
	}
}

func TestDOCX_DisabledEntityTypes(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	data := createTestDocx(standardBriefDocXML)
	r := bytes.NewReader(data)
	var outBuf bytes.Buffer

	options := map[string]interface{}{
		"disabled_entities": []string{"PASSPORT"},
	}

	dm, err := h.Process(context.Background(), r, int64(len(data)), &outBuf, options)
	if err != nil {
		t.Fatalf("Process with disabled entity failed: %v", err)
	}

	anonData := outBuf.Bytes()
	anonDocXML, err := readZipFile(anonData, "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read document.xml: %v", err)
	}

	// FIO should be anonymized
	if strings.Contains(anonDocXML, "Иванов Иван Иванович") {
		t.Errorf("FIO should be anonymized")
	}
	// Phone should be anonymized
	if strings.Contains(anonDocXML, "+7 916 123-45-67") {
		t.Errorf("Phone should be anonymized")
	}
	// Passport should NOT be anonymized because it was disabled!
	if !strings.Contains(anonDocXML, "45") || !strings.Contains(anonDocXML, "10 123890") {
		t.Errorf("Passport should be preserved when disabled: %s", anonDocXML)
	}

	// Ensure dm does not have passport mapping
	for _, orig := range dm.Mapping {
		if strings.Contains(orig, "4510") || strings.Contains(orig, "123890") {
			t.Errorf("dm should not map disabled passport entity: %s", orig)
		}
	}
}

func TestDOCX_MultipleEntitiesInSingleParagraph(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Сотрудник Иванов Иван Иванович, СНИЛС 123-456-789 01, тел. +7 916 123-45-67</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	data := createTestDocx(docXML)
	r := bytes.NewReader(data)
	var outBuf bytes.Buffer

	dm, err := h.Process(context.Background(), r, int64(len(data)), &outBuf, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	anonData := outBuf.Bytes()
	anonDocXML, _ := readZipFile(anonData, "word/document.xml")

	if strings.Contains(anonDocXML, "Иванов Иван Иванович") {
		t.Errorf("FIO not anonymized")
	}
	if strings.Contains(anonDocXML, "123-456-789 01") {
		t.Errorf("SNILS not anonymized")
	}
	if strings.Contains(anonDocXML, "+7 916 123-45-67") {
		t.Errorf("Phone not anonymized")
	}

	// Restore
	var restoredBuf bytes.Buffer
	_, err = formats.RestoreDOCX(context.Background(), bytes.NewReader(anonData), int64(len(anonData)), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	restoredDocXML, _ := readZipFile(restoredBuf.Bytes(), "word/document.xml")
	if !strings.Contains(restoredDocXML, "Иванов Иван Иванович") ||
		!strings.Contains(restoredDocXML, "123-456-789 01") ||
		!strings.Contains(restoredDocXML, "+7 916 123-45-67") {
		t.Errorf("restored paragraph missing entities: %s", restoredDocXML)
	}
}

func TestDOCX_EdgeCases(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)
	nilHandler := formats.NewDOCXHandler(nil)

	data := createTestDocx(standardBriefDocXML)
	r := bytes.NewReader(data)
	var buf bytes.Buffer

	// Nil anonymizer
	if _, err := nilHandler.Process(context.Background(), r, int64(len(data)), &buf, nil); err == nil {
		t.Errorf("expected error with nil anonymizer")
	}

	// Nil reader
	if _, err := h.Process(context.Background(), nil, 100, &buf, nil); err == nil {
		t.Errorf("expected error with nil reader")
	}
	if _, _, _, err := h.Inspect(nil, 100); err == nil {
		t.Errorf("expected error in Inspect with nil reader")
	}
	if _, err := formats.RestoreDOCX(context.Background(), nil, 100, &buf, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error in RestoreDOCX with nil reader")
	}

	// Invalid size
	if _, err := h.Process(context.Background(), r, 0, &buf, nil); err == nil {
		t.Errorf("expected error with size 0")
	}
	if _, _, _, err := h.Inspect(r, -5); err == nil {
		t.Errorf("expected error in Inspect with negative size")
	}
	if _, err := formats.RestoreDOCX(context.Background(), r, 0, &buf, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error in RestoreDOCX with size 0")
	}

	// Nil writer
	if _, err := h.Process(context.Background(), r, int64(len(data)), nil, nil); err == nil {
		t.Errorf("expected error with nil writer")
	}
	if _, err := formats.RestoreDOCX(context.Background(), r, int64(len(data)), nil, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error in RestoreDOCX with nil writer")
	}

	// Nil depmap in RestoreDOCX
	if _, err := formats.RestoreDOCX(context.Background(), r, int64(len(data)), &buf, nil); err == nil {
		t.Errorf("expected error in RestoreDOCX with nil depmap")
	}

	// Cancelled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Process(ctxCancel, r, int64(len(data)), &buf, nil); err == nil {
		t.Errorf("expected error with cancelled context in Process")
	}
	if _, err := formats.RestoreDOCX(ctxCancel, r, int64(len(data)), &buf, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error with cancelled context in RestoreDOCX")
	}

	// Invalid zip bytes
	invalidData := []byte("this is not a zip file at all")
	invR := bytes.NewReader(invalidData)
	if _, err := h.Process(context.Background(), invR, int64(len(invalidData)), &buf, nil); err == nil {
		t.Errorf("expected error with invalid zip in Process")
	}
	if _, _, _, err := h.Inspect(invR, int64(len(invalidData))); err == nil {
		t.Errorf("expected error with invalid zip in Inspect")
	}
	if _, err := formats.RestoreDOCX(context.Background(), invR, int64(len(invalidData)), &buf, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error with invalid zip in RestoreDOCX")
	}
}

func TestDOCX_ContractNumberNotFlaggedAsPassport(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Договор № 1234567890 от 15.01.2023</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	data := createTestDocx(docXML)
	r := bytes.NewReader(data)

	_, entities, snippets, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	for _, e := range entities {
		if e.Type == "PASSPORT" || e.Type == string(detector.EntityPassport) {
			t.Errorf("contract number should NOT be detected as PASSPORT: %+v", snippets)
		}
	}
}

func TestDOCX_RestoreCascadePrevention(t *testing.T) {
	// Suppose synthetic token A restores to an original string that contains synthetic token B.
	// In a sequential replacer, token B inside original A would be replaced by original B (cascade).
	// With single-pass replacement, token B inside original A is NOT replaced!
	synthA := "TokenAlpha99"
	origA := "RestoredAlpha with TokenBeta88 inside"
	synthB := "TokenBeta88"
	origB := "WRONG_CASCADE_REPLACED"

	dm := depmap.New("cascade.docx")
	dm.Put(synthA, origA)
	dm.Put(synthB, origB)

	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Target text: %s</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`, synthA)

	data := createTestDocx(docXML)
	var restoredBuf bytes.Buffer

	restoredCount, err := formats.RestoreDOCX(context.Background(), bytes.NewReader(data), int64(len(data)), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}

	if restoredCount != 1 {
		t.Errorf("expected exactly 1 restored entity (preventing cascade), got %d", restoredCount)
	}

	restoredDocXML, err := readZipFile(restoredBuf.Bytes(), "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read restored doc: %v", err)
	}

	if strings.Contains(restoredDocXML, "WRONG_CASCADE_REPLACED") {
		t.Errorf("cascade replacement occurred: %s", restoredDocXML)
	}
	if !strings.Contains(restoredDocXML, "TokenBeta88") {
		t.Errorf("expected 'TokenBeta88' inside original text to remain intact: %s", restoredDocXML)
	}
}

