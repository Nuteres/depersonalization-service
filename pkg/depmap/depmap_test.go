package depmap_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dep_service/pkg/depmap"
)

func TestDepMap_PutGet(t *testing.T) {
	dm := depmap.New("test_clients.xlsx")
	if dm.OriginalFileName != "test_clients.xlsx" {
		t.Fatalf("expected OriginalFileName to be 'test_clients.xlsx', got '%s'", dm.OriginalFileName)
	}
	if dm.Version != 1 {
		t.Fatalf("expected Version 1, got %d", dm.Version)
	}
	if dm.RecordsCount != 0 {
		t.Fatalf("expected initial RecordsCount 0, got %d", dm.RecordsCount)
	}

	dm.Put("Смирнов Алексей Петрович", "Иванов Иван Иванович")
	dm.Put("452-190-883 12", "112-233-445 95")

	if dm.RecordsCount != 2 {
		t.Fatalf("expected RecordsCount 2, got %d", dm.RecordsCount)
	}

	val, found := dm.Get("Смирнов Алексей Петрович")
	if !found || val != "Иванов Иван Иванович" {
		t.Fatalf("expected 'Иванов Иван Иванович', got '%s' (found=%v)", val, found)
	}

	val, found = dm.Get("452-190-883 12")
	if !found || val != "112-233-445 95" {
		t.Fatalf("expected '112-233-445 95', got '%s' (found=%v)", val, found)
	}

	_, found = dm.Get("nonexistent")
	if found {
		t.Fatalf("expected nonexistent key to return found=false")
	}
}

func TestDepMap_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "clients.xlsx.depmap")
	password := "SecretPass123!"

	dm := depmap.New("clients.xlsx")
	dm.Put("Synthetic_Name_1", "Original_Name_1")
	dm.Put("Synthetic_SNILS_1", "Original_SNILS_1")
	dm.Put("Synthetic_Phone_1", "+79991234567")

	err := dm.SaveFile(filePath, password)
	if err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// Verify file is created and has at least header size
	fi, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if fi.Size() < 52 { // 8 magic + 16 salt + 12 nonce + 16 tag = 52
		t.Fatalf("file size is suspiciously small: %d bytes", fi.Size())
	}

	loaded, err := depmap.LoadFile(filePath, password)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if loaded.OriginalFileName != "clients.xlsx" {
		t.Errorf("expected OriginalFileName 'clients.xlsx', got '%s'", loaded.OriginalFileName)
	}
	if loaded.RecordsCount != 3 {
		t.Errorf("expected RecordsCount 3, got %d", loaded.RecordsCount)
	}
	if loaded.Version != 1 {
		t.Errorf("expected Version 1, got %d", loaded.Version)
	}
	if time.Since(loaded.CreatedAt) > time.Hour {
		t.Errorf("CreatedAt seems invalid: %v", loaded.CreatedAt)
	}

	for k, v := range dm.Mapping {
		got, ok := loaded.Get(k)
		if !ok || got != v {
			t.Errorf("mapping mismatch for %s: expected %s, got %s (ok=%v)", k, v, got, ok)
		}
	}
}

func TestDepMap_WrongPassword(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "clients.xlsx.depmap")
	correctPassword := "CorrectPassword123"
	wrongPassword := "WrongPassword456"

	dm := depmap.New("data.csv")
	dm.Put("Synthetic_A", "Original_A")

	if err := dm.SaveFile(filePath, correctPassword); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	_, err := depmap.LoadFile(filePath, wrongPassword)
	if err == nil {
		t.Fatalf("expected error when loading with wrong password, got nil")
	}
}

func TestDepMap_CorruptedData(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "corrupted.depmap")
	password := "SecurePassword"

	dm := depmap.New("data.csv")
	dm.Put("Synthetic_A", "Original_A")
	dm.Put("Synthetic_B", "Original_B")

	if err := dm.SaveFile(filePath, password); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// Corrupt ciphertext byte (after 36 bytes of header)
	if len(data) > 40 {
		data[len(data)-5] ^= 0xFF
	}
	corruptedPath := filepath.Join(tmpDir, "tampered.depmap")
	if err := os.WriteFile(corruptedPath, data, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	_, err = depmap.LoadFile(corruptedPath, password)
	if err == nil {
		t.Fatalf("expected GCM authentication error on corrupted data, got nil")
	}
}

func TestDepMap_SaveFile_EmptyPassword(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty_password.depmap")

	dm := depmap.New("unprotected.xlsx")
	dm.Put("Synthetic_Val", "Original_Val")

	// Save with empty password must fail with ErrEmptyPassword
	err := dm.SaveFile(filePath, "")
	if err == nil {
		t.Fatalf("expected error when saving with empty password, got nil")
	}
	if !errors.Is(err, depmap.ErrEmptyPassword) {
		t.Fatalf("expected ErrEmptyPassword, got: %v", err)
	}

	// Also whitespace-only password must fail
	err = dm.SaveFile(filePath, "   ")
	if !errors.Is(err, depmap.ErrEmptyPassword) {
		t.Fatalf("expected ErrEmptyPassword for whitespace password, got: %v", err)
	}
}

func TestDepMap_SaveFileWithAutoPassword(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "auto_password.depmap")

	dm := depmap.New("auto.xlsx")
	dm.Put("Syn1", "Orig1")

	// Save with empty password should auto-generate password
	effectivePwd, err := dm.SaveFileWithAutoPassword(filePath, "")
	if err != nil {
		t.Fatalf("SaveFileWithAutoPassword failed: %v", err)
	}
	if len(effectivePwd) != 16 {
		t.Fatalf("expected 16-char auto-generated password, got %d (%s)", len(effectivePwd), effectivePwd)
	}

	// Should be decryptable with effectivePwd
	loaded, err := depmap.LoadFile(filePath, effectivePwd)
	if err != nil {
		t.Fatalf("LoadFile with effective password failed: %v", err)
	}
	val, ok := loaded.Get("Syn1")
	if !ok || val != "Orig1" {
		t.Fatalf("expected 'Orig1', got '%s'", val)
	}

	// Loading with empty password should fail (new files are not encrypted with legacy DefaultMasterSecret)
	_, err = depmap.LoadFile(filePath, "")
	if err == nil {
		t.Fatalf("expected failure loading auto-password file with empty password")
	}

	// Providing explicit password to SaveFileWithAutoPassword preserves it
	filePath2 := filepath.Join(tmpDir, "explicit.depmap")
	explicitPwd, err := dm.SaveFileWithAutoPassword(filePath2, "CustomSecret123")
	if err != nil {
		t.Fatalf("SaveFileWithAutoPassword with explicit password failed: %v", err)
	}
	if explicitPwd != "CustomSecret123" {
		t.Fatalf("expected 'CustomSecret123', got '%s'", explicitPwd)
	}
	loaded2, err := depmap.LoadFile(filePath2, "CustomSecret123")
	if err != nil {
		t.Fatalf("failed to load explicit password file: %v", err)
	}
	if val, _ := loaded2.Get("Syn1"); val != "Orig1" {
		t.Fatalf("expected 'Orig1', got '%s'", val)
	}
}

func TestDepMap_LegacyDefaultMasterKey(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "legacy_default_key.depmap")

	dm := depmap.New("legacy.xlsx")
	dm.Put("Synthetic_Val", "Original_Val")

	// Create legacy file using DefaultMasterSecret directly
	if err := dm.SaveFile(filePath, depmap.DefaultMasterSecret); err != nil {
		t.Fatalf("failed to create legacy file: %v", err)
	}

	// Load with empty password should succeed via legacy fallback
	loaded, err := depmap.LoadFile(filePath, "")
	if err != nil {
		t.Fatalf("LoadFile with empty password on legacy file failed: %v", err)
	}

	got, ok := loaded.Get("Synthetic_Val")
	if !ok || got != "Original_Val" {
		t.Fatalf("expected 'Original_Val', got '%s'", got)
	}

	// Trying to open with an arbitrary non-empty wrong password should fail
	_, err = depmap.LoadFile(filePath, "SomeOtherPassword")
	if err == nil {
		t.Fatalf("expected failure when opening legacy file with non-empty wrong password")
	}
}

func TestDepMap_InspectKeyFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "inspect.depmap")

	dm := depmap.New("test.xlsx")
	dm.Put("syn", "orig")
	if err := dm.SaveFile(filePath, "pw"); err != nil {
		t.Fatalf("SaveFile failed: %v", err)
	}

	// Valid file
	valid, err := depmap.InspectKeyFile(filePath)
	if err != nil || !valid {
		t.Fatalf("expected InspectKeyFile to return true, nil; got %v, %v", valid, err)
	}

	// Non-existent file
	valid, err = depmap.InspectKeyFile(filepath.Join(tmpDir, "missing.depmap"))
	if err == nil || valid {
		t.Fatalf("expected InspectKeyFile to return false, err for missing file; got %v, %v", valid, err)
	}

	// Truncated file
	truncatedPath := filepath.Join(tmpDir, "truncated.depmap")
	if err := os.WriteFile(truncatedPath, []byte("DEPMAP01"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	valid, err = depmap.InspectKeyFile(truncatedPath)
	if err == nil || valid {
		t.Fatalf("expected InspectKeyFile to return false, err for truncated file; got %v, %v", valid, err)
	}

	// Bad magic header
	badMagicPath := filepath.Join(tmpDir, "badmagic.depmap")
	badData := make([]byte, 60)
	copy(badData, "WRONGMAG")
	if err := os.WriteFile(badMagicPath, badData, 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	valid, err = depmap.InspectKeyFile(badMagicPath)
	if valid {
		t.Fatalf("expected InspectKeyFile to return valid=false for bad magic header")
	}
	if err == nil || !strings.Contains(err.Error(), "magic") {
		t.Fatalf("expected error mentioning magic, got: %v", err)
	}
}

func TestDerivePBKDF2Key_RFC6070(t *testing.T) {
	// RFC 6070 PBKDF2 HMAC-SHA256 test vectors:
	// P = "password", S = "salt", c = 1, dkLen = 32
	// Expected: 120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b
	k1 := depmap.DerivePBKDF2Key([]byte("password"), []byte("salt"), 1, 32)
	expectedHex1 := "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"
	if fmt.Sprintf("%x", k1) != expectedHex1 {
		t.Fatalf("RFC 6070 test vector 1 mismatch:\ngot:  %x\nwant: %s", k1, expectedHex1)
	}

	// P = "password", S = "salt", c = 2, dkLen = 32
	// Expected: ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43
	k2 := depmap.DerivePBKDF2Key([]byte("password"), []byte("salt"), 2, 32)
	expectedHex2 := "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"
	if fmt.Sprintf("%x", k2) != expectedHex2 {
		t.Fatalf("RFC 6070 test vector 2 mismatch:\ngot:  %x\nwant: %s", k2, expectedHex2)
	}

	// P = "password", S = "salt", c = 4096, dkLen = 32
	// Expected: c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a
	k4096 := depmap.DerivePBKDF2Key([]byte("password"), []byte("salt"), 4096, 32)
	expectedHex4096 := "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"
	if fmt.Sprintf("%x", k4096) != expectedHex4096 {
		t.Fatalf("RFC 6070 test vector 4096 mismatch:\ngot:  %x\nwant: %s", k4096, expectedHex4096)
	}
}

func TestGenerateRandomPassword(t *testing.T) {
	pwd1, err := depmap.GenerateRandomPassword(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pwd1) != 16 {
		t.Fatalf("expected length 16, got %d", len(pwd1))
	}
	pwd2, _ := depmap.GenerateRandomPassword(16)
	if pwd1 == pwd2 {
		t.Fatalf("passwords should be unique")
	}
}

func TestDepMap_CellAndOrdinalMappings(t *testing.T) {
	dm := depmap.New("test.xlsx")
	dm.Mode = "delete"
	dm.PutCell("Лист1", "ФИО", 5, "Иванов Иван Иванович")
	dm.PutCell("Лист1", "Телефон", 5, "+79991234567")
	dm.AddTargetHeader("ФИО")
	dm.AddTargetHeader("Телефон")
	dm.PutOrdinal("Паспорт серии 1234")

	val, ok := dm.GetCell("Лист1", "ФИО", 5)
	if !ok || val != "Иванов Иван Иванович" {
		t.Fatalf("expected 'Иванов Иван Иванович', got %q (found: %v)", val, ok)
	}

	tmpFile := filepath.Join(t.TempDir(), "test.depmap")
	pwd, err := dm.SaveFileWithAutoPassword(tmpFile, "TestPassword123!")
	if err != nil {
		t.Fatalf("SaveFileWithAutoPassword failed: %v", err)
	}

	loaded, err := depmap.LoadFile(tmpFile, pwd)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if loaded.Mode != "delete" {
		t.Errorf("expected Mode 'delete', got %q", loaded.Mode)
	}
	loadedVal, ok := loaded.GetCell("Лист1", "ФИО", 5)
	if !ok || loadedVal != "Иванов Иван Иванович" {
		t.Errorf("expected loaded cell value 'Иванов Иван Иванович', got %q", loadedVal)
	}
	if len(loaded.TargetHeaders) != 2 || loaded.TargetHeaders[0] != "ФИО" {
		t.Errorf("unexpected TargetHeaders: %v", loaded.TargetHeaders)
	}
	if len(loaded.OrdinalMappings) != 1 || loaded.OrdinalMappings[0] != "Паспорт серии 1234" {
		t.Errorf("unexpected OrdinalMappings: %v", loaded.OrdinalMappings)
	}
}

func TestDepMap_TargetHeadersAndGetCellEdgeCases(t *testing.T) {
	dm := depmap.New("test.xlsx")

	// GetCell on empty/nil CellMappings
	val, ok := dm.GetCell("Лист1", "Колонка", 1)
	if ok || val != "" {
		t.Fatalf("expected empty and false for GetCell on uninitialized CellMappings, got %q, %v", val, ok)
	}

	// AddTargetHeader with empty / whitespace strings
	dm.AddTargetHeader("")
	dm.AddTargetHeader("   ")
	if len(dm.TargetHeaders) != 0 {
		t.Fatalf("expected 0 headers, got %v", dm.TargetHeaders)
	}

	// AddTargetHeader with duplicates
	dm.AddTargetHeader("ФИО")
	dm.AddTargetHeader("  ФИО  ")
	if len(dm.TargetHeaders) != 1 || dm.TargetHeaders[0] != "ФИО" {
		t.Fatalf("expected duplicate header to be ignored, got %v", dm.TargetHeaders)
	}

	// PutCell and GetCell with surrounding whitespace in sheet and header
	dm.PutCell("  Лист1  ", "  ФИО  ", 10, "Петров")
	val, ok = dm.GetCell("Лист1", "ФИО", 10)
	if !ok || val != "Петров" {
		t.Fatalf("expected 'Петров', got %q (found: %v)", val, ok)
	}

	// Non-existent cell lookup
	_, ok = dm.GetCell("Лист1", "ФИО", 999)
	if ok {
		t.Fatalf("expected false for nonexistent cell")
	}
}

func TestDepMap_BackwardCompatibilityV1(t *testing.T) {
	// Verify that a standard v1 DepMap without new fields still loads cleanly
	tmpFile := filepath.Join(t.TempDir(), "v1.depmap")
	v1dm := depmap.New("legacy.csv")
	v1dm.Put("synthetic_1", "original_1")

	pwd, err := v1dm.SaveFileWithAutoPassword(tmpFile, "SecretPass123!")
	if err != nil {
		t.Fatalf("SaveFileWithAutoPassword failed: %v", err)
	}

	loaded, err := depmap.LoadFile(tmpFile, pwd)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if loaded.Mode != "" {
		t.Errorf("expected empty Mode for legacy v1, got %q", loaded.Mode)
	}
	if loaded.CellMappings != nil {
		t.Errorf("expected nil CellMappings for legacy v1, got %v", loaded.CellMappings)
	}
	if loaded.TargetHeaders != nil {
		t.Errorf("expected nil TargetHeaders for legacy v1, got %v", loaded.TargetHeaders)
	}
	if loaded.OrdinalMappings != nil {
		t.Errorf("expected nil OrdinalMappings for legacy v1, got %v", loaded.OrdinalMappings)
	}
	val, ok := loaded.Get("synthetic_1")
	if !ok || val != "original_1" {
		t.Fatalf("expected 'original_1', got %q", val)
	}
	_, ok = loaded.GetCell("Sheet", "Col", 1)
	if ok {
		t.Fatalf("expected GetCell on legacy v1 to return false")
	}
}


