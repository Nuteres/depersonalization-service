package formats_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/detector"
	"dep_service/pkg/formats"
)

func TestCSVHandler_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewCSVHandler(anon)

	exts := handler.SupportedExtensions()
	foundCSV := false
	for _, ext := range exts {
		if ext == ".csv" {
			foundCSV = true
		}
	}
	if !foundCSV {
		t.Fatalf("expected .csv in supported extensions, got %v", exts)
	}
}

func TestCSVHandler_Process(t *testing.T) {
	anon := anonymizer.New("test-salt-csv-152fz")
	handler := formats.NewCSVHandler(anon)

	inputData := `ФИО,СНИЛС,ИНН,Паспорт,Телефон,Номер карты,Дата рождения,Отдел
Иванов Иван Иванович,112-233-445 95,500100732259,4510 123456,+7 999 123-45-67,2200 1234 5678 9012,1985-05-12,Бухгалтерия
Петрова Анна Сергеевна,112-233-445 95,500100732259,4510 654321,+7 916 234-56-78,2200 8765 4321 0987,15.08.1990,IT отдел
,,,,,,,Склад
`

	r := strings.NewReader(inputData)
	var w bytes.Buffer

	ctx := context.Background()
	err := handler.Process(ctx, r, &w, nil)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	reader := csv.NewReader(&w)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output csv: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Verify Header
	expectedHeader := []string{"ФИО", "СНИЛС", "ИНН", "Паспорт", "Телефон", "Номер карты", "Дата рождения", "Отдел"}
	for i, h := range expectedHeader {
		if rows[0][i] != h {
			t.Errorf("header col %d mismatch: expected %q, got %q", i, h, rows[0][i])
		}
	}

	// Row 1 checks
	row1 := rows[1]
	if row1[0] == "Иванов Иван Иванович" {
		t.Errorf("FIO was not anonymized: %s", row1[0])
	}
	if !detector.ValidateSNILS(row1[1]) {
		t.Errorf("generated SNILS is invalid: %s", row1[1])
	}
	if !detector.ValidateINN12(row1[2]) {
		t.Errorf("generated INN is invalid: %s", row1[2])
	}
	if row1[3] == "4510 123456" || !strings.Contains(row1[3], " ") {
		t.Errorf("passport was not properly anonymized: %s", row1[3])
	}
	if row1[4] == "+7 999 123-45-67" || !strings.HasPrefix(row1[4], "+7 9") {
		t.Errorf("phone was not properly anonymized: %s", row1[4])
	}
	cleanCard := strings.ReplaceAll(row1[5], " ", "")
	if row1[5] == "2200 1234 5678 9012" || !strings.HasPrefix(cleanCard, "2200") {
		t.Errorf("card was not properly anonymized: %s", row1[5])
	}
	if row1[6] == "1985-05-12" {
		t.Errorf("date was not perturbed: %s", row1[6])
	}
	if row1[7] != "Бухгалтерия" {
		t.Errorf("non-PII column 'Отдел' was modified: expected 'Бухгалтерия', got %q", row1[7])
	}

	// Row 2 checks & Determinism
	row2 := rows[2]
	if row2[0] == "Петрова Анна Сергеевна" {
		t.Errorf("FIO was not anonymized: %s", row2[0])
	}
	if row1[0] == row2[0] {
		t.Errorf("different input FIOs produced identical synthetic FIOs: %s", row1[0])
	}
	// Identical SNILS in input must produce identical SNILS in output (referential integrity)
	if row1[1] != row2[1] {
		t.Errorf("referential integrity violation: same input SNILS produced different outputs: %s vs %s", row1[1], row2[1])
	}
	// Identical INN in input must produce identical INN in output
	if row1[2] != row2[2] {
		t.Errorf("referential integrity violation: same input INN produced different outputs: %s vs %s", row1[2], row2[2])
	}
	if row2[7] != "IT отдел" {
		t.Errorf("non-PII column 'Отдел' was modified: expected 'IT отдел', got %q", row2[7])
	}

	// Row 3 (empty values preserved)
	row3 := rows[3]
	for i := 0; i < 7; i++ {
		if row3[i] != "" {
			t.Errorf("empty cell at col %d was populated with value: %q", i, row3[i])
		}
	}
	if row3[7] != "Склад" {
		t.Errorf("non-PII column 'Отдел' row 3 mismatch: %q", row3[7])
	}
}

func TestCSVHandler_CompositeHeaders(t *testing.T) {
	anon := anonymizer.New("test-salt-composite")
	handler := formats.NewCSVHandler(anon)

	// Headers containing "Дата выдачи паспорта" and "Срок действия карты"
	inputData := "Паспорт,Дата выдачи паспорта,Номер карты,Срок действия карты\n" +
		"4510 123456,2015-04-10,2200 1234 5678 9012,2028-12-31\n"
	var w bytes.Buffer

	ctx := context.Background()
	err := handler.Process(ctx, strings.NewReader(inputData), &w, nil)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	reader := csv.NewReader(&w)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output csv: %v", err)
	}

	row := rows[1]
	// row[0] is Passport (should be anonymized passport)
	if row[0] == "4510 123456" {
		t.Errorf("passport was not anonymized: %s", row[0])
	}
	// row[1] is "Дата выдачи паспорта" (must be perturbed date, e.g. YYYY-MM-DD, NOT a passport string!)
	if row[1] == "2015-04-10" {
		t.Errorf("passport issue date was not perturbed: %s", row[1])
	}
	if strings.Contains(row[1], " ") {
		t.Errorf("passport issue date was misclassified as passport: %s", row[1])
	}

	// row[2] is Card (should be anonymized card)
	if row[2] == "2200 1234 5678 9012" {
		t.Errorf("card was not anonymized: %s", row[2])
	}
	// row[3] is "Срок действия карты" (must be perturbed date, e.g. YYYY-MM-DD, NOT a 16-digit card!)
	if row[3] == "2028-12-31" {
		t.Errorf("card expiration date was not perturbed: %s", row[3])
	}
	if strings.HasPrefix(row[3], "2200") {
		t.Errorf("card expiration date was misclassified as card: %s", row[3])
	}
}

func TestCSVHandler_TrimUTF8BOM(t *testing.T) {
	anon := anonymizer.New("test-salt-bom")
	handler := formats.NewCSVHandler(anon)

	// UTF-8 BOM is \xef\xbb\xbf (\ufeff)
	inputData := "\ufeffФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	var w bytes.Buffer

	ctx := context.Background()
	options := map[string]interface{}{
		"columns": map[string]string{
			"ФИО": "FIO",
		},
	}
	err := handler.Process(ctx, strings.NewReader(inputData), &w, options)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	reader := csv.NewReader(&w)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output csv: %v", err)
	}

	// Verify header has no BOM
	if strings.HasPrefix(rows[0][0], "\ufeff") {
		t.Errorf("header still contains UTF-8 BOM: %q", rows[0][0])
	}
	if rows[0][0] != "ФИО" {
		t.Errorf("expected clean header 'ФИО', got %q", rows[0][0])
	}

	// Verify row 1 was anonymized using the options override
	if rows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("FIO was not anonymized when BOM was present: %s", rows[1][0])
	}
}

func TestCSVHandler_CustomDelimiter(t *testing.T) {
	anon := anonymizer.New("test-salt-delim")
	handler := formats.NewCSVHandler(anon)

	inputData := "ФИО;СНИЛС;Статус\nИванов Иван Иванович;112-233-445 95;Активен\n"
	var w bytes.Buffer

	ctx := context.Background()
	options := map[string]interface{}{
		"delimiter": ';',
	}
	err := handler.Process(ctx, strings.NewReader(inputData), &w, options)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	output := w.String()
	if !strings.Contains(output, ";") {
		t.Fatalf("expected semicolon delimited output, got: %s", output)
	}
	if strings.Contains(output, "Иванов Иван Иванович") {
		t.Errorf("FIO was not anonymized in semicolon-delimited CSV")
	}
	if !strings.Contains(output, "Активен") {
		t.Errorf("non-PII column lost")
	}
}

func TestCSVHandler_AutoDetectDelimiter(t *testing.T) {
	anon := anonymizer.New("test-salt-autodelim")
	handler := formats.NewCSVHandler(anon)

	// Semicolon delimited without options
	inputData := "ФИО;СНИЛС;Статус\nИванов Иван Иванович;112-233-445 95;Активен\n"
	var w bytes.Buffer

	ctx := context.Background()
	err := handler.Process(ctx, strings.NewReader(inputData), &w, nil)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	output := w.String()
	if !strings.Contains(output, ";") {
		t.Fatalf("expected auto-detected semicolon delimited output, got: %s", output)
	}
	if strings.Contains(output, "Иванов Иван Иванович") {
		t.Errorf("FIO was not anonymized")
	}
}

func TestCSVHandler_ManualColumnOverrides(t *testing.T) {
	anon := anonymizer.New("test-salt-overrides")
	handler := formats.NewCSVHandler(anon)

	inputData := "CustomColumn,ОбычныйТекст\nИванов Иван Иванович,НекийТекст\n"
	var w bytes.Buffer

	ctx := context.Background()
	options := map[string]interface{}{
		"columns": map[string]string{
			"CustomColumn": "FIO",
		},
	}
	err := handler.Process(ctx, strings.NewReader(inputData), &w, options)
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	reader := csv.NewReader(&w)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	if rows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("CustomColumn was not anonymized as FIO: %s", rows[1][0])
	}
	if rows[1][1] != "НекийТекст" {
		t.Errorf("ОбычныйТекст should not be modified: %s", rows[1][1])
	}
}

func TestCSVHandler_EmptyFile(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewCSVHandler(anon)

	var w bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader(""), &w, nil)
	if err != nil {
		t.Fatalf("expected nil on empty input, got: %v", err)
	}
}

func TestCSVHandler_NilAnonymizer(t *testing.T) {
	handler := formats.NewCSVHandler(nil)
	var w bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader("col1\nval1"), &w, nil)
	if err == nil {
		t.Fatal("expected error with nil anonymizer, got nil")
	}
}

func TestCSVHandler_ContextCancellation(t *testing.T) {
	anon := anonymizer.New("test-salt")
	handler := formats.NewCSVHandler(anon)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	inputData := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	var w bytes.Buffer

	err := handler.Process(ctx, strings.NewReader(inputData), &w, nil)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

func TestCSVHandler_ModeDelete(t *testing.T) {
	anon := anonymizer.New("test-salt-csv-mode-delete")
	handler := formats.NewCSVHandler(anon)

	inputCSV := "ID,ФИО,Телефон\n1,Иванов Иван Иванович,+7 999 123-45-67\n"
	inBuf := strings.NewReader(inputCSV)
	var outBuf bytes.Buffer

	dm := depmap.New("test.csv")
	options := map[string]interface{}{
		"mode":   domain.ModeDelete,
		"depmap": dm,
	}

	if err := handler.Process(context.Background(), inBuf, &outBuf, options); err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	r := csv.NewReader(bytes.NewReader(outBuf.Bytes()))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output csv: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[1][0] != "1" {
		t.Errorf("expected ID '1', got %q", records[1][0])
	}
	if records[1][1] != formats.DeletionMarker {
		t.Errorf("expected FIO %q, got %q", formats.DeletionMarker, records[1][1])
	}
	if records[1][2] != formats.DeletionMarker {
		t.Errorf("expected Phone %q, got %q", formats.DeletionMarker, records[1][2])
	}

	if dm.Mode != "delete" {
		t.Errorf("expected dm.Mode 'delete', got %q", dm.Mode)
	}

	origFIO, ok := dm.GetCell("", "ФИО", 2)
	if !ok || origFIO != "Иванов Иван Иванович" {
		t.Errorf("expected orig FIO in depmap, got %q (found: %v)", origFIO, ok)
	}

	origPhone, ok := dm.GetCell("", "Телефон", 2)
	if !ok || origPhone != "+7 999 123-45-67" {
		t.Errorf("expected orig Phone in depmap, got %q (found: %v)", origPhone, ok)
	}

	foundFIO, foundPhone := false, false
	for _, h := range dm.TargetHeaders {
		if h == "ФИО" {
			foundFIO = true
		}
		if h == "Телефон" {
			foundPhone = true
		}
	}
	if !foundFIO || !foundPhone {
		t.Errorf("expected TargetHeaders to have ФИО and Телефон, got %v", dm.TargetHeaders)
	}
}
