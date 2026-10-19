package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

func biff12WriteID(buf *bytes.Buffer, id int) {
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
}

func biff12WriteLen(buf *bytes.Buffer, n int) {
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
}

func biff12WriteRec(buf *bytes.Buffer, id int, payload []byte) {
	biff12WriteID(buf, id)
	biff12WriteLen(buf, len(payload))
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

func biff12Le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func createTestXLSB(t *testing.T, sheetName string, headers []string, dataRows [][]string) []byte {
	t.Helper()

	// 1. Collect all unique strings for SST
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

	// 2. Build xl/sharedStrings.bin
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

	// 3. Build xl/workbook.bin
	var wb bytes.Buffer
	biff12WriteRec(&wb, 0x0183, nil) // WORKBOOK start
	biff12WriteRec(&wb, 0x018F, nil) // SHEETS start

	var sheetRec bytes.Buffer
	sheetRec.Write(biff12Le32(0))              // state flags
	sheetRec.Write(biff12Le32(1))              // sheetId = 1
	sheetRec.Write(biff12EncStr("rId1"))       // relId
	sheetRec.Write(biff12EncStr(sheetName))    // name
	biff12WriteRec(&wb, 0x019C, sheetRec.Bytes())

	biff12WriteRec(&wb, 0x0190, nil) // SHEETS end
	biff12WriteRec(&wb, 0x0184, nil) // WORKBOOK end

	// 4. Build xl/worksheets/sheet1.bin
	var ws bytes.Buffer
	biff12WriteRec(&ws, 0x0181, nil) // WORKSHEET start

	totalRows := uint32(1 + len(dataRows))
	totalCols := uint32(len(headers))

	var dim bytes.Buffer
	dim.Write(biff12Le32(0))
	dim.Write(biff12Le32(totalRows - 1))
	dim.Write(biff12Le32(0))
	dim.Write(biff12Le32(totalCols - 1))
	biff12WriteRec(&ws, 0x0194, dim.Bytes()) // DIMENSION

	biff12WriteRec(&ws, 0x0191, nil) // SHEETDATA start

	// Row 0: Headers
	biff12WriteRec(&ws, 0x0000, biff12Le32(0)) // ROW 0
	for cIdx, h := range headers {
		var strCell bytes.Buffer
		strCell.Write(biff12Le32(uint32(cIdx)))
		strCell.Write(biff12Le32(0)) // style
		strCell.Write(biff12Le32(strMap[h]))
		biff12WriteRec(&ws, 0x0007, strCell.Bytes())
	}

	// Data rows
	for rIdx, row := range dataRows {
		rowNum := uint32(rIdx + 1)
		biff12WriteRec(&ws, 0x0000, biff12Le32(rowNum))
		for cIdx, val := range row {
			var strCell bytes.Buffer
			strCell.Write(biff12Le32(uint32(cIdx)))
			strCell.Write(biff12Le32(0))
			strCell.Write(biff12Le32(strMap[val]))
			biff12WriteRec(&ws, 0x0007, strCell.Bytes())
		}
	}

	biff12WriteRec(&ws, 0x0192, nil) // SHEETDATA end
	biff12WriteRec(&ws, 0x0182, nil) // WORKSHEET end

	// 5. Assemble ZIP archive
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	relsXML := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.bin"/>` +
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.bin"/>` +
		`</Relationships>`

	addZipFile := func(name string, content []byte) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip file %s: %v", name, err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("failed to write zip file %s: %v", name, err)
		}
	}

	addZipFile("xl/_rels/workbook.bin.rels", []byte(relsXML))
	addZipFile("xl/workbook.bin", wb.Bytes())
	addZipFile("xl/sharedStrings.bin", sst.Bytes())
	addZipFile("xl/worksheets/sheet1.bin", ws.Bytes())

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}

	return zipBuf.Bytes()
}

func writeTempXLSB(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.xlsb")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write temp xlsb: %v", err)
	}
	return path
}

func TestXLSBHandler_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)
	exts := h.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".xlsb" {
		t.Fatalf("unexpected supported extensions: %v", exts)
	}
}

func TestXLSBHandler_Inspect(t *testing.T) {
	headers := []string{"ФИО", "СНИЛС", "Телефон"}
	rows := [][]string{
		{"Иванов Иван Иванович", "112-233-445 95", "+7 916 123-45-67"},
		{"Петров Петр Петрович", "223-344-556 06", "+7 916 234-56-78"},
	}
	raw := createTestXLSB(t, "Сотрудники", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	sheets, err := h.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if len(sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(sheets))
	}
	s := sheets[0]
	if s.Name != "Сотрудники" {
		t.Errorf("expected sheet name 'Сотрудники', got %q", s.Name)
	}
	if s.RowCount != 3 {
		t.Errorf("expected 3 rows, got %d", s.RowCount)
	}
	if len(s.Columns) != 3 || s.Columns[0] != "ФИО" || s.Columns[1] != "СНИЛС" || s.Columns[2] != "Телефон" {
		t.Errorf("unexpected columns: %v", s.Columns)
	}
}

func TestXLSBHandler_Lifecycle(t *testing.T) {
	headers := []string{"ФИО", "СНИЛС", "Телефон"}
	rows := [][]string{
		{"Иванов Иван Иванович", "112-233-445 95", "+7 916 123-45-67"},
		{"Петров Петр Петрович", "223-344-556 06", "+7 916 234-56-78"},
	}
	raw := createTestXLSB(t, "Сотрудники", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{}
	dm, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	if dm == nil || len(dm.Entities) == 0 {
		t.Fatalf("expected non-empty depmap, got %+v", dm)
	}

	foundOriginal := false
	for anonVal, origVal := range dm.Mapping {
		if origVal == "Иванов Иван Иванович" {
			foundOriginal = true
			if anonVal == origVal {
				t.Errorf("anonymized FIO is identical to original")
			}
		}
	}
	if !foundOriginal {
		t.Errorf("original FIO 'Иванов Иван Иванович' not found in depmap")
	}

	// Verify output XLSX opens with excelize
	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX with excelize: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Сотрудники")
	if err != nil {
		t.Fatalf("failed to get rows from sheet 'Сотрудники': %v", err)
	}

	if len(outRows) != 3 {
		t.Fatalf("expected 3 rows in output XLSX, got %d", len(outRows))
	}

	// Header row intact
	if outRows[0][0] != "ФИО" || outRows[0][1] != "СНИЛС" || outRows[0][2] != "Телефон" {
		t.Errorf("unexpected headers: %v", outRows[0])
	}

	// Data rows anonymized
	if outRows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("FIO was not anonymized: %s", outRows[1][0])
	}
	if outRows[1][1] == "112-233-445 95" {
		t.Errorf("SNILS was not anonymized: %s", outRows[1][1])
	}
	if outRows[1][2] == "+7 916 123-45-67" {
		t.Errorf("Phone was not anonymized: %s", outRows[1][2])
	}

	if outRows[2][0] == "Петров Петр Петрович" {
		t.Errorf("FIO 2 was not anonymized: %s", outRows[2][0])
	}
}

func TestXLSBHandler_DisabledColumns(t *testing.T) {
	headers := []string{"ФИО", "СНИЛС", "Телефон"}
	rows := [][]string{
		{"Иванов Иван Иванович", "112-233-445 95", "+7 916 123-45-67"},
	}
	raw := createTestXLSB(t, "Сотрудники", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"disabled_columns": []int{2}, // disable SNILS
	}
	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Сотрудники")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	// FIO is anonymized
	if outRows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("FIO should be anonymized")
	}
	// SNILS is NOT anonymized because column 2 is disabled
	if outRows[1][1] != "112-233-445 95" {
		t.Errorf("SNILS should NOT be anonymized when disabled, got %q", outRows[1][1])
	}
	// Phone is anonymized
	if outRows[1][2] == "+7 916 123-45-67" {
		t.Errorf("Phone should be anonymized")
	}
}

func TestXLSBHandler_DisabledColumnsBySheet(t *testing.T) {
	headers := []string{"ФИО", "СНИЛС", "Телефон"}
	rows := [][]string{
		{"Иванов Иван Иванович", "112-233-445 95", "+7 916 123-45-67"},
	}
	raw := createTestXLSB(t, "Лист1", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"disabled_columns_by_sheet": map[string][]int{
			"Лист1": {3}, // disable Phone
		},
	}
	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Лист1")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	if outRows[1][2] != "+7 916 123-45-67" {
		t.Errorf("Phone should NOT be anonymized when disabled by sheet, got %q", outRows[1][2])
	}
	if outRows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("FIO should be anonymized")
	}
}

func TestXLSBHandler_ColumnOverridesAndProgress(t *testing.T) {
	headers := []string{"Код", "Имя", "Доп"}
	rows := [][]string{
		{"C001", "Сергей", "Примечание"},
	}
	raw := createTestXLSB(t, "Данные", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	progressCalled := false
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"columns": map[string]string{
			"Код": "SNILS",
		},
		"progress_callback": func(n int) {
			progressCalled = true
		},
	}
	dm, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	// Verify SNILS override was applied to "Код"
	foundSNILS := false
	for _, origVal := range dm.Mapping {
		if origVal == "C001" {
			foundSNILS = true
			break
		}
	}
	if !foundSNILS {
		t.Errorf("column override for 'Код' as SNILS was not recorded in depmap")
	}
	if !progressCalled {
		t.Errorf("expected progress_callback to be invoked")
	}
}

func TestXLSBHandler_ContextCancellation(t *testing.T) {
	headers := []string{"ФИО"}
	rows := [][]string{{"Иванов Иван Иванович"}}
	raw := createTestXLSB(t, "Sheet1", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	outBuf := new(bytes.Buffer)
	_, err := h.ProcessToXLSX(ctx, path, outBuf, nil)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}

func TestXLSBHandler_InvalidFile(t *testing.T) {
	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	_, err := h.Inspect("non_existent_file.xlsb")
	if err == nil {
		t.Fatalf("expected error inspecting nonexistent file")
	}

	outBuf := new(bytes.Buffer)
	_, err = h.ProcessToXLSX(context.Background(), "non_existent_file.xlsb", outBuf, nil)
	if err == nil {
		t.Fatalf("expected error processing nonexistent file")
	}
}

func TestXLSBHandler_MissingSheetError(t *testing.T) {
	headers := []string{"ФИО"}
	rows := [][]string{{"Иванов Иван Иванович"}}
	raw := createTestXLSB(t, "Лист1", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"sheet": "NonExistentSheet",
	}
	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected sheet not found error, got: %v", err)
	}
}

func TestXLSBHandler_StrategySemanticTokens(t *testing.T) {
	headers := []string{"ФИО", "Паспорт"}
	rows := [][]string{{"Иванов Иван Иванович", "4508 123456"}}
	raw := createTestXLSB(t, "Лист1", headers, rows)
	path := writeTempXLSB(t, raw)

	anon := anonymizer.New("test-salt")
	h := NewXLSBHandler(anon)

	outBuf := new(bytes.Buffer)
	dm := depmap.New("test.xlsb")
	opts := map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"salt":     "test-salt",
		"depmap":   dm,
	}

	resDm, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	if resDm.Mode != string(domain.StrategySemanticTokens) {
		t.Errorf("expected depmap mode to be %q, got %q", domain.StrategySemanticTokens, resDm.Mode)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Лист1")
	if err != nil {
		t.Fatalf("failed to get rows from sheet 'Лист1': %v", err)
	}

	if len(outRows) < 2 {
		t.Fatalf("expected at least 2 rows in output XLSX, got %d", len(outRows))
	}

	// Headers intact
	if outRows[0][0] != "ФИО" || outRows[0][1] != "Паспорт" {
		t.Errorf("unexpected headers: %v", outRows[0])
	}

	// Row 1 tokens
	expectedFIO := "[ФИО_0001]"
	if outRows[1][0] != expectedFIO {
		t.Errorf("expected semantic token %q in FIO column, got %q", expectedFIO, outRows[1][0])
	}
	expectedPass := "[Паспорт_0001]"
	if outRows[1][1] != expectedPass {
		t.Errorf("expected semantic token %q in Passport column, got %q", expectedPass, outRows[1][1])
	}
}
