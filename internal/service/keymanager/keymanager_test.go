package keymanager_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/internal/service/keymanager"
	"dep_service/pkg/depmap"
)

func TestKeyManager_InspectKey_NonexistentFile(t *testing.T) {
	km := keymanager.New()
	_, err := km.InspectKey("/nonexistent/path/to/key.depmap", "secret")
	if err == nil {
		t.Fatalf("expected error for nonexistent key file, got nil")
	}
}

func TestKeyManager_InspectKey_EmptyPath(t *testing.T) {
	km := keymanager.New()
	_, err := km.InspectKey("", "secret")
	if err == nil {
		t.Fatalf("expected error for empty key path, got nil")
	}
}

func TestKeyManager_InspectKey_ValidAndInvalidPassword(t *testing.T) {
	km := keymanager.New()
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "test.depmap")

	mappings := map[string]string{
		"[NAME_1]":  "Иванов Иван",
		"[PHONE_1]": "+79991234567",
	}

	err := km.CreateContainer(keyPath, "correct_password", "source.csv", mappings)
	if err != nil {
		t.Fatalf("failed to create key container: %v", err)
	}

	// 1. Inspect with correct password
	info, err := km.InspectKey(keyPath, "correct_password")
	if err != nil {
		t.Fatalf("unexpected error inspecting valid key: %v", err)
	}
	if info == nil {
		t.Fatalf("expected non-nil KeyFileInfo")
	}
	if !info.Valid {
		t.Errorf("expected info.Valid to be true, got false (error: %s)", info.Error)
	}
	if info.OriginalFileName != "source.csv" {
		t.Errorf("expected original file name 'source.csv', got '%s'", info.OriginalFileName)
	}
	if info.RecordsCount != 2 {
		t.Errorf("expected 2 records, got %d", info.RecordsCount)
	}
	if info.CreatedAt == "" {
		t.Errorf("expected non-empty CreatedAt")
	}

	// 2. Inspect with invalid password
	infoWrong, err := km.InspectKey(keyPath, "wrong_password")
	if err == nil {
		t.Fatalf("expected error for wrong password, got nil")
	}
	if !errors.Is(err, domain.ErrInvalidPassword) {
		t.Errorf("expected error to wrap domain.ErrInvalidPassword, got: %v", err)
	}
	if infoWrong != nil && infoWrong.Valid {
		t.Errorf("expected infoWrong.Valid to be false")
	}
}

func TestKeyManager_InspectKey_CorruptedFile(t *testing.T) {
	km := keymanager.New()
	tmpDir := t.TempDir()
	corruptedPath := filepath.Join(tmpDir, "corrupted.depmap")

	if err := os.WriteFile(corruptedPath, []byte("NOT_A_VALID_DEPMAP_FILE_PAYLOAD"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	info, err := km.InspectKey(corruptedPath, "any_pass")
	// Ported from InspectKeyFile: returns invalid info, error message populated
	if info == nil {
		t.Fatalf("expected non-nil KeyFileInfo for corrupted file")
	}
	if info.Valid {
		t.Errorf("expected info.Valid=false for corrupted file")
	}
	if info.Error == "" {
		t.Errorf("expected non-empty Error in KeyFileInfo for corrupted file")
	}
	_ = err
}

func TestKeyManager_ExtractMappings_CSV(t *testing.T) {
	km := keymanager.New()
	tmpDir := t.TempDir()

	inCSV := filepath.Join(tmpDir, "input.csv")
	outCSV := filepath.Join(tmpDir, "output.csv")

	inContent := "Name;Email;Phone\nИванов;ivanov@example.com;+79991112233\nПетров;petrov@example.com;+79994445566\n"
	outContent := "Name;Email;Phone\n[FIO_1];[EMAIL_1];[PHONE_1]\n[FIO_2];[EMAIL_2];[PHONE_2]\n"

	if err := os.WriteFile(inCSV, []byte(inContent), 0644); err != nil {
		t.Fatalf("failed to write input CSV: %v", err)
	}
	if err := os.WriteFile(outCSV, []byte(outContent), 0644); err != nil {
		t.Fatalf("failed to write output CSV: %v", err)
	}

	dm := depmap.New("input.csv")
	if err := km.ExtractMappings(inCSV, outCSV, ".csv", dm); err != nil {
		t.Fatalf("ExtractMappings failed: %v", err)
	}

	if dm.RecordsCount != 6 {
		t.Fatalf("expected 6 mappings, got %d", dm.RecordsCount)
	}

	expected := map[string]string{
		"[FIO_1]":   "Иванов",
		"[EMAIL_1]": "ivanov@example.com",
		"[PHONE_1]": "+79991112233",
		"[FIO_2]":   "Петров",
		"[EMAIL_2]": "petrov@example.com",
		"[PHONE_2]": "+79994445566",
	}

	for anon, orig := range expected {
		val, ok := dm.Get(anon)
		if !ok || val != orig {
			t.Errorf("mapping mismatch for %s: expected %s, got %s (ok=%v)", anon, orig, val, ok)
		}
	}
}

func TestKeyManager_ExtractMappings_Excel(t *testing.T) {
	km := keymanager.New()
	tmpDir := t.TempDir()

	inXLSX := filepath.Join(tmpDir, "input.xlsx")
	outXLSX := filepath.Join(tmpDir, "output.xlsx")

	fIn := excelize.NewFile()
	_ = fIn.SetCellValue("Sheet1", "A1", "Иванов Иван")
	_ = fIn.SetCellValue("Sheet1", "B1", "ivan@test.com")
	if err := fIn.SaveAs(inXLSX); err != nil {
		t.Fatalf("failed to save input xlsx: %v", err)
	}
	_ = fIn.Close()

	fOut := excelize.NewFile()
	_ = fOut.SetCellValue("Sheet1", "A1", "[NAME_1]")
	_ = fOut.SetCellValue("Sheet1", "B1", "[EMAIL_1]")
	if err := fOut.SaveAs(outXLSX); err != nil {
		t.Fatalf("failed to save output xlsx: %v", err)
	}
	_ = fOut.Close()

	dm := depmap.New("input.xlsx")
	if err := km.ExtractMappings(inXLSX, outXLSX, ".xlsx", dm); err != nil {
		t.Fatalf("ExtractMappings failed: %v", err)
	}

	if dm.RecordsCount != 2 {
		t.Fatalf("expected 2 mappings, got %d", dm.RecordsCount)
	}
	if val, ok := dm.Get("[NAME_1]"); !ok || val != "Иванов Иван" {
		t.Errorf("expected Иванов Иван, got %s (ok=%v)", val, ok)
	}
	if val, ok := dm.Get("[EMAIL_1]"); !ok || val != "ivan@test.com" {
		t.Errorf("expected ivan@test.com, got %s (ok=%v)", val, ok)
	}
}

func TestKeyManager_CreateContainer_Validation(t *testing.T) {
	km := keymanager.New()
	err := km.CreateContainer("", "pass", "file.csv", nil)
	if err == nil {
		t.Errorf("expected error for empty key path, got nil")
	}
}
