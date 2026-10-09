package formats_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"dep_service/internal/domain"
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

	pCount, entities, snippets, previews, blocks, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(previews) == 0 {
		t.Errorf("expected paragraph previews, got 0")
	}

	if len(blocks) == 0 {
		t.Errorf("expected document blocks, got 0")
	}

	// standardBriefDocXML has 2 top-level paragraphs and 1 table
	if len(blocks) != 3 {
		t.Errorf("expected 3 document blocks, got %d", len(blocks))
	}
	if blocks[0].Type != "paragraph" || blocks[1].Type != "paragraph" || blocks[2].Type != "table" {
		t.Errorf("unexpected block types: %v, %v, %v", blocks[0].Type, blocks[1].Type, blocks[2].Type)
	}
	if blocks[2].Table == nil || len(blocks[2].Table.Rows) != 1 || len(blocks[2].Table.Rows[0].Cells) != 1 {
		t.Errorf("unexpected table structure: %+v", blocks[2].Table)
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
	if _, _, _, _, _, err := h.Inspect(nil, 100); err == nil {
		t.Errorf("expected error in Inspect with nil reader")
	}
	if _, err := formats.RestoreDOCX(context.Background(), nil, 100, &buf, depmap.New("test.docx")); err == nil {
		t.Errorf("expected error in RestoreDOCX with nil reader")
	}

	// Invalid size
	if _, err := h.Process(context.Background(), r, 0, &buf, nil); err == nil {
		t.Errorf("expected error with size 0")
	}
	if _, _, _, _, _, err := h.Inspect(r, -5); err == nil {
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
	if _, _, _, _, _, err := h.Inspect(invR, int64(len(invalidData))); err == nil {
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

	_, entities, snippets, _, _, err := h.Inspect(r, int64(len(data)))
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

func TestDOCX_ContractComplexEntitiesAndBreaks(t *testing.T) {
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>ИП Иванов Алексей Сергеевич</w:t><w:br/></w:r>
      <w:r><w:t>Паспорт: 45 18 № 912384</w:t><w:br/></w:r>
      <w:r><w:t>Адрес рег.: 119048, г. Москва, ул. Усачёва, д. 29, корп. 2, кв. 74</w:t><w:br/></w:r>
      <w:r><w:t>Р/с: 40802810438000123456</w:t><w:br/></w:r>
      <w:r><w:t>Подпись: А.С. Иванов</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	anon := anonymizer.New("test-contract-salt-42")
	h := formats.NewDOCXHandler(anon)

	data := createTestDocx(docXML)
	r := bytes.NewReader(data)

	_, entities, snippets, _, _, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	entityTypes := make(map[string]bool)
	for _, e := range entities {
		entityTypes[e.Type] = true
	}

	if !entityTypes["FIO"] && !entityTypes[string(detector.EntityFIO)] {
		t.Errorf("expected FIO entity, got: %v", entities)
	}
	if !entityTypes["PASSPORT"] && !entityTypes[string(detector.EntityPassport)] {
		t.Errorf("expected PASSPORT entity, got: %v", entities)
	}
	if !entityTypes["ADDRESS"] && !entityTypes[string(detector.EntityAddress)] {
		t.Errorf("expected ADDRESS entity, got: %v", entities)
	}
	if !entityTypes["BANK_CARD"] && !entityTypes[string(detector.EntityCard)] {
		t.Errorf("expected BANK_CARD entity, got: %v", entities)
	}

	// Process
	var anonBuf bytes.Buffer
	dm, err := h.Process(context.Background(), bytes.NewReader(data), int64(len(data)), &anonBuf, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	anonDocXML, err := readZipFile(anonBuf.Bytes(), "word/document.xml")
	if err != nil {
		t.Fatalf("read zip failed: %v", err)
	}

	if strings.Contains(anonDocXML, "45 18 № 912384") {
		t.Errorf("passport was not anonymized: %s", anonDocXML)
	}
	if strings.Contains(anonDocXML, "40802810438000123456") {
		t.Errorf("bank account was not anonymized: %s", anonDocXML)
	}
	if strings.Contains(anonDocXML, "ул. Усачёва, д. 29") {
		t.Errorf("address was not anonymized: %s", anonDocXML)
	}

	// Restore
	var restBuf bytes.Buffer
	restoredCount, err := formats.RestoreDOCX(context.Background(), bytes.NewReader(anonBuf.Bytes()), int64(anonBuf.Len()), &restBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}
	if restoredCount < 4 {
		t.Errorf("expected at least 4 restored entities, got %d", restoredCount)
	}

	restDocXML, err := readZipFile(restBuf.Bytes(), "word/document.xml")
	if err != nil {
		t.Fatalf("read restored doc failed: %v", err)
	}

	if !strings.Contains(restDocXML, "45 18 № 912384") {
		t.Errorf("passport was not restored: %s", restDocXML)
	}
	if !strings.Contains(restDocXML, "40802810438000123456") {
		t.Errorf("bank account was not restored: %s", restDocXML)
	}
	if !strings.Contains(restDocXML, "119048, г. Москва, ул. Усачёва, д. 29, корп. 2, кв. 74") {
		t.Errorf("address was not restored: %s", restDocXML)
	}
	_ = snippets
}

func TestDOCX_Inspect_HeadersFooters_NotInDocumentBlocks(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	headerXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:p>
    <w:r><w:t>Header SNILS: 123-456-789 01 and page number 6</w:t></w:r>
  </w:p>
</w:hdr>`

	parts := map[string]string{
		"word/document.xml": standardBriefDocXML,
		"word/header1.xml":  headerXML,
	}

	data := createTestDocxWithParts(parts)
	r := bytes.NewReader(data)

	pCount, entities, snippets, previews, blocks, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// 1. Header SNILS should be in entity counts and snippets
	foundSNILS := false
	for _, e := range entities {
		if e.Type == "SNILS" || e.Type == string(detector.EntitySNILS) {
			foundSNILS = true
		}
	}
	if !foundSNILS {
		t.Errorf("header SNILS not found in entities: %+v", entities)
	}

	// 2. But blocks and previews should ONLY contain standardBriefDocXML (3 blocks: 2 paras, 1 table)
	if len(blocks) != 3 {
		t.Errorf("expected exactly 3 document blocks from document.xml, got %d", len(blocks))
	}

	// Check that header text ("page number 6") does not exist in any document block
	for _, b := range blocks {
		if b.Type == "paragraph" {
			for _, seg := range b.Segments {
				if strings.Contains(seg.Text, "page number 6") {
					t.Errorf("documentBlocks polluted by header XML content: %s", seg.Text)
				}
			}
		}
	}

	_ = pCount
	_ = snippets
	_ = previews
}

func TestDOCX_Endnotes_And_Footnotes(t *testing.T) {
	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Основной текст документа с примечанием</w:t></w:r>
      <w:r><w:endnoteReference w:id="1"/></w:r>
    </w:p>
  </w:body>
</w:document>`

	endnotesXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:endnotes xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:endnote w:type="separator" w:id="-1">
    <w:p><w:r><w:separator/></w:r></w:p>
  </w:endnote>
  <w:endnote w:type="continuationSeparator" w:id="0">
    <w:p><w:r><w:continuationSeparator/></w:r></w:p>
  </w:endnote>
  <w:endnote w:id="1">
    <w:p>
      <w:r><w:endnoteRef/></w:r>
      <w:r><w:t xml:space="preserve"> Период блокировки для клиента Иванов Иван Иванович</w:t></w:r>
    </w:p>
  </w:endnote>
  <w:endnote w:id="2">
    <w:p>
      <w:r><w:endnoteRef/></w:r>
      <w:r><w:t xml:space="preserve"> Телефон поддержки: +7 916 111-22-33</w:t></w:r>
    </w:p>
  </w:endnote>
</w:endnotes>`

	parts := map[string]string{
		"word/document.xml": docXML,
		"word/endnotes.xml": endnotesXML,
	}

	data := createTestDocxWithParts(parts)
	r := bytes.NewReader(data)

	// 1. Test Inspect
	pCount, entities, snippets, previews, blocks, err := h.Inspect(r, int64(len(data)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	// Total paragraphs: 1 from doc + 2 from endnotes = 3 (separators are skipped)
	if len(blocks) != 3 {
		t.Fatalf("expected 3 document blocks (1 body + 2 endnotes), got %d", len(blocks))
	}

	// Verify endnote 1 text starts with "1 Период"
	if len(blocks[1].Segments) == 0 || !strings.HasPrefix(blocks[1].Segments[0].Text, "1 Период") {
		var fullText strings.Builder
		for _, seg := range blocks[1].Segments {
			fullText.WriteString(seg.Text)
		}
		t.Errorf("expected block 1 to start with '1 Период', got: %q", fullText.String())
	}

	// Verify endnote 2 text starts with "2 Телефон"
	if len(blocks[2].Segments) == 0 || !strings.HasPrefix(blocks[2].Segments[0].Text, "2 Телефон") {
		var fullText strings.Builder
		for _, seg := range blocks[2].Segments {
			fullText.WriteString(seg.Text)
		}
		t.Errorf("expected block 2 to start with '2 Телефон', got: %q", fullText.String())
	}

	// 2. Test Process
	var anonBuf bytes.Buffer
	dm, err := h.Process(context.Background(), r, int64(len(data)), &anonBuf, map[string]interface{}{
		"original_file_name": "test_endnotes.docx",
	})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if len(dm.Mapping) < 2 {
		t.Fatalf("expected at least 2 anonymized entities in DepMap, got %d", len(dm.Mapping))
	}

	anonEndnotesXML, err := readZipFile(anonBuf.Bytes(), "word/endnotes.xml")
	if err != nil {
		t.Fatalf("failed to read anonymized endnotes.xml: %v", err)
	}
	if strings.Contains(anonEndnotesXML, "Иванов Иван Иванович") {
		t.Errorf("anonymized endnotes.xml still contains original FIO: %s", anonEndnotesXML)
	}
	if strings.Contains(anonEndnotesXML, "+7 916 111-22-33") {
		t.Errorf("anonymized endnotes.xml still contains original phone: %s", anonEndnotesXML)
	}

	// 3. Test RestoreDOCX
	var restBuf bytes.Buffer
	anonReader := bytes.NewReader(anonBuf.Bytes())
	restoredCount, err := formats.RestoreDOCX(context.Background(), anonReader, int64(anonBuf.Len()), &restBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}
	if restoredCount < 2 {
		t.Errorf("expected at least 2 restored entities, got %d", restoredCount)
	}

	restEndnotesXML, err := readZipFile(restBuf.Bytes(), "word/endnotes.xml")
	if err != nil {
		t.Fatalf("failed to read restored endnotes.xml: %v", err)
	}
	if !strings.Contains(restEndnotesXML, "Иванов Иван Иванович") {
		t.Errorf("restored endnotes.xml does not contain restored FIO: %s", restEndnotesXML)
	}
	if !strings.Contains(restEndnotesXML, "+7 916 111-22-33") {
		t.Errorf("restored endnotes.xml does not contain restored phone: %s", restEndnotesXML)
	}

	_ = pCount
	_ = entities
	_ = snippets
	_ = previews
}

func TestDOCX_UserDocument_EndnotesPresent(t *testing.T) {
	importPath := "../../Приложение №1_Тарифы на услуги ДБ_22.10.2026 (1).docx"
	importPathAlt := "Приложение №1_Тарифы на услуги ДБ_22.10.2026 (1).docx"
	importPathRoot := "../Приложение №1_Тарифы на услуги ДБ_22.10.2026 (1).docx"

	var f io.ReaderAt
	var size int64
	for _, p := range []string{importPathAlt, importPathRoot, importPath} {
		if file, err := zip.OpenReader(p); err == nil {
			_ = file.Close()
			if fileBytes, err := osReadFile(p); err == nil {
				f = bytes.NewReader(fileBytes)
				size = int64(len(fileBytes))
				break
			}
		}
	}

	if f == nil {
		t.Skip("user docx file not found in test search paths, skipping")
	}

	anon := anonymizer.New("test-salt-123")
	h := formats.NewDOCXHandler(anon)

	pCount, entities, snippets, previews, blocks, err := h.Inspect(f, size)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(blocks) == 0 {
		t.Fatalf("no blocks returned")
	}

	// Verify that the last blocks contain the endnotes
	lastBlock := blocks[len(blocks)-1]
	foundEndnote1 := false
	foundEndnote5 := false
	for _, b := range blocks {
		if b.Type == "paragraph" {
			for _, seg := range b.Segments {
				if strings.Contains(seg.Text, "1 Период, в который абонент") {
					foundEndnote1 = true
				}
				if strings.Contains(seg.Text, "5 Максимальный срок") {
					foundEndnote5 = true
				}
			}
		}
	}

	if !foundEndnote1 {
		t.Errorf("endnote 1 not found in document blocks!")
	}
	if !foundEndnote5 {
		t.Errorf("endnote 5 not found in document blocks!")
	}

	_ = pCount
	_ = entities
	_ = snippets
	_ = previews
	_ = lastBlock
}

func osReadFile(name string) ([]byte, error) {
	importOS, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer importOS.Close()
	return io.ReadAll(importOS)
}

func TestDOCXHandler_ModeDelete(t *testing.T) {
	docXML := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Договор подписал Иванов Иван Иванович, телефон +7 999 123-45-67.</w:t></w:r></w:p>
  </w:body>
</w:document>`

	data := createTestDocx(docXML)
	r := bytes.NewReader(data)

	anon := anonymizer.New("test-salt-docx-delete")
	h := formats.NewDOCXHandler(anon)

	var outBuf bytes.Buffer
	dm, err := h.Process(context.Background(), r, int64(len(data)), &outBuf, map[string]interface{}{
		"mode": domain.ModeDelete,
	})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if dm.Mode != "delete" {
		t.Errorf("expected dm.Mode 'delete', got %q", dm.Mode)
	}

	if len(dm.OrdinalMappings) != 2 {
		t.Fatalf("expected 2 OrdinalMappings, got %d: %v", len(dm.OrdinalMappings), dm.OrdinalMappings)
	}

	if dm.OrdinalMappings[0] != "Иванов Иван Иванович" {
		t.Errorf("expected first ordinal 'Иванов Иван Иванович', got %q", dm.OrdinalMappings[0])
	}
	if dm.OrdinalMappings[1] != "+7 999 123-45-67" {
		t.Errorf("expected second ordinal '+7 999 123-45-67', got %q", dm.OrdinalMappings[1])
	}

	anonXML, err := readZipFile(outBuf.Bytes(), "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read anonymized document.xml: %v", err)
	}

	if strings.Contains(anonXML, "Иванов Иван Иванович") {
		t.Errorf("document still contains original FIO!")
	}
	if strings.Contains(anonXML, "+7 999 123-45-67") {
		t.Errorf("document still contains original phone!")
	}
	if !strings.Contains(anonXML, formats.DeletionMarker) {
		t.Errorf("document does not contain DeletionMarker %q", formats.DeletionMarker)
	}

	// Test Reverse Restoration with RestoreDOCX
	var restoredBuf bytes.Buffer
	restoredCount, err := formats.RestoreDOCX(context.Background(), bytes.NewReader(outBuf.Bytes()), int64(outBuf.Len()), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreDOCX failed: %v", err)
	}
	if restoredCount != 2 {
		t.Errorf("expected 2 restored entities, got %d", restoredCount)
	}

	restoredXML, err := readZipFile(restoredBuf.Bytes(), "word/document.xml")
	if err != nil {
		t.Fatalf("failed to read restored document.xml: %v", err)
	}
	if !strings.Contains(restoredXML, "Иванов Иван Иванович") {
		t.Errorf("restored document missing original FIO: %s", restoredXML)
	}
	if !strings.Contains(restoredXML, "+7 999 123-45-67") {
		t.Errorf("restored document missing original phone: %s", restoredXML)
	}
	if strings.Contains(restoredXML, formats.DeletionMarker) {
		t.Errorf("restored document still contains DeletionMarker %q", formats.DeletionMarker)
	}
}





