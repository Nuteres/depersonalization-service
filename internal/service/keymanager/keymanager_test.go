package keymanager_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"dep_service/internal/domain"
	"dep_service/internal/service/keymanager"
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

func TestKeyManager_CreateContainer_Validation(t *testing.T) {
	km := keymanager.New()
	err := km.CreateContainer("", "pass", "file.csv", nil)
	if err == nil {
		t.Errorf("expected error for empty key path, got nil")
	}
}
