package inspector_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
)

func TestInspect_EmptyPath(t *testing.T) {
	svc := inspector.New()
	_, err := svc.Inspect(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty path, got nil")
	}
}

func TestInspect_ContextCancelled(t *testing.T) {
	svc := inspector.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.Inspect(ctx, "some_file.csv")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestInspect_FileNotFound(t *testing.T) {
	svc := inspector.New()
	_, err := svc.Inspect(context.Background(), "/nonexistent/path/file.csv")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestInspect_DirectoryPath(t *testing.T) {
	tmpDir := t.TempDir()
	svc := inspector.New()
	_, err := svc.Inspect(context.Background(), tmpDir)
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got: %v", err)
	}
}

func TestInspectCSV_ContextCancelledDuringRead(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "large.csv")
	var content bytes.Buffer
	content.WriteString("ФИО,СНИЛС\n")
	for i := 0; i < 200; i++ {
		content.WriteString("Иванов Иван Иванович,112-233-445 95\n")
	}
	if err := os.WriteFile(csvPath, content.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc := inspector.New()
	_, err := svc.Inspect(ctx, csvPath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestInspect_UnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	txtFile := filepath.Join(tmpDir, "test.exe")
	if err := os.WriteFile(txtFile, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	svc := inspector.New()
	_, err := svc.Inspect(context.Background(), txtFile)
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat, got: %v", err)
	}
}

func TestInspectCSV(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := `ФИО,СНИЛС,ИНН,Телефон,Департамент
Иванов Иван Иванович,112-233-445 95,500100732259,+7 999 123-45-67,Инженерия
Петров Петр Петрович,112-233-445 95,500100732259,+7 916 234-56-78,Бухгалтерия
`
	csvPath := filepath.Join(tmpDir, "employees.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), csvPath)
	if err != nil {
		t.Fatalf("failed to inspect CSV: %v", err)
	}

	if info.Format != "csv" || info.FileType != "table" {
		t.Errorf("expected format csv and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
	if info.RowCount != 2 {
		t.Errorf("expected rowCount 2, got %d", info.RowCount)
	}
	if len(info.Headers) != 5 {
		t.Errorf("expected 5 headers, got %d", len(info.Headers))
	}
	if len(info.Entities) < 3 {
		t.Errorf("expected at least 3 detected entities, got %d", len(info.Entities))
	}
	if len(info.PreviewRows) != 2 {
		t.Errorf("expected 2 preview rows, got %d", len(info.PreviewRows))
	}
}

func TestInspectCSV_SemicolonDelimiter(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "ФИО;СНИЛС;ИНН\nСидоров Сидор Сидорович;112-233-445 95;500100732259\n"
	csvPath := filepath.Join(tmpDir, "semi.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), csvPath)
	if err != nil {
		t.Fatalf("failed to inspect CSV: %v", err)
	}

	if len(info.Headers) != 3 {
		t.Errorf("expected 3 headers for semicolon CSV, got %d (%v)", len(info.Headers), info.Headers)
	}
}

func TestInspectCSV_Windows1251(t *testing.T) {
	tmpDir := t.TempDir()
	csvContent := "ФИО,СНИЛС,ИНН,Department\nИванов Иван Иванович,112-233-445 95,500100732259,IT\n"
	win1251Bytes, err := charmap.Windows1251.NewEncoder().Bytes([]byte(csvContent))
	if err != nil {
		t.Fatalf("failed to encode windows-1251: %v", err)
	}
	csvPath := filepath.Join(tmpDir, "win1251_inspect.csv")
	if err := os.WriteFile(csvPath, win1251Bytes, 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), csvPath)
	if err != nil {
		t.Fatalf("failed to inspect Windows-1251 CSV: %v", err)
	}

	if len(info.Headers) != 4 {
		t.Fatalf("expected 4 headers, got %d (%v)", len(info.Headers), info.Headers)
	}
	if info.Headers[0] != "ФИО" {
		t.Errorf("expected header 'ФИО', got %q", info.Headers[0])
	}
	if info.Headers[3] != "Department" {
		t.Errorf("expected header 'Department', got %q", info.Headers[3])
	}
	if len(info.Entities) < 3 {
		t.Errorf("expected at least 3 detected entities from Windows-1251 CSV, got %d", len(info.Entities))
	}
}

func TestInspectXLSX(t *testing.T) {
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "test.xlsx")

	f := excelize.NewFile()
	sheet1 := "Лист 1"
	f.SetSheetName("Sheet1", sheet1)
	_ = f.SetCellValue(sheet1, "A1", "ФИО")
	_ = f.SetCellValue(sheet1, "B1", "Телефон")
	_ = f.SetCellValue(sheet1, "A2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet1, "B2", "+7 999 123-45-67")

	sheet2 := "Лист 2"
	f.NewSheet(sheet2)
	_ = f.SetCellValue(sheet2, "A1", "Email")
	_ = f.SetCellValue(sheet2, "A2", "test@example.com")

	if err := f.SaveAs(xlsxPath); err != nil {
		f.Close()
		t.Fatalf("failed to save xlsx: %v", err)
	}
	f.Close()

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), xlsxPath)
	if err != nil {
		t.Fatalf("failed to inspect XLSX: %v", err)
	}

	if info.Format != "xlsx" || info.FileType != "table" {
		t.Errorf("expected format xlsx and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
	if len(info.Sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d", len(info.Sheets))
	}
	if info.Sheets[0].Name != sheet1 {
		t.Errorf("expected first sheet name %s, got %s", sheet1, info.Sheets[0].Name)
	}
	if len(info.Entities) < 1 {
		t.Errorf("expected entities detected in primary sheet, got %d", len(info.Entities))
	}
}

func TestInspectODS(t *testing.T) {
	tmpDir := t.TempDir()
	odsPath := filepath.Join(tmpDir, "test.ods")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := zw.CreateHeader(mh)
	_, _ = mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Сотрудники">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Телефон</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Иванов Иван Иванович</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>+7 916 123-45-67</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`
	cw, _ := zw.Create("content.xml")
	_, _ = cw.Write([]byte(contentXML))
	_ = zw.Close()

	if err := os.WriteFile(odsPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write ods: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), odsPath)
	if err != nil {
		t.Fatalf("failed to inspect ODS: %v", err)
	}

	if info.Format != "ods" || info.FileType != "table" {
		t.Errorf("expected format ods and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
	if len(info.Sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(info.Sheets))
	}
	if len(info.Entities) < 1 {
		t.Errorf("expected detected entities, got %d", len(info.Entities))
	}
}

func biff12Le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func biff12WriteRec(buf *bytes.Buffer, id int, payload []byte) {
	for {
		b := id & 0xFF
		id >>= 8
		if id > 0 {
			buf.WriteByte(byte(b) | 0x80)
		} else {
			buf.WriteByte(byte(b) &^ 0x80)
			break
		}
	}
	n := len(payload)
	for {
		b := n & 0x7F
		n >>= 7
		if n > 0 {
			buf.WriteByte(byte(b) | 0x80)
		} else {
			buf.WriteByte(byte(b))
			break
		}
	}
	buf.Write(payload)
}

func biff12EncStr(s string) []byte {
	runes := []rune(s)
	var sb bytes.Buffer
	_ = binary.Write(&sb, binary.LittleEndian, uint32(len(runes)))
	for _, r := range runes {
		_ = binary.Write(&sb, binary.LittleEndian, uint16(r))
	}
	return sb.Bytes()
}

func createTestXLSB(t *testing.T, sheetName string, headers []string, dataRows [][]string) []byte {
	t.Helper()
	var allStrs []string
	strMap := make(map[string]uint32)
	addStr := func(s string) uint32 {
		if idx, ok := strMap[s]; ok {
			return idx
		}
		idx := uint32(len(allStrs))
		strMap[s] = idx
		allStrs = append(allStrs, s)
		return idx
	}

	for _, h := range headers {
		addStr(h)
	}
	for _, row := range dataRows {
		for _, cell := range row {
			addStr(cell)
		}
	}

	var sst bytes.Buffer
	sstPayload := make([]byte, 8)
	binary.LittleEndian.PutUint32(sstPayload[0:], uint32(len(allStrs)))
	binary.LittleEndian.PutUint32(sstPayload[4:], uint32(len(allStrs)))
	biff12WriteRec(&sst, 0x019F, sstPayload)
	for _, s := range allStrs {
		payload := append([]byte{0x00}, biff12EncStr(s)...)
		biff12WriteRec(&sst, 0x0013, payload)
	}
	biff12WriteRec(&sst, 0x01A0, nil)

	var wb bytes.Buffer
	biff12WriteRec(&wb, 0x0183, nil)
	biff12WriteRec(&wb, 0x018F, nil)
	var sheetRec bytes.Buffer
	sheetRec.Write(biff12Le32(0))
	sheetRec.Write(biff12Le32(1))
	sheetRec.Write(biff12EncStr("rId1"))
	sheetRec.Write(biff12EncStr(sheetName))
	biff12WriteRec(&wb, 0x019C, sheetRec.Bytes())
	biff12WriteRec(&wb, 0x0190, nil)
	biff12WriteRec(&wb, 0x0184, nil)

	var ws bytes.Buffer
	biff12WriteRec(&ws, 0x0181, nil)
	totalRows := uint32(1 + len(dataRows))
	totalCols := uint32(len(headers))
	var dim bytes.Buffer
	dim.Write(biff12Le32(0))
	dim.Write(biff12Le32(totalRows - 1))
	dim.Write(biff12Le32(0))
	dim.Write(biff12Le32(totalCols - 1))
	biff12WriteRec(&ws, 0x0194, dim.Bytes())
	biff12WriteRec(&ws, 0x0191, nil)

	biff12WriteRec(&ws, 0x0000, biff12Le32(0))
	for cIdx, h := range headers {
		var strCell bytes.Buffer
		strCell.Write(biff12Le32(uint32(cIdx)))
		strCell.Write(biff12Le32(0))
		strCell.Write(biff12Le32(strMap[h]))
		biff12WriteRec(&ws, 0x0007, strCell.Bytes())
	}

	for rIdx, row := range dataRows {
		biff12WriteRec(&ws, 0x0000, biff12Le32(uint32(rIdx+1)))
		for cIdx, val := range row {
			var strCell bytes.Buffer
			strCell.Write(biff12Le32(uint32(cIdx)))
			strCell.Write(biff12Le32(0))
			strCell.Write(biff12Le32(strMap[val]))
			biff12WriteRec(&ws, 0x0007, strCell.Bytes())
		}
	}
	biff12WriteRec(&ws, 0x0192, nil)
	biff12WriteRec(&ws, 0x0182, nil)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	relsXML := `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.bin"/>
<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.bin"/>
</Relationships>`
	addZipFile := func(name string, content []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip file %s: %v", name, err)
		}
		_, _ = w.Write(content)
	}
	addZipFile("xl/_rels/workbook.bin.rels", []byte(relsXML))
	addZipFile("xl/workbook.bin", wb.Bytes())
	addZipFile("xl/sharedStrings.bin", sst.Bytes())
	addZipFile("xl/worksheets/sheet1.bin", ws.Bytes())
	_ = zw.Close()
	return zipBuf.Bytes()
}

func TestInspectXLSB(t *testing.T) {
	tmpDir := t.TempDir()
	xlsbPath := filepath.Join(tmpDir, "test.xlsb")
	data := createTestXLSB(t, "Данные", []string{"ФИО", "СНИЛС"}, [][]string{
		{"Иванов Иван Иванович", "112-233-445 95"},
	})
	if err := os.WriteFile(xlsbPath, data, 0644); err != nil {
		t.Fatalf("failed to write xlsb: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), xlsbPath)
	if err != nil {
		t.Fatalf("failed to inspect XLSB: %v", err)
	}

	if info.Format != "xlsb" || info.FileType != "table" {
		t.Errorf("expected format xlsb and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
	if len(info.Sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(info.Sheets))
	}
	if len(info.Entities) < 1 {
		t.Errorf("expected detected entities, got %d", len(info.Entities))
	}
}

func TestInspectXLS(t *testing.T) {
	tmpDir := t.TempDir()
	xlsPath := filepath.Join(tmpDir, "test.xls")
	rawXLS, err := base64.StdEncoding.DecodeString(testXLSBase64)
	if err != nil {
		t.Fatalf("failed to decode base64 XLS: %v", err)
	}
	if err := os.WriteFile(xlsPath, rawXLS, 0644); err != nil {
		t.Fatalf("failed to write xls: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), xlsPath)
	if err != nil {
		t.Fatalf("failed to inspect XLS: %v", err)
	}

	if info.Format != "xls" || info.FileType != "table" {
		t.Errorf("expected format xls and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
	if len(info.Sheets) == 0 {
		t.Fatalf("expected at least 1 sheet in XLS, got 0")
	}
}

func createTestDocx(docXML string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	contentTypes := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`
	w, _ := zw.Create("[Content_Types].xml")
	_, _ = w.Write([]byte(contentTypes))

	rels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`
	w, _ = zw.Create("_rels/.rels")
	_, _ = w.Write([]byte(rels))

	w, _ = zw.Create("word/document.xml")
	_, _ = w.Write([]byte(docXML))

	_ = zw.Close()
	return buf.Bytes()
}

func TestInspectDOCX(t *testing.T) {
	tmpDir := t.TempDir()
	docxPath := filepath.Join(tmpDir, "test.docx")
	docXML := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r><w:t>Договор заключен с гражданином Иванов Иван Иванович.</w:t></w:r>
    </w:p>
    <w:p>
      <w:r><w:t>Телефон: +7 999 123-45-67, СНИЛС 112-233-445 95.</w:t></w:r>
    </w:p>
  </w:body>
</w:document>`

	if err := os.WriteFile(docxPath, createTestDocx(docXML), 0644); err != nil {
		t.Fatalf("failed to write docx: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), docxPath)
	if err != nil {
		t.Fatalf("failed to inspect DOCX: %v", err)
	}

	if info.Format != "docx" || info.FileType != "text" {
		t.Errorf("expected format docx and type text, got format=%s, type=%s", info.Format, info.FileType)
	}
	if info.ParagraphCount != 2 {
		t.Errorf("expected 2 paragraphs, got %d", info.ParagraphCount)
	}
	if len(info.Entities) == 0 {
		t.Errorf("expected detected entities in DOCX, got 0")
	}
	if len(info.TextSnippets) == 0 {
		t.Errorf("expected text snippets in DOCX, got 0")
	}
}

func TestInspectODT(t *testing.T) {
	tmpDir := t.TempDir()
	odtPath := filepath.Join(tmpDir, "test.odt")

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	mh := &zip.FileHeader{Name: "mimetype", Method: zip.Store}
	mw, _ := zw.CreateHeader(mh)
	_, _ = mw.Write([]byte("application/vnd.oasis.opendocument.text"))

	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:text>
   <text:p>Сотрудник: Петров Петр Петрович</text:p>
   <text:p>Номер телефона: +7 916 123-45-67</text:p>
  </office:text>
 </office:body>
</office:document-content>`
	cw, _ := zw.CreateHeader(&zip.FileHeader{Name: "content.xml", Method: zip.Deflate})
	_, _ = cw.Write([]byte(contentXML))
	_ = zw.Close()

	if err := os.WriteFile(odtPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write odt: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), odtPath)
	if err != nil {
		t.Fatalf("failed to inspect ODT: %v", err)
	}

	if info.Format != "odt" || info.FileType != "text" {
		t.Errorf("expected format odt and type text, got format=%s, type=%s", info.Format, info.FileType)
	}
	if info.ParagraphCount != 2 {
		t.Errorf("expected 2 paragraphs, got %d", info.ParagraphCount)
	}
	if len(info.Entities) == 0 {
		t.Errorf("expected detected entities in ODT, got 0")
	}
}

func TestODT_DocumentBlocks_And_Headings(t *testing.T) {
	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), "../../../mock_documents/02_trudovoy_dogovor_rukovoditel/02_trudovoy_dogovor_rukovoditel.odt")
	if err != nil {
		t.Skip("skipping mock document test if file not found")
		return
	}

	foundHeading := false
	foundFIOVoronov := false
	foundAddressTverskaya := false
	foundDateStart := false

	for _, b := range info.DocumentBlocks {
		if b.Type == "heading" {
			foundHeading = true
		}
		for _, seg := range b.Segments {
			if seg.IsEntity && seg.Type == "FIO" && strings.Contains(seg.Original, "Воронова") {
				foundFIOVoronov = true
			}
			if seg.IsEntity && seg.Type == "ADDRESS" && strings.Contains(seg.Original, "1-я Тверская-Ямская") {
				foundAddressTverskaya = true
				if strings.Contains(seg.Original, "Дата начала") {
					t.Errorf("address should not swallow 'Дата начала', got: %s", seg.Original)
				}
			}
			if seg.IsEntity && seg.Type == "DATE" && (seg.Original == "18.06.1982" || seg.Original == "18.04.2016") {
				foundDateStart = true
			}
		}
		if b.Table != nil {
			for _, r := range b.Table.Rows {
				for _, c := range r.Cells {
					for _, seg := range c.Segments {
						if seg.IsEntity && seg.Type == "DATE" && (seg.Original == "18.06.1982" || seg.Original == "18.04.2016") {
							foundDateStart = true
						}
					}
				}
			}
		}
	}

	if !foundHeading {
		t.Errorf("expected heading blocks in ODT, found none")
	}
	if !foundFIOVoronov {
		t.Errorf("expected FIO Воронова Сергея Алексеевича to be detected in ODT")
	}
	if !foundAddressTverskaya {
		t.Errorf("expected address on 1-я Тверская-Ямская to be detected in ODT")
	}
	if !foundDateStart {
		t.Errorf("expected date 18.06.1982 to be detected in ODT")
	}

	info2, err := svc.Inspect(context.Background(), "../../../mock_documents/05_akt_priema_peredachi_tmc/05_akt_priema_peredachi_tmc.odt")
	if err != nil {
		return
	}
	for _, b := range info2.DocumentBlocks {
		if b.Type == "table" && b.Table != nil {
			for rIdx, r := range b.Table.Rows {
				if rIdx == 0 && !r.IsHeader {
					t.Errorf("row 0 in equipment specification table should be marked as header")
				}
				if rIdx > 0 && r.IsHeader {
					t.Errorf("row %d in equipment specification table should not be marked as header", rIdx)
				}
			}
		}
	}
}

func TestInspect_MockDocuments_Consistency(t *testing.T) {
	svc := inspector.New()
	ctx := context.Background()

	docDirs := []string{
		"01_trudovoy_dogovor_it",
		"02_trudovoy_dogovor_rukovoditel",
		"03_soglasie_pdn",
		"04_zayavlenie_kompensaciya",
		"05_akt_priema_peredachi_tmc",
	}

	for _, d := range docDirs {
		docxPath := filepath.Join("../../../mock_documents", d, d+".docx")
		odtPath := filepath.Join("../../../mock_documents", d, d+".odt")
		txtPath := filepath.Join("../../../mock_documents", d, d+".txt")

		docxInfo, err := svc.Inspect(ctx, docxPath)
		if err != nil {
			t.Skipf("mock files not found: %v", err)
			return
		}
		odtInfo, err := svc.Inspect(ctx, odtPath)
		if err != nil {
			t.Fatalf("failed to inspect %s: %v", odtPath, err)
		}
		txtInfo, err := svc.Inspect(ctx, txtPath)
		if err != nil {
			t.Fatalf("failed to inspect %s: %v", txtPath, err)
		}

		// Check entity categories count matches between DOCX, ODT, and TXT
		if len(odtInfo.Entities) != len(docxInfo.Entities) {
			t.Errorf("[%s] entity categories mismatch: ODT=%d, DOCX=%d", d, len(odtInfo.Entities), len(docxInfo.Entities))
		}
		if len(txtInfo.Entities) != len(docxInfo.Entities) {
			t.Errorf("[%s] entity categories mismatch: TXT=%d, DOCX=%d", d, len(txtInfo.Entities), len(docxInfo.Entities))
		}

		// Check headings exist in both DOCX and ODT
		docxHeadings := 0
		for _, b := range docxInfo.DocumentBlocks {
			if b.Type == "heading" {
				docxHeadings++
			}
		}
		odtHeadings := 0
		for _, b := range odtInfo.DocumentBlocks {
			if b.Type == "heading" {
				odtHeadings++
			}
		}
		if docxHeadings == 0 {
			t.Errorf("[%s] expected headings in DOCX, got 0", d)
		}
		if odtHeadings == 0 {
			t.Errorf("[%s] expected headings in ODT, got 0", d)
		}
	}
}

func buildTestPDF(contentStream string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, 6)

	offsets[1] = b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	offsets[2] = b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	offsets[3] = b.Len()
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")

	offsets[4] = b.Len()
	streamBytes := []byte(contentStream)
	fmt.Fprintf(&b, "4 0 obj\n<< /Length %d >>\nstream\n", len(streamBytes))
	b.Write(streamBytes)
	b.WriteString("\nendstream\nendobj\n")

	offsets[5] = b.Len()
	b.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	startXref := b.Len()
	b.WriteString("xref\n0 6\n0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	b.WriteString("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n")
	fmt.Fprintf(&b, "%d\n%%%%EOF\n", startXref)
	return b.Bytes()
}

func TestInspectPDF(t *testing.T) {
	tmpDir := t.TempDir()
	pdfPath := filepath.Join(tmpDir, "test.pdf")

	content := "BT /F1 12 Tf 100 700 Td (сотрудник: Иванов Иван Иванович) Tj 100 680 Td (тел: +7 999 123-45-67) Tj ET"
	if err := os.WriteFile(pdfPath, buildTestPDF(content), 0644); err != nil {
		t.Fatalf("failed to write pdf: %v", err)
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("failed to inspect PDF: %v", err)
	}

	if info.Format != "pdf" || info.FileType != "pdf" {
		t.Errorf("expected format pdf and type pdf, got format=%s, type=%s", info.Format, info.FileType)
	}
	if info.PageCount < 1 {
		t.Errorf("expected pageCount >= 1, got %d", info.PageCount)
	}
	if len(info.Entities) == 0 {
		t.Errorf("expected detected entities in PDF, got 0")
	}
	if info.PDFTextSnippet == "" {
		t.Errorf("expected non-empty PDFTextSnippet")
	}
}

func TestInspect_UserMockXLS(t *testing.T) {
	mockPath := filepath.Join("..", "..", "..", "Шаблон импорта в ОУ. Передача данных 01.10.2026.xls")
	if _, err := os.Stat(mockPath); err != nil {
		t.Skip("mock xls file not found, skipping")
	}

	insp := inspector.New()
	info, err := insp.Inspect(context.Background(), mockPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if info.RowCount != 29 {
		t.Errorf("expected 29 rows, got %d", info.RowCount)
	}

	// Verify entities:
	// Col 1: Логин(вторичныйключ) -> EMAIL
	// Col 2: Фамилия -> LAST_NAME
	// Col 3: Имя -> FIRST_NAME
	// Col 4: Отчество -> PATRONYMIC
	// Col 9: Дата увольнения -> DATE
	// Col 10: Телефон -> PHONE
	entityMap := make(map[string]domain.EntityInfo)
	for _, e := range info.Entities {
		entityMap[e.ColumnName] = e
	}

	emailEnt, ok := entityMap["Логин(вторичныйключ)"]
	if !ok {
		t.Fatalf("expected entity for 'Логин(вторичныйключ)', got none")
	}
	if emailEnt.Type != "EMAIL" {
		t.Errorf("expected EMAIL for login column, got %s", emailEnt.Type)
	}

	surEnt, ok := entityMap["Фамилия"]
	if !ok || surEnt.Type != "LAST_NAME" {
		t.Errorf("expected LAST_NAME for 'Фамилия', got %+v", surEnt)
	}

	firstEnt, ok := entityMap["Имя"]
	if !ok || firstEnt.Type != "FIRST_NAME" {
		t.Errorf("expected FIRST_NAME for 'Имя', got %+v", firstEnt)
	}

	patrEnt, ok := entityMap["Отчество"]
	if !ok || patrEnt.Type != "PATRONYMIC" {
		t.Errorf("expected PATRONYMIC for 'Отчество', got %+v", patrEnt)
	}

	// Verify preview rows:
	if len(info.PreviewRows) == 0 {
		t.Fatalf("expected preview rows, got 0")
	}
	p0 := info.PreviewRows[0]
	// p0.Before[1] = "Самедов"
	if len(strings.Fields(p0.After[1])) != 1 {
		t.Errorf("expected single-word surname, got %q", p0.After[1])
	}
	// p0.Before[2] = "Самед"
	if len(strings.Fields(p0.After[2])) != 1 {
		t.Errorf("expected single-word first name, got %q", p0.After[2])
	}
	// p0.Before[3] = "Намиг Оглы"
	if len(strings.Fields(p0.After[3])) != 1 {
		t.Errorf("expected single-word patronymic, got %q", p0.After[3])
	}
}

func TestInspect_UserMockPDF(t *testing.T) {
	mockPath := filepath.Join("..", "..", "..", "mock_documents", "05_akt_priema_peredachi_tmc", "05_akt_priema_peredachi_tmc.pdf")
	if _, err := os.Stat(mockPath); err != nil {
		t.Skip("mock pdf file not found, skipping")
	}

	insp := inspector.New()
	info, err := insp.Inspect(context.Background(), mockPath)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if info.Format != "pdf" || info.FileType != "pdf" {
		t.Errorf("expected format and type pdf, got format=%s, type=%s", info.Format, info.FileType)
	}

	// Verify that false positives do NOT exist in TextSnippets or Entities
	for _, snippet := range info.TextSnippets {
		if strings.Contains(snippet.Original, "дает согласие") {
			t.Errorf("found false-positive FIO snippet: %q", snippet.Original)
		}
		if strings.Contains(snippet.Original, "обязуется не") {
			t.Errorf("found false-positive FIO snippet: %q", snippet.Original)
		}
		if strings.Contains(snippet.Original, "компании") {
			t.Errorf("found false-positive FIO snippet: %q", snippet.Original)
		}
	}

	// Verify genuine PII are found
	entityTypes := make(map[string]bool)
	for _, e := range info.Entities {
		entityTypes[e.Type] = true
	}
	for _, required := range []string{"FIO", "PASSPORT", "SNILS", "INN", "PHONE"} {
		if !entityTypes[required] {
			t.Errorf("expected entity %s to be detected in mock PDF, detected: %+v", required, info.Entities)
		}
	}
}

func TestInspect_UserDogovorExamplePDF(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "..", "Dogovor_example.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skip("skipping test: Dogovor_example.pdf not present in root")
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("unexpected error inspecting Dogovor_example.pdf: %v", err)
	}

	if info.PageCount != 13 {
		t.Errorf("expected 13 pages, got %d", info.PageCount)
	}

	if len(info.PDFTextSnippet) == 0 {
		t.Fatal("expected PDFTextSnippet, got empty")
	}

	if !strings.Contains(info.PDFTextSnippet, "ДОГОВОР") && !strings.Contains(info.PDFTextSnippet, "договор") {
		t.Errorf("expected 'ДОГОВОР' in text snippet for Dogovor_example.pdf")
	}
}

func TestInspect_UserFormaTipovogoDogovoraPDF(t *testing.T) {
	pdfPath := filepath.Join("..", "..", "..", "forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Skip("skipping test: forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf not present in root")
	}

	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("unexpected error inspecting forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf: %v", err)
	}

	if info.PageCount != 5 {
		t.Errorf("expected 5 pages, got %d", info.PageCount)
	}

	if len(info.TextSnippets) == 0 {
		t.Fatal("expected text snippets, got none")
	}

	foundFIO := false
	for _, e := range info.Entities {
		if e.Type == "FIO" {
			foundFIO = true
			break
		}
	}
	if !foundFIO {
		t.Errorf("expected FIO entity to be detected in forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf")
	}
}

func TestInspect_ThreeNewPDFs(t *testing.T) {
	files := []string{
		"../../../ETICKET_PLETNEV_I._31.05-04.06_GDZ-SVO-GDZ.PDF",
		"../../../Плетнёв_ЯА_Оригинал_заявления (переход на ЭТК).pdf",
		"../../../Положение о управлении.pdf",
	}

	svc := inspector.New()
	for _, f := range files {
		info, err := svc.Inspect(context.Background(), f)
		t.Logf("=== File: %s ===", filepath.Base(f))
		if err != nil {
			t.Logf("  ERROR: %v", err)
			continue
		}
		t.Logf("  Pages: %d, Snippets: %d, Entities: %d", info.PageCount, len(info.TextSnippets), len(info.Entities))
		if info.PageCount <= 0 {
			t.Errorf("expected PageCount > 0, got %d", info.PageCount)
		}
	}
}

func TestInspect_InspectSnippets_RealDogovor(t *testing.T) {
	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), "../../../forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf")
	if err != nil {
		t.Skipf("Inspect failed: %v", err)
	}

	t.Logf("Total snippets: %d", len(info.TextSnippets))
	for i, s := range info.TextSnippets {
		t.Logf("Snippet [%d] (%s): %q", i, s.Type, s.Original)
	}

	if info.PageCount != 5 {
		t.Errorf("expected 5 pages, got %d", info.PageCount)
	}
	if len(info.TextSnippets) < 4 {
		t.Errorf("expected at least 4 text snippets, got %d", len(info.TextSnippets))
	}
	if len(info.Entities) == 0 {
		t.Errorf("expected detected entities in real dogovor")
	}
}

func TestInspector_LargeCSV_Allowed_And_LargePDF_Rejected(t *testing.T) {
	tmpDir := t.TempDir()
	svc := inspector.New()

	// 1. Create a 120 MB CSV file (exceeds 100 MB limit)
	csvPath := filepath.Join(tmpDir, "large.csv")
	f, err := os.Create(csvPath)
	if err != nil {
		t.Fatalf("failed to create large.csv: %v", err)
	}
	_, _ = f.WriteString("ФИО,Телефон\n")
	chunk := bytes.Repeat([]byte("Иванов Иван,+79991234567\n"), 1024)
	written := 0
	target := 101 * 1024 * 1024
	for written < target {
		toWrite := chunk
		if written+len(toWrite) > target {
			toWrite = toWrite[:target-written]
		}
		n, err := f.Write(toWrite)
		if err != nil {
			t.Fatalf("failed to write to large.csv: %v", err)
		}
		written += n
	}
	_ = f.Close()

	// 2. Inspecting large CSV MUST succeed and not return ErrFileTooLarge
	info, err := svc.Inspect(context.Background(), csvPath)
	if err != nil {
		t.Fatalf("expected large CSV to be inspected without error, got: %v", err)
	}
	if info.Format != "csv" || info.Size < 100*1024*1024 {
		t.Errorf("expected CSV format and size >= 100MB, got format=%q, size=%d", info.Format, info.Size)
	}

	// 3. Create a 120 MB PDF file (must be rejected)
	pdfPath := filepath.Join(tmpDir, "large.pdf")
	pf, err := os.Create(pdfPath)
	if err != nil {
		t.Fatalf("failed to create large.pdf: %v", err)
	}
	_, _ = pf.WriteString("%PDF-1.4\n")
	if err := pf.Truncate(120 * 1024 * 1024); err != nil {
		t.Fatalf("failed to truncate large.pdf: %v", err)
	}
	_ = pf.Close()

	// 4. Inspecting large PDF MUST return ErrFileTooLarge
	_, err = svc.Inspect(context.Background(), pdfPath)
	if err == nil {
		t.Fatalf("expected error for 120MB PDF, got nil")
	}
	if !errors.Is(err, domain.ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge for 120MB PDF, got: %v", err)
	}
}

// testXLSBase64 is a base64-encoded valid XLS file fixture.
const testXLSBase64 = "0M8R4KGxGuEAAAAAAAAAAAAAAAAAAAAAPgADAP7/CQAGAAAAAAAAAAAAAAABAAAAAQAAAAAAAAAAEAAAIwAAAAEAAAD+////AAAAAAAAAAD////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////9////JQAAAAMAAAAEAAAABQAAAAYAAAAHAAAACAAAAAkAAAAKAAAACwAAAAwAAAANAAAADgAAAA8AAAAQAAAAEQAAABIAAAATAAAAFAAAABUAAAAWAAAAFwAAABgAAAAZAAAAGgAAABsAAAAcAAAAHQAAAB4AAAAfAAAAIAAAACEAAAAiAAAA/v////7///8mAAAA/v////7//////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////1IAbwBvAHQAIABFAG4AdAByAHkAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAWAAUA//////////8CAAAAIAgCAAAAAADAAAAAAAAARgAAAAAAAAAAAAAAACDqAgEu+9ABJAAAAIACAAAAAAAAVwBvAHIAawBiAG8AbwBrAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABIAAgEEAAAA//////////8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAAw0EAAAAAAAAFAFMAdQBtAG0AYQByAHkASQBuAGYAbwByAG0AYQB0AGkAbwBuAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAKAACAQEAAAADAAAA/////wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAADwAAAAAAAAAAUARABvAGMAdQBtAGUAbgB0AFMAdQBtAG0AYQByAHkASQBuAGYAbwByAG0AYQB0AGkAbwBuAAAAAAAAAAAAAAA4AAIB////////////////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAANwAAAAAAAAACQgQAAAGBQDHZswH2IADAAYHAADhAAIAsATBAAIAAADiAAAAXABwABMAAU0AaQBjAHIAbwBzAG8AZgB0ACAATwBmAGYAaQBjAGUAIAAodTdiICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgICBCAAIAsARhAQIAAAA9AQIAAQCcAAIAEQAZAAIAAAASAAIAAAATAAIAAACvAQIAAAC8AQIAAAA9ABIAAADMAWxwQB84AAAAAAABAFgCQAACAAAAjQACAAAAIgACAAAADgACAAEAtwECAAAA2gACAAAA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AA8AFQAAAP0ACgAHAAIADwAfAAAA/QAKAAgAAAAPAAwAAAD9AAoACAABAA8AFgAAAP0ACgAIAAIADwAgAAAA/QAKAAkAAAAPAA0AAAD9AAoACQABAA8AFwAAAP0ACgAJAAIADwAhAAAA/QAKAAoAAAAPAA4AAAD9AAoACgABAA8AGAAAAP0ACgAKAAIADwAiAAAA/QAKAAsAAAAPAA8AAAD9AAoACwABAA8AGQAAAP0ACgALAAIADwAjAAAA1wAcAOgCAADcACoAKgAqACoAKgAqACoAKgAqACoAKgA+AhIAtgYFAAAAQAAAAAAAAAAAAAAAiwgQAIsIAAAAAAAAAAAAAAAAAgAdAA8AAwwAAwAAAAEADAAMAAMDmQACANUI7wAGAAUANwAAAGcIFwBnCAAAAAAAAAAAAAACAAH/////A0QAAJwIJgCcCAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAPDMAAAAAAAAAAAoAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAIAAAADAAAA/v///wUAAAAGAAAABwAAAP7///8JAAAA/v/////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////+/wAAAwoBAAAAAAAAAAAAAAAAAAAAAAABAAAA4IWf8vlPaBCrkQgAKyez2TAAAADAAAAABwAAAAEAAABAAAAABAAAAEgAAAAIAAAAXAAAABIAAAB8AAAADAAAAKAAAAANAAAArAAAABMAAAC4AAAAAgAAABgnAAAeAAAADAAAAGh1YW5neGlvbmcAAB4AAAAYAAAATWljcm9zb2Z0IE9mZmljZSDTw7unAAAAHgAAABwAAABNaWNyb3NvZnQgTWFjaW50b3NoIEV4Y2VsAAAAQAAAAAAS0CB63dABQAAAAGAJLfot+9ABAwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA/v8AAAMKAQAAAAAAAAAAAAAAAAAAAAAAAQAAAALVzdWcLhsQk5cIACss+a4wAAAArAAAAAgAAAABAAAASAAAABcAAABQAAAACwAAAFgAAAAQAAAAYAAAABMAAABoAAAAFgAAAHAAAAANAAAAeAAAAAwAAACKAAAAAgAAABgnAAADAAAAAAAPAAsAAAAAAAAACwAAAAAAAAALAAAAAAAAAAsAAAAAAAAAHhAAAAEAAAAGAAAAVGFibGUADBAAAAIAAAAeAAAABwAAALmk1/ex7QADAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAEAQwBvAG0AcABPAGIAagAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAASAAIA////////////////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAAAG0AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAD///////////////8AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAP///////////////wAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA////////////////AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQD+/wMKAAD/////IAgCAAAAAADAAAAAAAAARiEAAABNaWNyb3NvZnQgRXhjZWwgOTcgLSAyMDA0ILmk1/ex7QAGAAAAQmlmZjgADgAAAEV4Y2VsLlNoZWV0LjgA9DmycQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
