package formats_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

func TestExcelHandler_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewExcelHandler(anon)

	exts := handler.SupportedExtensions()
	foundXLSX := false
	for _, ext := range exts {
		if ext == ".xlsx" {
			foundXLSX = true
		}
	}
	if !foundXLSX {
		t.Fatalf("expected .xlsx in supported extensions, got %v", exts)
	}
}

func TestExcelHandler_Process(t *testing.T) {
	anon := anonymizer.New("test-salt-excel-152fz")
	handler := formats.NewExcelHandler(anon)

	// Create test workbook with 2 sheets
	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Сотрудники"
	f.SetSheetName("Sheet1", sheet1)

	// Headers for Sheet 1
	headers1 := []string{"ФИО", "СНИЛС", "ИНН", "Паспорт", "Телефон", "Номер карты", "Дата рождения", "Должность"}
	for i, h := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet1, cell, h)
	}

	// Row 1
	row1Data := []interface{}{"Сидоров Петр Алексеевич", "112-233-445 95", "500100732259", "4512 654321", "+7 916 555-44-33", "2200 9876 5432 1098", "1992-11-20", "Инженер"}
	for i, v := range row1Data {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	// Row 2 (identical SNILS and INN to test referential integrity)
	row2Data := []interface{}{"Кузнецова Елена Павловна", "112-233-445 95", "500100732259", "4508 112233", "+7 925 111-22-33", "2200 1111 2222 3333", "01.03.1988", "Менеджер"}
	for i, v := range row2Data {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	// Row 3 (empty PII values)
	row3Data := []interface{}{"", "", "", "", "", "", "", "Стажер"}
	for i, v := range row3Data {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	// Sheet 2: "Клиенты"
	sheet2 := "Клиенты"
	_, _ = f.NewSheet(sheet2)
	headers2 := []string{"Клиент", "Контактный телефон"}
	for i, h := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet2, cell, h)
	}
	clientRow := []interface{}{"Иванов Иван Иванович", "+7 900 123-45-67"}
	for i, v := range clientRow {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet2, cell, v)
	}

	var inBuf bytes.Buffer
	if err := f.Write(&inBuf); err != nil {
		t.Fatalf("failed to write source excel file: %v", err)
	}

	var outBuf bytes.Buffer
	ctx := context.Background()
	if err := handler.Process(ctx, &inBuf, &outBuf, nil); err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open processed excel file: %v", err)
	}
	defer outExcel.Close()

	// Verify sheets exist
	sheets := outExcel.GetSheetList()
	hasSheet1 := false
	hasSheet2 := false
	for _, s := range sheets {
		if s == sheet1 {
			hasSheet1 = true
		}
		if s == sheet2 {
			hasSheet2 = true
		}
	}
	if !hasSheet1 || !hasSheet2 {
		t.Fatalf("expected sheets %q and %q, got %v", sheet1, sheet2, sheets)
	}

	// Verify Sheet 1 rows
	s1Rows, err := outExcel.GetRows(sheet1)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet1, err)
	}
	if len(s1Rows) < 4 {
		t.Fatalf("expected at least 4 rows in %s, got %d", sheet1, len(s1Rows))
	}

	// Header
	for i, h := range headers1 {
		if s1Rows[0][i] != h {
			t.Errorf("sheet1 header col %d mismatch: expected %q, got %q", i, h, s1Rows[0][i])
		}
	}

	// Row 1
	r1 := s1Rows[1]
	if r1[0] == "Сидоров Петр Алексеевич" {
		t.Errorf("FIO was not anonymized: %s", r1[0])
	}
	if !detector.ValidateSNILS(r1[1]) {
		t.Errorf("generated SNILS is invalid: %s", r1[1])
	}
	if !detector.ValidateINN12(r1[2]) {
		t.Errorf("generated INN is invalid: %s", r1[2])
	}
	if r1[3] == "4512 654321" {
		t.Errorf("passport was not anonymized: %s", r1[3])
	}
	if r1[4] == "+7 916 555-44-33" {
		t.Errorf("phone was not anonymized: %s", r1[4])
	}
	cleanCard := strings.ReplaceAll(r1[5], " ", "")
	if r1[5] == "2200 9876 5432 1098" || !strings.HasPrefix(cleanCard, "2200") {
		t.Errorf("card was not anonymized: %s", r1[5])
	}
	if r1[6] == "1992-11-20" {
		t.Errorf("date was not perturbed: %s", r1[6])
	}
	if r1[7] != "Инженер" {
		t.Errorf("non-PII column 'Должность' modified: expected 'Инженер', got %q", r1[7])
	}

	// Row 2 and determinism check
	r2 := s1Rows[2]
	if r2[0] == "Кузнецова Елена Павловна" {
		t.Errorf("FIO was not anonymized: %s", r2[0])
	}
	if r1[0] == r2[0] {
		t.Errorf("different input FIOs produced identical synthetic FIOs: %s", r1[0])
	}
	if r1[1] != r2[1] {
		t.Errorf("referential integrity violation: same input SNILS gave different output: %s vs %s", r1[1], r2[1])
	}
	if r1[2] != r2[2] {
		t.Errorf("referential integrity violation: same input INN gave different output: %s vs %s", r1[2], r2[2])
	}
	if r2[7] != "Менеджер" {
		t.Errorf("non-PII column 'Должность' modified: expected 'Менеджер', got %q", r2[7])
	}

	// Row 3 empty values preserved
	r3 := s1Rows[3]
	for i := 0; i < 7; i++ {
		if i < len(r3) && r3[i] != "" {
			t.Errorf("empty cell at col %d populated with value: %q", i, r3[i])
		}
	}
	if r3[7] != "Стажер" {
		t.Errorf("non-PII column row 3 mismatch: %q", r3[7])
	}

	// Verify Sheet 2
	s2Rows, err := outExcel.GetRows(sheet2)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet2, err)
	}
	if len(s2Rows) < 2 {
		t.Fatalf("expected at least 2 rows in %s, got %d", sheet2, len(s2Rows))
	}
	if s2Rows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("Sheet 2 client FIO not anonymized: %s", s2Rows[1][0])
	}
	if s2Rows[1][1] == "+7 900 123-45-67" {
		t.Errorf("Sheet 2 client phone not anonymized: %s", s2Rows[1][1])
	}
}

func TestExcelHandler_CompositeHeaders(t *testing.T) {
	anon := anonymizer.New("test-salt-excel-composite")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "Паспорт")
	_ = f.SetCellValue("Sheet1", "B1", "Дата выдачи паспорта")
	_ = f.SetCellValue("Sheet1", "C1", "Номер карты")
	_ = f.SetCellValue("Sheet1", "D1", "Срок действия карты")

	_ = f.SetCellValue("Sheet1", "A2", "4512 654321")
	_ = f.SetCellValue("Sheet1", "B2", "2018-06-25")
	_ = f.SetCellValue("Sheet1", "C2", "2200 9876 5432 1098")
	_ = f.SetCellValue("Sheet1", "D2", "2029-05-31")

	var inBuf bytes.Buffer
	_ = f.Write(&inBuf)
	_ = f.Close()

	var outBuf bytes.Buffer
	err := handler.Process(context.Background(), &inBuf, &outBuf, nil)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open output excel: %v", err)
	}
	defer outExcel.Close()

	rows, err := outExcel.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	r := rows[1]
	// Passport
	if r[0] == "4512 654321" {
		t.Errorf("passport was not anonymized: %s", r[0])
	}
	// "Дата выдачи паспорта" must be perturbed date, NOT passport
	if r[1] == "2018-06-25" {
		t.Errorf("passport date was not perturbed: %s", r[1])
	}
	if strings.Contains(r[1], " ") {
		t.Errorf("passport date was misclassified as passport: %s", r[1])
	}
	// "Номер карты"
	if r[2] == "2200 9876 5432 1098" {
		t.Errorf("card was not anonymized: %s", r[2])
	}
	// "Срок действия карты" must be perturbed date, NOT card
	if r[3] == "2029-05-31" {
		t.Errorf("card expiry date was not perturbed: %s", r[3])
	}
	if strings.HasPrefix(r[3], "2200") {
		t.Errorf("card expiry date was misclassified as card: %s", r[3])
	}
}

func TestExcelHandler_ManualColumnOverrides(t *testing.T) {
	anon := anonymizer.New("test-salt-excel-overrides")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "CustomField")
	_ = f.SetCellValue("Sheet1", "B1", "PlainText")
	_ = f.SetCellValue("Sheet1", "A2", "Петров Петр Петрович")
	_ = f.SetCellValue("Sheet1", "B2", "НеизменныйТекст")

	var inBuf bytes.Buffer
	_ = f.Write(&inBuf)
	_ = f.Close()

	var outBuf bytes.Buffer
	options := map[string]interface{}{
		"columns": map[string]string{
			"CustomField": "FIO",
		},
	}
	err := handler.Process(context.Background(), &inBuf, &outBuf, options)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open output: %v", err)
	}
	defer outExcel.Close()

	rows, err := outExcel.GetRows("Sheet1")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}
	if rows[1][0] == "Петров Петр Петрович" {
		t.Errorf("CustomField was not anonymized as FIO: %s", rows[1][0])
	}
	if rows[1][1] != "НеизменныйТекст" {
		t.Errorf("PlainText was modified: %s", rows[1][1])
	}
}

func TestExcelHandler_SpecificSheetOption(t *testing.T) {
	anon := anonymizer.New("test-salt-sheet-filter")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	f.SetSheetName("Sheet1", "TargetSheet")
	_, _ = f.NewSheet("IgnoredSheet")

	_ = f.SetCellValue("TargetSheet", "A1", "ФИО")
	_ = f.SetCellValue("TargetSheet", "A2", "Сидоров Сидор")
	_ = f.SetCellValue("IgnoredSheet", "A1", "ФИО")
	_ = f.SetCellValue("IgnoredSheet", "A2", "Не Обрабатывать")

	var inBuf bytes.Buffer
	_ = f.Write(&inBuf)
	_ = f.Close()

	var outBuf bytes.Buffer
	options := map[string]interface{}{
		"sheet": "TargetSheet",
	}
	err := handler.Process(context.Background(), &inBuf, &outBuf, options)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open output: %v", err)
	}
	defer outExcel.Close()

	sheets := outExcel.GetSheetList()
	if len(sheets) != 1 || sheets[0] != "TargetSheet" {
		t.Fatalf("expected only TargetSheet, got: %v", sheets)
	}
}

func TestExcelHandler_NilAnonymizer(t *testing.T) {
	handler := formats.NewExcelHandler(nil)
	var inBuf, outBuf bytes.Buffer
	err := handler.Process(context.Background(), &inBuf, &outBuf, nil)
	if err == nil {
		t.Fatal("expected error for nil anonymizer, got nil")
	}
}

func TestExcelHandler_InvalidExcelReader(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewExcelHandler(anon)
	var outBuf bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader("not an excel file"), &outBuf, nil)
	if err == nil {
		t.Fatal("expected error for invalid excel input, got nil")
	}
}

func TestExcelHandler_ContextCancellation(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "ФИО")
	_ = f.SetCellValue("Sheet1", "A2", "Иванов Иван")
	var buf bytes.Buffer
	_ = f.Write(&buf)
	_ = f.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	var out bytes.Buffer
	err := handler.Process(ctx, &buf, &out, nil)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestExcelHandler_DisabledColumnsBySheet(t *testing.T) {
	anon := anonymizer.New("test-salt-sheets")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Сотрудники"
	sheet2 := "Клиенты"
	f.SetSheetName("Sheet1", sheet1)
	_, _ = f.NewSheet(sheet2)

	_ = f.SetCellValue(sheet1, "A1", "ФИО")
	_ = f.SetCellValue(sheet1, "B1", "Должность")
	_ = f.SetCellValue(sheet1, "A2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet1, "B2", "Директор")

	_ = f.SetCellValue(sheet2, "A1", "ФИО")
	_ = f.SetCellValue(sheet2, "B1", "Статус")
	_ = f.SetCellValue(sheet2, "A2", "Петров Петр Петрович")
	_ = f.SetCellValue(sheet2, "B2", "VIP")

	var inBuf, outBuf bytes.Buffer
	if err := f.Write(&inBuf); err != nil {
		t.Fatalf("failed to write workbook: %v", err)
	}

	// Disable column 1 only for "Сотрудники", leave "Клиенты" enabled
	options := map[string]interface{}{
		"disabled_columns_by_sheet": map[string][]int{
			"Сотрудники": {1},
		},
	}

	if err := handler.Process(context.Background(), &inBuf, &outBuf, options); err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open processed file: %v", err)
	}
	defer outExcel.Close()

	// Sheet 1: FIO must NOT be anonymized
	s1Val, _ := outExcel.GetCellValue(sheet1, "A2")
	if s1Val != "Иванов Иван Иванович" {
		t.Errorf("expected Sheet 1 FIO to be untouched, got %q", s1Val)
	}

	// Sheet 2: FIO MUST be anonymized
	s2Val, _ := outExcel.GetCellValue(sheet2, "A2")
	if s2Val == "Петров Петр Петрович" || s2Val == "" {
		t.Errorf("expected Sheet 2 FIO to be anonymized, got %q", s2Val)
	}
}

func TestExcelHandler_ModeDelete(t *testing.T) {
	anon := anonymizer.New("test-salt-excel-mode-delete")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	defer f.Close()

	sheet := "Лист1"
	f.SetSheetName("Sheet1", sheet)

	_ = f.SetCellValue(sheet, "A1", "ID")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "C1", "Телефон")

	_ = f.SetCellValue(sheet, "A2", "1")
	_ = f.SetCellValue(sheet, "B2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet, "C2", "+7 999 123-45-67")

	var inBuf, outBuf bytes.Buffer
	if err := f.Write(&inBuf); err != nil {
		t.Fatalf("failed to write workbook: %v", err)
	}

	dm := depmap.New("test.xlsx")
	options := map[string]interface{}{
		"mode":   domain.ModeDelete,
		"depmap": dm,
	}

	if err := handler.Process(context.Background(), &inBuf, &outBuf, options); err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	outExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to open processed file: %v", err)
	}
	defer outExcel.Close()

	idVal, _ := outExcel.GetCellValue(sheet, "A2")
	if idVal != "1" {
		t.Errorf("expected ID untouched '1', got %q", idVal)
	}

	fioVal, _ := outExcel.GetCellValue(sheet, "B2")
	if fioVal != formats.DeletionMarker {
		t.Errorf("expected FIO %q, got %q", formats.DeletionMarker, fioVal)
	}

	phoneVal, _ := outExcel.GetCellValue(sheet, "C2")
	if phoneVal != formats.DeletionMarker {
		t.Errorf("expected Phone %q, got %q", formats.DeletionMarker, phoneVal)
	}

	if dm.Mode != "delete" {
		t.Errorf("expected dm.Mode 'delete', got %q", dm.Mode)
	}

	origFIO, ok := dm.GetCell(sheet, "ФИО", 2)
	if !ok || origFIO != "Иванов Иван Иванович" {
		t.Errorf("expected orig FIO in depmap, got %q (found: %v)", origFIO, ok)
	}

	origPhone, ok := dm.GetCell(sheet, "Телефон", 2)
	if !ok || origPhone != "+7 999 123-45-67" {
		t.Errorf("expected orig Phone in depmap, got %q (found: %v)", origPhone, ok)
	}

	foundFIOHeader, foundPhoneHeader := false, false
	for _, h := range dm.TargetHeaders {
		if h == "ФИО" {
			foundFIOHeader = true
		}
		if h == "Телефон" {
			foundPhoneHeader = true
		}
	}
	if !foundFIOHeader || !foundPhoneHeader {
		t.Errorf("expected TargetHeaders to have ФИО and Телефон, got %v", dm.TargetHeaders)
	}
}

func TestExcelHandler_Process_StrategySemanticTokens(t *testing.T) {
	anon := anonymizer.New("salt")
	handler := formats.NewExcelHandler(anon)

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ФИО")
	_ = f.SetCellValue(sheet, "B1", "Паспорт")
	_ = f.SetCellValue(sheet, "A2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet, "B2", "4508 123456")

	var inBuf bytes.Buffer
	if err := f.Write(&inBuf); err != nil {
		t.Fatalf("failed to write excel: %v", err)
	}

	dm := depmap.New("test.xlsx")
	opts := map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"salt":     "salt",
		"depmap":   dm,
	}

	var outBuf bytes.Buffer
	if err := handler.Process(context.Background(), &inBuf, &outBuf, opts); err != nil {
		t.Fatalf("process failed: %v", err)
	}

	resExcel, err := excelize.OpenReader(&outBuf)
	if err != nil {
		t.Fatalf("failed to read out excel: %v", err)
	}
	defer resExcel.Close()

	rows, err := resExcel.GetRows(sheet)
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	if len(rows) < 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Verify headers preserved intact
	if rows[0][0] != "ФИО" || rows[0][1] != "Паспорт" {
		t.Errorf("headers altered: %v", rows[0])
	}

	// Verify tokens
	if rows[1][0] != "[ФИО_0001]" {
		t.Errorf("expected [ФИО_0001], got %q", rows[1][0])
	}
	if rows[1][1] != "[Паспорт_0001]" {
		t.Errorf("expected [Паспорт_0001], got %q", rows[1][1])
	}

	// Verify depmap mapping
	if orig, ok := dm.Get("[ФИО_0001]"); !ok || orig != "Иванов Иван Иванович" {
		t.Errorf("expected depmap mapping for [ФИО_0001] to be Иванов Иван Иванович, got %q (ok=%v)", orig, ok)
	}
	if orig, ok := dm.Get("[Паспорт_0001]"); !ok || orig != "4508 123456" {
		t.Errorf("expected depmap mapping for [Паспорт_0001] to be 4508 123456, got %q (ok=%v)", orig, ok)
	}
}


