package depmap_test

import (
	"bytes"
	"errors"
	"testing"

	"dep_service/pkg/depmap"
)

func TestAES256GCMProvider_Name(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	if provider.Name() != "AES-256-GCM" {
		t.Fatalf("expected provider name 'AES-256-GCM', got '%s'", provider.Name())
	}
}

func TestAES256GCMProvider_EncryptDecryptRoundtrip(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	plaintext := []byte("Sensitive PII 152-FZ test data: Иванов Иван Иванович")
	password := "CorrectSecurePassword123"

	salt, nonce, ciphertext, err := provider.Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if len(salt) != depmap.SaltSize {
		t.Fatalf("expected salt size %d, got %d", depmap.SaltSize, len(salt))
	}
	if len(nonce) != depmap.NonceSize {
		t.Fatalf("expected nonce size %d, got %d", depmap.NonceSize, len(nonce))
	}
	if len(ciphertext) <= len(plaintext) {
		t.Fatalf("expected ciphertext to include tag (longer than plaintext)")
	}

	decrypted, err := provider.Decrypt(salt, nonce, ciphertext, password)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted text mismatch:\ngot:  %s\nwant: %s", string(decrypted), string(plaintext))
	}
}

func TestAES256GCMProvider_WrongPassword(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	plaintext := []byte("secret payload")
	correctPassword := "Password123"
	wrongPassword := "WrongPassword456"

	salt, nonce, ciphertext, err := provider.Encrypt(plaintext, correctPassword)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = provider.Decrypt(salt, nonce, ciphertext, wrongPassword)
	if err == nil {
		t.Fatalf("expected Decrypt to fail with wrong password, got nil")
	}
	if !errors.Is(err, depmap.ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed, got: %v", err)
	}
}

func TestAES256GCMProvider_EmptyPassword(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	plaintext := []byte("payload")

	_, _, _, err := provider.Encrypt(plaintext, "")
	if err == nil {
		t.Fatalf("expected Encrypt with empty password to fail")
	}
	if !errors.Is(err, depmap.ErrEmptyPassword) {
		t.Fatalf("expected ErrEmptyPassword, got: %v", err)
	}
}

func TestAES256GCMProvider_CorruptedData(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	plaintext := []byte("payload to be corrupted")
	password := "SecretKey999"

	salt, nonce, ciphertext, err := provider.Encrypt(plaintext, password)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Corrupt ciphertext
	corrupted := make([]byte, len(ciphertext))
	copy(corrupted, ciphertext)
	corrupted[len(corrupted)-1] ^= 0x01

	_, err = provider.Decrypt(salt, nonce, corrupted, password)
	if err == nil {
		t.Fatalf("expected Decrypt to fail on corrupted ciphertext, got nil")
	}
	if !errors.Is(err, depmap.ErrAuthenticationFailed) {
		t.Fatalf("expected ErrAuthenticationFailed on corrupted ciphertext, got: %v", err)
	}
}

func TestAES256GCMProvider_InvalidLengths(t *testing.T) {
	provider := depmap.NewAES256GCMProvider()
	password := "ValidPassword123"

	// Invalid salt
	_, err := provider.Decrypt([]byte("short-salt"), make([]byte, depmap.NonceSize), make([]byte, 32), password)
	if err == nil || !errors.Is(err, depmap.ErrInvalidFormat) {
		t.Fatalf("expected ErrInvalidFormat for invalid salt length, got: %v", err)
	}

	// Invalid nonce
	_, err = provider.Decrypt(make([]byte, depmap.SaltSize), []byte("short-nonce"), make([]byte, 32), password)
	if err == nil || !errors.Is(err, depmap.ErrInvalidFormat) {
		t.Fatalf("expected ErrInvalidFormat for invalid nonce length, got: %v", err)
	}

	// Ciphertext too short
	_, err = provider.Decrypt(make([]byte, depmap.SaltSize), make([]byte, depmap.NonceSize), []byte("tiny"), password)
	if err == nil || !errors.Is(err, depmap.ErrInvalidFormat) {
		t.Fatalf("expected ErrInvalidFormat for too short ciphertext, got: %v", err)
	}
}

func TestGenerateRandomPassword_Validation(t *testing.T) {
	// Length <= 0 should return error
	_, err := depmap.GenerateRandomPassword(0)
	if err == nil {
		t.Fatalf("expected error for length 0")
	}

	_, err = depmap.GenerateRandomPassword(-5)
	if err == nil {
		t.Fatalf("expected error for negative length")
	}

	// Valid longer password
	pwd, err := depmap.GenerateRandomPassword(32)
	if err != nil {
		t.Fatalf("unexpected error for length 32: %v", err)
	}
	if len(pwd) != 32 {
		t.Fatalf("expected length 32, got %d", len(pwd))
	}
}

type mockEchoProvider struct{}

func (m *mockEchoProvider) Name() string { return "MOCK-ECHO" }
func (m *mockEchoProvider) Encrypt(plaintext []byte, password string) ([]byte, []byte, []byte, error) {
	if password == "" {
		return nil, nil, nil, depmap.ErrEmptyPassword
	}
	salt := make([]byte, depmap.SaltSize)
	nonce := make([]byte, depmap.NonceSize)
	// Append dummy 16-byte tag so length >= TagSize
	ciphertext := append(plaintext, make([]byte, depmap.TagSize)...)
	return salt, nonce, ciphertext, nil
}
func (m *mockEchoProvider) Decrypt(salt, nonce, ciphertext []byte, password string) ([]byte, error) {
	if len(ciphertext) < depmap.TagSize {
		return nil, depmap.ErrInvalidFormat
	}
	return ciphertext[:len(ciphertext)-depmap.TagSize], nil
}

func TestDepMap_CustomProvider(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := tmpDir + "/custom_provider.depmap"

	dm := depmap.New("custom.csv")
	dm.Put("syn", "orig")

	custom := &mockEchoProvider{}
	if custom.Name() != "MOCK-ECHO" {
		t.Fatalf("expected MOCK-ECHO")
	}

	err := dm.SaveFileWithProvider(filePath, "testpass", custom)
	if err != nil {
		t.Fatalf("SaveFileWithProvider failed: %v", err)
	}

	loaded, err := depmap.LoadFileWithProvider(filePath, "testpass", custom)
	if err != nil {
		t.Fatalf("LoadFileWithProvider failed: %v", err)
	}

	val, ok := loaded.Get("syn")
	if !ok || val != "orig" {
		t.Fatalf("expected 'orig', got '%s'", val)
	}
}

