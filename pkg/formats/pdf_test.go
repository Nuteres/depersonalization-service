package formats

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"dep_service/pkg/anonymizer"
)

// buildTestPDF constructs a syntactically valid PDF with the provided content stream.
func buildTestPDF(contentStream string, compress bool) []byte {
	var streamBytes []byte
	filterStr := ""
	if compress {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		_, _ = zw.Write([]byte(contentStream))
		_ = zw.Close()
		streamBytes = buf.Bytes()
		filterStr = "\n  /Filter /FlateDecode"
	} else {
		streamBytes = []byte(contentStream)
	}

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	offsets := make([]int, 6)

	// 1 0 obj: Catalog
	offsets[1] = b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// 2 0 obj: Pages
	offsets[2] = b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// 3 0 obj: Page
	offsets[3] = b.Len()
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")

	// 4 0 obj: Contents stream
	offsets[4] = b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<< /Length %d%s >>\nstream\n", len(streamBytes), filterStr)
	b.Write(streamBytes)
	b.WriteString("\nendstream\nendobj\n")

	// 5 0 obj: Font
	offsets[5] = b.Len()
	b.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// xref table
	startXref := b.Len()
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}

	// trailer
	b.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(&b, "%d\n%%%%EOF\n", startXref)

	return b.Bytes()
}

// extractAllStreamContents decompresses and extracts all stream data from a PDF byte slice.
func extractAllStreamContents(pdfData []byte) (string, error) {
	var combined strings.Builder
	idx := 0
	for {
		streamTag := []byte("stream")
		sPos := bytes.Index(pdfData[idx:], streamTag)
		if sPos == -1 {
			break
		}
		streamStart := idx + sPos + len(streamTag)
		// Skip \r\n or \n
		if streamStart < len(pdfData) && pdfData[streamStart] == '\r' {
			streamStart++
		}
		if streamStart < len(pdfData) && pdfData[streamStart] == '\n' {
			streamStart++
		}

		endstreamTag := []byte("endstream")
		ePos := bytes.Index(pdfData[streamStart:], endstreamTag)
		if ePos == -1 {
			break
		}
		streamEnd := streamStart + ePos
		// Trim trailing \r or \n
		rawStream := bytes.TrimRight(pdfData[streamStart:streamEnd], "\r\n")

		// Try decompressing with zlib
		zr, err := zlib.NewReader(bytes.NewReader(rawStream))
		if err == nil {
			decompressed, err := io.ReadAll(zr)
			_ = zr.Close()
			if err == nil {
				combined.Write(decompressed)
				combined.WriteString("\n")
				idx = streamEnd + len(endstreamTag)
				continue
			}
		}

		// Otherwise raw uncompressed
		combined.Write(rawStream)
		combined.WriteString("\n")
		idx = streamEnd + len(endstreamTag)
	}

	return combined.String(), nil
}

func TestPDFHandler_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt")
	h := NewPDFHandler(anon)
	exts := h.SupportedExtensions()
	if len(exts) == 0 {
		t.Fatal("expected at least one supported extension")
	}
	found := false
	for _, ext := range exts {
		if ext == ".pdf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .pdf in supported extensions, got %v", exts)
	}
}

func TestPDFHandler_Process_BlackoutRedaction(t *testing.T) {
	anon := anonymizer.New("test-salt-pdf")
	h := NewPDFHandler(anon)

	rawContent := `BT
/F1 12 Tf
72 700 Td
(Сотрудник: Иванов Иван Иванович) Tj
0 -20 Td
(СНИЛС: 112-233-445 95) Tj
0 -20 Td
(Паспорт: 4510 123890) Tj
0 -20 Td
(ИНН: 500100732259) Tj
0 -20 Td
(Телефон: +7 (999) 123-45-67) Tj
ET`

	inputPDF := buildTestPDF(rawContent, false)

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(inputPDF), &out, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	outputPDF := out.Bytes()
	if !bytes.HasPrefix(outputPDF, []byte("%PDF-")) {
		t.Fatal("output does not have valid PDF header")
	}

	extractedStreams, err := extractAllStreamContents(outputPDF)
	if err != nil {
		t.Fatalf("failed to extract streams from output: %v", err)
	}

	// 1. Sensitive text must be physically absent from output
	sensitiveStrings := []string{
		"112-233-445 95",
		"4510 123890",
		"500100732259",
		"+7 (999) 123-45-67",
		"Иванов Иван Иванович",
	}

	for _, s := range sensitiveStrings {
		if strings.Contains(extractedStreams, s) {
			t.Errorf("sensitive string %q is still physically present in PDF content stream!", s)
		}
		if bytes.Contains(outputPDF, []byte(s)) {
			t.Errorf("sensitive string %q is physically present in output PDF bytes!", s)
		}
	}

	// 2. Blackout redaction marker [ ПДн СКРЫТО ] must be present
	if !strings.Contains(extractedStreams, "[ ПДн СКРЫТО ]") {
		t.Errorf("blackout redaction marker '[ ПДн СКРЫТО ]' is missing from output streams: %s", extractedStreams)
	}

	// 3. Blackout rectangle operators (0 0 0 rg ... re f) must be present
	if !strings.Contains(extractedStreams, "re f") || !strings.Contains(extractedStreams, "0 0 0 rg") {
		t.Errorf("blackout solid rectangle operator (0 0 0 rg ... re f) is missing from output streams: %s", extractedStreams)
	}
}

func TestPDFHandler_Process_FlateCompressed(t *testing.T) {
	anon := anonymizer.New("test-salt-pdf-compressed")
	h := NewPDFHandler(anon)

	rawContent := `BT
/F1 14 Tf
100 750 Td
(Клиент: Смирнова Ольга Алексеевна) Tj
0 -25 Td
(СНИЛС: 112-233-445 95) Tj
0 -25 Td
(Паспорт РФ: 45 10 123890) Tj
ET`

	inputPDF := buildTestPDF(rawContent, true)

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(inputPDF), &out, nil)
	if err != nil {
		t.Fatalf("Process failed on compressed PDF: %v", err)
	}

	outputPDF := out.Bytes()
	extractedStreams, err := extractAllStreamContents(outputPDF)
	if err != nil {
		t.Fatalf("failed to extract streams: %v", err)
	}

	for _, s := range []string{"112-233-445 95", "45 10 123890", "Смирнова Ольга Алексеевна"} {
		if strings.Contains(extractedStreams, s) {
			t.Errorf("sensitive string %q found in compressed output stream!", s)
		}
		if bytes.Contains(outputPDF, []byte(s)) {
			t.Errorf("sensitive string %q found in raw output PDF bytes!", s)
		}
	}

	if !strings.Contains(extractedStreams, "[ ПДн СКРЫТО ]") {
		t.Errorf("blackout marker missing from compressed output streams: %s", extractedStreams)
	}
	if !strings.Contains(extractedStreams, "0 0 0 rg") || !strings.Contains(extractedStreams, "re f") {
		t.Errorf("blackout rectangle operator missing from compressed output streams")
	}
}

func TestPDFHandler_Process_TJOperator(t *testing.T) {
	anon := anonymizer.New("test-salt-tj")
	h := NewPDFHandler(anon)

	rawContent := `BT
/F1 12 Tf
50 600 Td
[(Номер СНИЛС: ) 10 (112-233-445 95)] TJ
0 -20 Td
[(Серия и номер: ) 10 (4510 123890)] TJ
ET`

	inputPDF := buildTestPDF(rawContent, false)

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(inputPDF), &out, nil)
	if err != nil {
		t.Fatalf("Process failed with TJ operator: %v", err)
	}

	extractedStreams, err := extractAllStreamContents(out.Bytes())
	if err != nil {
		t.Fatalf("failed to extract streams: %v", err)
	}

	for _, s := range []string{"112-233-445 95", "4510 123890"} {
		if strings.Contains(extractedStreams, s) {
			t.Errorf("sensitive string %q found in TJ output stream!", s)
		}
	}
	if !strings.Contains(extractedStreams, "[ ПДн СКРЫТО ]") {
		t.Errorf("blackout marker missing from TJ output streams")
	}
}

func TestPDFHandler_Process_CustomRedactTerms(t *testing.T) {
	anon := anonymizer.New("test-salt-custom")
	h := NewPDFHandler(anon)

	rawContent := `BT
/F1 12 Tf
72 700 Td
(Договор с ООО Альфа-Банк по проекту ПроектДельта) Tj
ET`

	inputPDF := buildTestPDF(rawContent, false)

	opts := map[string]interface{}{
		"redact_terms": []string{"ООО Альфа-Банк", "ПроектДельта"},
	}

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(inputPDF), &out, opts)
	if err != nil {
		t.Fatalf("Process failed with custom terms: %v", err)
	}

	extracted, err := extractAllStreamContents(out.Bytes())
	if err != nil {
		t.Fatalf("failed to extract streams: %v", err)
	}

	if strings.Contains(extracted, "ООО Альфа-Банк") {
		t.Errorf("custom term 'ООО Альфа-Банк' still present in stream!")
	}
	if strings.Contains(extracted, "ПроектДельта") {
		t.Errorf("custom term 'ПроектДельта' still present in stream!")
	}
	if !strings.Contains(extracted, "[ ПДн СКРЫТО ]") {
		t.Errorf("blackout marker missing for custom terms")
	}
}

func TestPDFHandler_NilAnonymizer(t *testing.T) {
	h := &PDFHandler{Anon: nil}
	err := h.Process(context.Background(), strings.NewReader("dummy"), io.Discard, nil)
	if err == nil {
		t.Fatal("expected error for nil anonymizer, got nil")
	}
}

func TestPDFHandler_InvalidPDF(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewPDFHandler(anon)
	err := h.Process(context.Background(), strings.NewReader("Not a PDF file"), io.Discard, nil)
	if err == nil {
		t.Fatal("expected error for non-PDF input, got nil")
	}
}

func TestPDFHandler_ContextCancellation(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewPDFHandler(anon)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	inputPDF := buildTestPDF(`BT /F1 12 Tf 72 700 Td (test) Tj ET`, false)
	err := h.Process(ctx, bytes.NewReader(inputPDF), io.Discard, nil)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestPDFHandler_TimeoutDuringProcessing(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewPDFHandler(anon)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)

	inputPDF := buildTestPDF(`BT /F1 12 Tf 72 700 Td (test) Tj ET`, false)
	err := h.Process(ctx, bytes.NewReader(inputPDF), io.Discard, nil)
	if err == nil {
		t.Fatal("expected context timeout error, got nil")
	}
}

func TestPDFHandler_Process_UnprefixedCyrillicFIO(t *testing.T) {
	anon := anonymizer.New("test-salt-fio-unprefixed")
	h := NewPDFHandler(anon)

	// Test names without any role prefixes like "Сотрудник:" or "ФИО:"
	rawContent := `BT
/F1 12 Tf
72 700 Td
(Иванов Иван Иванович) Tj
0 -20 Td
(Петрова Анна Сергеевна) Tj
0 -20 Td
(Сидоров-Михайлов Алексей Дмитриевич) Tj
ET`

	inputPDF := buildTestPDF(rawContent, false)

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(inputPDF), &out, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	extractedStreams, err := extractAllStreamContents(out.Bytes())
	if err != nil {
		t.Fatalf("failed to extract streams: %v", err)
	}

	unprefixedNames := []string{
		"Иванов Иван Иванович",
		"Петрова Анна Сергеевна",
		"Сидоров-Михайлов Алексей Дмитриевич",
	}

	for _, name := range unprefixedNames {
		if strings.Contains(extractedStreams, name) {
			t.Errorf("unprefixed Cyrillic name %q is still present in stream!", name)
		}
		if bytes.Contains(out.Bytes(), []byte(name)) {
			t.Errorf("unprefixed Cyrillic name %q is still present in output PDF bytes!", name)
		}
	}

	if !strings.Contains(extractedStreams, "[ ПДн СКРЫТО ]") {
		t.Errorf("blackout marker missing from stream after unprefixed FIO redaction")
	}
	if !strings.Contains(extractedStreams, "re f") {
		t.Errorf("blackout rectangle operator missing from stream")
	}
}

func TestPDFHandler_SkipNonContentStreams(t *testing.T) {
	anon := anonymizer.New("salt")
	h := NewPDFHandler(anon)

	// Construct a PDF that has a non-content stream with /Subtype /Image
	imageStream := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x0A} // Ends with 0x0A
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R >>\nendobj\n")
	b.WriteString("4 0 obj\n<< /Length 38 >>\nstream\nBT /F1 12 Tf 72 700 Td (Clean text) Tj ET\nendstream\nendobj\n")
	fmt.Fprintf(&b, "5 0 obj\n<< /Type /XObject /Subtype /Image /Width 1 /Height 1 /Length %d >>\nstream\n", len(imageStream))
	b.Write(imageStream)
	b.WriteString("\nendstream\nendobj\n")
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	b.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n500\n%%EOF\n")

	var out bytes.Buffer
	err := h.Process(context.Background(), bytes.NewReader(b.Bytes()), &out, nil)
	if err != nil {
		t.Fatalf("Process failed on PDF with image stream: %v", err)
	}

	// Verify image stream binary bytes were not corrupted or truncated
	if !bytes.Contains(out.Bytes(), imageStream) {
		t.Errorf("image stream binary bytes were modified or corrupted!")
	}
}
