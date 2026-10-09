package preview_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"dep_service/internal/domain"
	"dep_service/internal/service/preview"
)

// helper to create a sample CSV file
func createSampleCSV(t *testing.T, dir, filename, delimiter string, numRows int) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)

	var sb strings.Builder
	// Header: FIO, Phone, City (FIO and Phone trigger PII detector, City does not)
	sb.WriteString(fmt.Sprintf("ФИО%sТелефон%sГород\n", delimiter, delimiter))

	for i := 1; i <= numRows; i++ {
		sb.WriteString(fmt.Sprintf("Иванов Иван Иванович%s+7 999 123-45-%02d%sМосква\n", delimiter, i, delimiter))
	}

	if err := os.WriteFile(filePath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write sample csv: %v", err)
	}
	return filePath
}

// helper to create a sample XLSX file
func createSampleXLSX(t *testing.T, dir, filename string, sheets map[string]int) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)

	xl := excelize.NewFile()
	names := make([]string, 0, len(sheets))
	for name := range sheets {
		names = append(names, name)
	}
	sort.Strings(names)

	first := true
	for _, sheetName := range names {
		numRows := sheets[sheetName]
		if first {
			xl.SetSheetName("Sheet1", sheetName)
			first = false
		} else {
			xl.NewSheet(sheetName)
		}

		_ = xl.SetCellValue(sheetName, "A1", "ФИО")
		_ = xl.SetCellValue(sheetName, "B1", "СНИЛС")
		_ = xl.SetCellValue(sheetName, "C1", "Город")

		for i := 1; i <= numRows; i++ {
			rowNum := i + 1
			_ = xl.SetCellValue(sheetName, fmt.Sprintf("A%d", rowNum), "Петров Петр Петрович")
			_ = xl.SetCellValue(sheetName, fmt.Sprintf("B%d", rowNum), "112-233-445 95")
			_ = xl.SetCellValue(sheetName, fmt.Sprintf("C%d", rowNum), "Санкт-Петербург")
		}
	}

	if err := xl.SaveAs(filePath); err != nil {
		t.Fatalf("failed to save sample xlsx: %v", err)
	}
	return filePath
}

func TestGetSheetRows_Validation(t *testing.T) {
	svc := preview.New()
	ctx := context.Background()

	// Empty path
	_, err := svc.GetSheetRows(ctx, "", "", 0, 10, "test-salt", nil)
	if err == nil {
		t.Errorf("expected error on empty path, got nil")
	}

	// Non-existent file
	_, err = svc.GetSheetRows(ctx, "/path/does/not/exist.csv", "", 0, 10, "test-salt", nil)
	if err == nil {
		t.Errorf("expected error on non-existent file, got nil")
	}

	// Directory path
	tempDir := t.TempDir()
	_, err = svc.GetSheetRows(ctx, tempDir, "", 0, 10, "test-salt", nil)
	if err == nil {
		t.Errorf("expected error on directory path, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat for directory, got %v", err)
	}

	// Unsupported format
	unsupportedFile := filepath.Join(tempDir, "sample.txt")
	_ = os.WriteFile(unsupportedFile, []byte("some text"), 0644)
	_, err = svc.GetSheetRows(ctx, unsupportedFile, "", 0, 10, "test-salt", nil)
	if err == nil {
		t.Errorf("expected error on unsupported format, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected ErrUnsupportedFormat for txt, got %v", err)
	}
}

func TestGetCSVPreviewRows(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "sample.csv", ",", 5)

	svc := preview.New()
	rows, err := svc.GetSheetRows(context.Background(), csvPath, "", 0, 10, "test-salt", nil)
	if err != nil {
		t.Fatalf("failed to get preview rows: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 preview rows, got %d", len(rows))
	}
	if rows[0].Index != 1 {
		t.Errorf("expected first row index 1, got %d", rows[0].Index)
	}
	if len(rows[0].Before) != 3 || len(rows[0].After) != 3 {
		t.Fatalf("expected 3 before/after columns, got %d / %d", len(rows[0].Before), len(rows[0].After))
	}
	if rows[0].Before[0] != "Иванов Иван Иванович" {
		t.Errorf("unexpected Before[0]: %s", rows[0].Before[0])
	}
	if rows[0].After[0] == rows[0].Before[0] {
		t.Errorf("expected FIO to be anonymized in After[0], got %s", rows[0].After[0])
	}
}

func TestGetCSVRows_Pagination(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "data.csv", ",", 25)

	svc := preview.New()
	ctx := context.Background()

	// Page 1: offset 0, limit 10
	p1, err := svc.GetSheetRows(ctx, csvPath, "", 0, 10, "salt-1", nil)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(p1) != 10 {
		t.Fatalf("expected 10 rows on page 1, got %d", len(p1))
	}
	if p1[0].Index != 1 || p1[9].Index != 10 {
		t.Errorf("expected indices 1..10, got %d..%d", p1[0].Index, p1[9].Index)
	}

	// Page 2: offset 10, limit 10
	p2, err := svc.GetSheetRows(ctx, csvPath, "", 10, 10, "salt-1", nil)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(p2) != 10 {
		t.Fatalf("expected 10 rows on page 2, got %d", len(p2))
	}
	if p2[0].Index != 11 || p2[9].Index != 20 {
		t.Errorf("expected indices 11..20, got %d..%d", p2[0].Index, p2[9].Index)
	}

	// Page 3: offset 20, limit 10 (remaining 5 rows)
	p3, err := svc.GetSheetRows(ctx, csvPath, "", 20, 10, "salt-1", nil)
	if err != nil {
		t.Fatalf("page 3 failed: %v", err)
	}
	if len(p3) != 5 {
		t.Fatalf("expected 5 rows on page 3, got %d", len(p3))
	}
	if p3[0].Index != 21 || p3[4].Index != 25 {
		t.Errorf("expected indices 21..25, got %d..%d", p3[0].Index, p3[4].Index)
	}

	// Page 4: offset 30, limit 10 (past end)
	p4, err := svc.GetSheetRows(ctx, csvPath, "", 30, 10, "salt-1", nil)
	if err != nil {
		t.Fatalf("page 4 failed: %v", err)
	}
	if len(p4) != 0 {
		t.Fatalf("expected 0 rows on page 4, got %d", len(p4))
	}

	// Default limit (limit <= 0 defaults to 20)
	pDef, err := svc.GetSheetRows(ctx, csvPath, "", 0, 0, "salt-1", nil)
	if err != nil {
		t.Fatalf("default limit query failed: %v", err)
	}
	if len(pDef) != 20 {
		t.Fatalf("expected 20 rows with default limit, got %d", len(pDef))
	}

	// Negative offset (offset < 0 defaults to 0)
	pNeg, err := svc.GetSheetRows(ctx, csvPath, "", -5, 5, "salt-1", nil)
	if err != nil {
		t.Fatalf("negative offset query failed: %v", err)
	}
	if len(pNeg) != 5 || pNeg[0].Index != 1 {
		t.Errorf("expected 5 rows starting at index 1 for negative offset, got %d rows, start %d", len(pNeg), pNeg[0].Index)
	}
}

func TestGetCSVRows_DelimiterDetection(t *testing.T) {
	tempDir := t.TempDir()
	svc := preview.New()
	ctx := context.Background()

	// Semicolon
	semiPath := createSampleCSV(t, tempDir, "data_semi.csv", ";", 5)
	rowsSemi, err := svc.GetSheetRows(ctx, semiPath, "", 0, 10, "salt", nil)
	if err != nil {
		t.Fatalf("semicolon csv failed: %v", err)
	}
	if len(rowsSemi) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rowsSemi))
	}
	if len(rowsSemi[0].Before) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(rowsSemi[0].Before))
	}

	// Tab-separated (.tsv)
	tabPath := createSampleCSV(t, tempDir, "data_tab.tsv", "\t", 5)
	rowsTab, err := svc.GetSheetRows(ctx, tabPath, "", 0, 10, "salt", nil)
	if err != nil {
		t.Fatalf("tab tsv failed: %v", err)
	}
	if len(rowsTab) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rowsTab))
	}
	if len(rowsTab[0].Before) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(rowsTab[0].Before))
	}
}

func TestGetSheetRows_Windows1251(t *testing.T) {
	tempDir := t.TempDir()
	csvContent := "ФИО,Телефон,Город\nИванов Иван Иванович,+79991234567,Москва\n"
	win1251Bytes, err := charmap.Windows1251.NewEncoder().Bytes([]byte(csvContent))
	if err != nil {
		t.Fatalf("failed to encode windows-1251: %v", err)
	}
	csvPath := filepath.Join(tempDir, "win1251_preview.csv")
	if err := os.WriteFile(csvPath, win1251Bytes, 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	svc := preview.New()
	rows, err := svc.GetSheetRows(context.Background(), csvPath, "", 0, 10, "salt-test", nil)
	if err != nil {
		t.Fatalf("failed to get sheet rows for Windows-1251: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Verify Before row has Russian Cyrillic decoded from Windows-1251 without question marks
	if rows[0].Before[0] != "Иванов Иван Иванович" {
		t.Errorf("expected original 'Иванов Иван Иванович', got %q", rows[0].Before[0])
	}
	if rows[0].Before[2] != "Москва" {
		t.Errorf("expected original 'Москва', got %q", rows[0].Before[2])
	}
	// Verify After row has anonymized Russian FIO without question marks
	if strings.Contains(rows[0].After[0], "?") || strings.Contains(rows[0].After[0], "\ufffd") {
		t.Errorf("anonymized FIO contains question marks: %q", rows[0].After[0])
	}
}

func TestGetCSVRows_SaltDeterminism(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "salt_test.csv", ",", 5)
	svc := preview.New()
	ctx := context.Background()

	// Same salt produces identical results
	res1, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "salt-alpha", nil)
	if err != nil {
		t.Fatalf("res1 failed: %v", err)
	}
	res2, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "salt-alpha", nil)
	if err != nil {
		t.Fatalf("res2 failed: %v", err)
	}
	for i := range res1 {
		for j := range res1[i].After {
			if res1[i].After[j] != res2[i].After[j] {
				t.Fatalf("salt determinism failed at row %d col %d: %q != %q", i, j, res1[i].After[j], res2[i].After[j])
			}
		}
	}

	// Different salt produces different results for sensitive columns
	res3, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "salt-beta", nil)
	if err != nil {
		t.Fatalf("res3 failed: %v", err)
	}
	// FIO column (col 0) should differ between salt-alpha and salt-beta
	if res1[0].After[0] == res3[0].After[0] {
		t.Errorf("expected different anonymized values for different salts, got identical %s", res1[0].After[0])
	}

	// Empty salt defaults consistently
	resDef1, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "", nil)
	if err != nil {
		t.Fatalf("resDef1 failed: %v", err)
	}
	resDef2, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "", nil)
	if err != nil {
		t.Fatalf("resDef2 failed: %v", err)
	}
	if resDef1[0].After[0] != resDef2[0].After[0] {
		t.Errorf("expected identical default salt results, got %s != %s", resDef1[0].After[0], resDef2[0].After[0])
	}
}

func TestGetCSVRows_DisabledCols(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "mask_test.csv", ",", 5)
	svc := preview.New()
	ctx := context.Background()

	// Without disabledCols: col 1 (FIO) and col 2 (Phone) should be anonymized
	unmasked, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "salt-test", nil)
	if err != nil {
		t.Fatalf("unmasked query failed: %v", err)
	}
	if unmasked[0].After[0] == unmasked[0].Before[0] {
		t.Errorf("expected col 1 to be anonymized when not disabled")
	}
	if unmasked[0].After[1] == unmasked[0].Before[1] {
		t.Errorf("expected col 2 to be anonymized when not disabled")
	}

	// With disabledCols=[1]: col 1 (FIO, 1-based) is excluded from anonymization, col 2 is still anonymized
	maskedCol1, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "salt-test", []int{1})
	if err != nil {
		t.Fatalf("masked col 1 query failed: %v", err)
	}
	if maskedCol1[0].After[0] != maskedCol1[0].Before[0] {
		t.Errorf("expected col 1 to remain unchanged when disabled, got After: %s, Before: %s", maskedCol1[0].After[0], maskedCol1[0].Before[0])
	}
	if maskedCol1[0].After[1] == maskedCol1[0].Before[1] {
		t.Errorf("expected col 2 to still be anonymized")
	}
}

func TestGetCSVRows_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "ctx_test.csv", ",", 20)
	svc := preview.New()

	// Pre-cancelled context
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.GetSheetRows(ctxCancel, csvPath, "", 0, 10, "salt", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestGetXLSXRows_Pagination(t *testing.T) {
	tempDir := t.TempDir()
	xlsxPath := createSampleXLSX(t, tempDir, "sample.xlsx", map[string]int{"Sheet1": 25})

	svc := preview.New()
	ctx := context.Background()

	// Page 1: offset 0, limit 10
	p1, err := svc.GetSheetRows(ctx, xlsxPath, "Sheet1", 0, 10, "salt", nil)
	if err != nil {
		t.Fatalf("page 1 failed: %v", err)
	}
	if len(p1) != 10 {
		t.Fatalf("expected 10 rows on page 1, got %d", len(p1))
	}
	if p1[0].Index != 1 || p1[9].Index != 10 {
		t.Errorf("expected indices 1..10, got %d..%d", p1[0].Index, p1[9].Index)
	}
	if p1[0].Before[0] != "Петров Петр Петрович" {
		t.Errorf("unexpected Before[0]: %s", p1[0].Before[0])
	}
	if p1[0].After[0] == p1[0].Before[0] {
		t.Errorf("expected FIO to be anonymized in xlsx preview row")
	}

	// Page 2: offset 10, limit 10
	p2, err := svc.GetSheetRows(ctx, xlsxPath, "Sheet1", 10, 10, "salt", nil)
	if err != nil {
		t.Fatalf("page 2 failed: %v", err)
	}
	if len(p2) != 10 {
		t.Fatalf("expected 10 rows on page 2, got %d", len(p2))
	}
	if p2[0].Index != 11 || p2[9].Index != 20 {
		t.Errorf("expected indices 11..20, got %d..%d", p2[0].Index, p2[9].Index)
	}

	// Page 3: offset 20, limit 10 (remaining 5 rows)
	p3, err := svc.GetSheetRows(ctx, xlsxPath, "Sheet1", 20, 10, "salt", nil)
	if err != nil {
		t.Fatalf("page 3 failed: %v", err)
	}
	if len(p3) != 5 {
		t.Fatalf("expected 5 rows on page 3, got %d", len(p3))
	}
	if p3[0].Index != 21 || p3[4].Index != 25 {
		t.Errorf("expected indices 21..25, got %d..%d", p3[0].Index, p3[4].Index)
	}
}

func TestGetXLSXRows_MultiSheet(t *testing.T) {
	tempDir := t.TempDir()
	xlsxPath := createSampleXLSX(t, tempDir, "multi.xlsx", map[string]int{
		"Primary":   5,
		"Secondary": 8,
	})

	svc := preview.New()
	ctx := context.Background()

	// Default sheet (empty sheetName selects first sheet "Primary")
	pDefault, err := svc.GetSheetRows(ctx, xlsxPath, "", 0, 10, "salt", nil)
	if err != nil {
		t.Fatalf("default sheet failed: %v", err)
	}
	if len(pDefault) != 5 {
		t.Fatalf("expected 5 rows from Primary sheet, got %d", len(pDefault))
	}

	// Named sheet "Secondary"
	pSec, err := svc.GetSheetRows(ctx, xlsxPath, "Secondary", 0, 10, "salt", nil)
	if err != nil {
		t.Fatalf("secondary sheet failed: %v", err)
	}
	if len(pSec) != 8 {
		t.Fatalf("expected 8 rows from Secondary sheet, got %d", len(pSec))
	}
}

func TestGetXLSXRows_SaltDeterminism(t *testing.T) {
	tempDir := t.TempDir()
	xlsxPath := createSampleXLSX(t, tempDir, "salt_xlsx.xlsx", map[string]int{"Sheet1": 5})

	svc := preview.New()
	ctx := context.Background()

	res1, err := svc.GetSheetRows(ctx, xlsxPath, "", 0, 5, "salt-x", nil)
	if err != nil {
		t.Fatalf("res1 failed: %v", err)
	}
	res2, err := svc.GetSheetRows(ctx, xlsxPath, "", 0, 5, "salt-x", nil)
	if err != nil {
		t.Fatalf("res2 failed: %v", err)
	}
	for i := range res1 {
		for j := range res1[i].After {
			if res1[i].After[j] != res2[i].After[j] {
				t.Fatalf("salt determinism failed on xlsx: %s != %s", res1[i].After[j], res2[i].After[j])
			}
		}
	}

	res3, err := svc.GetSheetRows(ctx, xlsxPath, "", 0, 5, "salt-y", nil)
	if err != nil {
		t.Fatalf("res3 failed: %v", err)
	}
	if res1[0].After[0] == res3[0].After[0] {
		t.Errorf("expected different anonymized values for different salts on xlsx")
	}
}

func TestGetXLSXRows_DisabledCols(t *testing.T) {
	tempDir := t.TempDir()
	xlsxPath := createSampleXLSX(t, tempDir, "mask_xlsx.xlsx", map[string]int{"Sheet1": 5})

	svc := preview.New()
	ctx := context.Background()

	// Column 1 is FIO, column 2 is SNILS
	// Disable column 2 (SNILS)
	res, err := svc.GetSheetRows(ctx, xlsxPath, "", 0, 5, "salt", []int{2})
	if err != nil {
		t.Fatalf("disabled cols xlsx failed: %v", err)
	}
	if res[0].After[0] == res[0].Before[0] {
		t.Errorf("expected col 1 (FIO) to be anonymized")
	}
	if res[0].After[1] != res[0].Before[1] {
		t.Errorf("expected col 2 (SNILS) to remain unmasked when disabled, got After: %s, Before: %s", res[0].After[1], res[0].Before[1])
	}
}

func TestGetXLSXRows_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	xlsxPath := createSampleXLSX(t, tempDir, "ctx_xlsx.xlsx", map[string]int{"Sheet1": 10})

	svc := preview.New()
	ctxCancel, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.GetSheetRows(ctxCancel, xlsxPath, "", 0, 5, "salt", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestGetTableRows(t *testing.T) {
	tempDir := t.TempDir()
	csvPath := createSampleCSV(t, tempDir, "table.csv", ",", 5)

	svc := preview.New()
	rows, err := svc.GetTableRows(context.Background(), csvPath, 0, 5, "salt", nil)
	if err != nil {
		t.Fatalf("GetTableRows failed: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(rows))
	}
}

func TestGetXLSRows_Pagination(t *testing.T) {
	mockPath := filepath.Join("..", "..", "..", "Шаблон импорта в ОУ. Передача данных 01.10.2026.xls")
	if _, err := os.Stat(mockPath); err != nil {
		t.Skip("mock xls file not found, skipping")
	}

	svc := preview.New()
	ctx := context.Background()

	// 1. Fetch batch past row 10: offset 10, limit 10
	rows, err := svc.GetSheetRows(ctx, mockPath, "Лист1", 10, 10, "test-salt", nil)
	if err != nil {
		t.Fatalf("GetSheetRows failed on .xls: %v", err)
	}

	if len(rows) == 0 {
		t.Fatalf("expected rows after offset 10, got 0")
	}

	// Verify row indexing starts at offset + 1 = 11
	if rows[0].Index != 11 {
		t.Errorf("expected first row index 11, got %d", rows[0].Index)
	}

	// Verify column 0 (Логин) is anonymized as email
	origEmail := rows[0].Before[0]
	anonEmail := rows[0].After[0]
	if !strings.Contains(origEmail, "@") {
		t.Errorf("expected original cell 0 to be email, got %q", origEmail)
	}
	if !strings.Contains(anonEmail, "@") || anonEmail == origEmail {
		t.Errorf("expected anonymized email in cell 0, got %q", anonEmail)
	}

	// Verify column 1 (Фамилия) is single-word surname
	origSurname := rows[0].Before[1]
	anonSurname := rows[0].After[1]
	if len(strings.Fields(anonSurname)) != 1 {
		t.Errorf("expected single-word surname for %q, got %q", origSurname, anonSurname)
	}

	// Verify column 2 (Имя) is single-word first name
	origName := rows[0].Before[2]
	anonName := rows[0].After[2]
	if len(strings.Fields(anonName)) != 1 {
		t.Errorf("expected single-word first name for %q, got %q", origName, anonName)
	}

	// Verify column 3 (Отчество) is single-word patronymic
	origPatr := rows[0].Before[3]
	anonPatr := rows[0].After[3]
	if len(strings.Fields(anonPatr)) != 1 {
		t.Errorf("expected single-word patronymic for %q, got %q", origPatr, anonPatr)
	}
}

func TestGetSheetRows_ModeDelete(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "delete_test.csv")
	content := "ID,ФИО,Количество\n1,Иванов Иван Иванович,42\n2,Петров Петр Петрович,100\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	svc := preview.New()
	ctx := context.Background()

	// 1. Request rows with mode = "delete"
	rows, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "test-salt", nil, "delete")
	if err != nil {
		t.Fatalf("GetSheetRows with mode=delete failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	for i, r := range rows {
		// Column 0 is ID -> non-sensitive, preserved
		if r.After[0] != r.Before[0] {
			t.Errorf("row %d: expected After[0] to be preserved %q, got %q", i, r.Before[0], r.After[0])
		}
		// Column 1 is ФИО -> sensitive, deleted!
		if r.After[1] != "[УДАЛЕНО]" {
			t.Errorf("row %d: expected After[1] to be [УДАЛЕНО], got %q", i, r.After[1])
		}
		// Column 2 is Количество -> non-sensitive, preserved
		if r.After[2] != r.Before[2] {
			t.Errorf("row %d: expected After[2] to be preserved %q, got %q", i, r.Before[2], r.After[2])
		}
	}

	// 2. Disabled columns in delete mode: disabled col 2 (ФИО) should NOT be deleted
	disabledRows, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "test-salt", []int{2}, "delete")
	if err != nil {
		t.Fatalf("GetSheetRows with disabled col failed: %v", err)
	}
	if disabledRows[0].After[1] != "Иванов Иван Иванович" {
		t.Errorf("expected disabled column to remain untouched, got %q", disabledRows[0].After[1])
	}

	// 3. Default / replace mode: sensitive col is anonymized, NOT replaced with [УДАЛЕНО]
	replaceRows, err := svc.GetSheetRows(ctx, csvPath, "", 0, 5, "test-salt", nil, "replace")
	if err != nil {
		t.Fatalf("GetSheetRows with mode=replace failed: %v", err)
	}
	if replaceRows[0].After[1] == "[УДАЛЕНО]" || replaceRows[0].After[1] == replaceRows[0].Before[1] {
		t.Errorf("expected anonymized replacement, got %q", replaceRows[0].After[1])
	}
}
