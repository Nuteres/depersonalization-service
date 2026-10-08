package formats_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
)

// encodeToWindows1251 converts a UTF-8 string to Windows-1251 byte slice.
func encodeToWindows1251(t *testing.T, s string) []byte {
	t.Helper()
	encoder := charmap.Windows1251.NewEncoder()
	encoded, err := encoder.Bytes([]byte(s))
	if err != nil {
		t.Fatalf("failed to encode string to windows-1251: %v", err)
	}
	return encoded
}

func TestCSVHandler_Windows1251_RussianAndEnglish(t *testing.T) {
	anon := anonymizer.New("test-salt-win1251")
	handler := formats.NewCSVHandler(anon)

	// CSV with Russian Cyrillic and English Latin text encoded in Windows-1251 (CP1251)
	utf8Input := "ID,ФИО,Email,Department,Примечание\n" +
		"1,Иванов Иван Иванович,ivanov@example.com,Engineering,Штатный сотрудник\n" +
		"2,Петрова Анна Сергеевна,petrova@example.com,Marketing,Внешний консультант\n"

	win1251Bytes := encodeToWindows1251(t, utf8Input)

	var out bytes.Buffer
	ctx := context.Background()

	// Process with write_bom=true (as used by the desktop application)
	options := map[string]interface{}{
		"write_bom": true,
	}

	err := handler.Process(ctx, bytes.NewReader(win1251Bytes), &out, options)
	if err != nil {
		t.Fatalf("handler.Process failed on Windows-1251 CSV: %v", err)
	}

	outBytes := out.Bytes()

	// 1. Verify output begins with UTF-8 BOM (\xef\xbb\xbf) for Excel compatibility
	if !bytes.HasPrefix(outBytes, formats.UTF8BOM) {
		t.Fatalf("expected output to begin with UTF-8 BOM, got prefix: %x", outBytes[:min(len(outBytes), 3)])
	}

	// 2. Read output with encoding-aware reader
	reader, delim, err := formats.NewEncodingAwareCSVReader(bytes.NewReader(outBytes), 0)
	if err != nil {
		t.Fatalf("failed to create reader for output: %v", err)
	}
	if delim != ',' {
		t.Errorf("expected delimiter ',', got %q", delim)
	}

	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read output csv: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Verify Header: both English ("ID", "Email", "Department") and Russian ("ФИО", "Примечание")
	expectedHeader := []string{"ID", "ФИО", "Email", "Department", "Примечание"}
	for i, h := range expectedHeader {
		if rows[0][i] != h {
			t.Errorf("header col %d mismatch: got %q, want %q", i, rows[0][i], h)
		}
	}

	// Verify row 1:
	// Col 0: ID "1" (English/ASCII)
	if rows[1][0] != "1" {
		t.Errorf("expected ID '1', got %q", rows[1][0])
	}
	// Col 1: ФИО must be anonymized to a valid Cyrillic name, NOT question marks
	if strings.Contains(rows[1][1], "?") || strings.Contains(rows[1][1], "\ufffd") {
		t.Errorf("anonymized Cyrillic FIO contains question marks: %q", rows[1][1])
	}
	if rows[1][1] == "Иванов Иван Иванович" {
		t.Errorf("FIO was not anonymized: %q", rows[1][1])
	}
	// Col 2: Email (English/Latin) preserved or anonymized cleanly
	if !strings.Contains(rows[1][2], "@") {
		t.Errorf("email column invalid: %q", rows[1][2])
	}
	// Col 3: Department "Engineering" (English) preserved
	if rows[1][3] != "Engineering" {
		t.Errorf("expected Department 'Engineering', got %q", rows[1][3])
	}
	// Col 4: Примечание "Штатный сотрудник" (Russian Cyrillic non-PII) preserved without question marks
	if rows[1][4] != "Штатный сотрудник" {
		t.Errorf("expected note 'Штатный сотрудник', got %q", rows[1][4])
	}

	// Verify row 2:
	if rows[2][0] != "2" {
		t.Errorf("expected ID '2', got %q", rows[2][0])
	}
	if strings.Contains(rows[2][1], "?") || strings.Contains(rows[2][1], "\ufffd") {
		t.Errorf("anonymized Cyrillic FIO row 2 contains question marks: %q", rows[2][1])
	}
	if rows[2][4] != "Внешний консультант" {
		t.Errorf("expected note 'Внешний консультант', got %q", rows[2][4])
	}
}

func TestCSVHandler_Windows1251_SemicolonDelimiter(t *testing.T) {
	anon := anonymizer.New("test-salt-semi-win1251")
	handler := formats.NewCSVHandler(anon)

	// Semicolon delimited (very common in Russian Excel exports) in Windows-1251
	utf8Input := "Номер;Сотрудник;Телефон;Статус\n" +
		"101;Сидоров Сидор Сидорович;+7 999 111-22-33;Работает\n"

	win1251Bytes := encodeToWindows1251(t, utf8Input)

	var out bytes.Buffer
	ctx := context.Background()

	err := handler.Process(ctx, bytes.NewReader(win1251Bytes), &out, map[string]interface{}{"write_bom": true})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	reader, delim, err := formats.NewEncodingAwareCSVReader(bytes.NewReader(out.Bytes()), 0)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	if delim != ';' {
		t.Errorf("expected delimiter ';', got %q", delim)
	}

	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read rows: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0][1] != "Сотрудник" || rows[0][3] != "Статус" {
		t.Errorf("headers corrupt: %v", rows[0])
	}

	if rows[1][3] != "Работает" {
		t.Errorf("expected status 'Работает', got %q", rows[1][3])
	}
}

func TestRestoreCSV_Windows1251_RestoreRoundtrip(t *testing.T) {
	dm := depmap.New("win1251.csv")
	dm.Put("Смирнов Иван Сергеевич", "Иванов Иван Иванович")

	anonInput := "ID;ФИО;Город\n1;Смирнов Иван Сергеевич;Москва\n"
	win1251Bytes := encodeToWindows1251(t, anonInput)

	var out bytes.Buffer
	ctx := context.Background()

	count, err := formats.RestoreCSV(ctx, bytes.NewReader(win1251Bytes), &out, dm, true)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 restored cell, got %d", count)
	}

	reader, delim, err := formats.NewEncodingAwareCSVReader(bytes.NewReader(out.Bytes()), 0)
	if err != nil {
		t.Fatalf("reader creation failed: %v", err)
	}
	if delim != ';' {
		t.Errorf("expected delimiter ';', got %q", delim)
	}

	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}

	if rows[1][1] != "Иванов Иван Иванович" {
		t.Errorf("restoration failed, expected 'Иванов Иван Иванович', got %q", rows[1][1])
	}
	if rows[1][2] != "Москва" {
		t.Errorf("city corrupted, expected 'Москва', got %q", rows[1][2])
	}
}
