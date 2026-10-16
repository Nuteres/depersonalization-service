package ziputil_test

import (
	"os"
	"path/filepath"
	"testing"

	"dep_service/pkg/ziputil"
	"github.com/yeka/zip"
)

func TestCreatePasswordZip_SuccessAndExtract(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "report_anonymized.xlsx")
	content := []byte("dummy excel content for zip testing")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	zipFile := filepath.Join(tmpDir, "report_anonymized.zip")
	password := "Secret123"

	err := ziputil.CreatePasswordZip(srcFile, zipFile, password)
	if err != nil {
		t.Fatalf("CreatePasswordZip failed: %v", err)
	}

	r, err := zip.OpenReader(zipFile)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	if len(r.File) != 1 {
		t.Fatalf("expected 1 file in zip, got %d", len(r.File))
	}

	f := r.File[0]
	if f.Name != "report_anonymized.xlsx" {
		t.Errorf("expected entry name report_anonymized.xlsx, got %s", f.Name)
	}
	if !f.IsEncrypted() {
		t.Errorf("expected file to be encrypted")
	}

	f.SetPassword(password)
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("failed to open encrypted entry: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, len(content))
	n, _ := rc.Read(buf)
	if string(buf[:n]) != string(content) {
		t.Errorf("expected content %q, got %q", string(content), string(buf[:n]))
	}
}

func TestValidateArchivePassword(t *testing.T) {
	if err := ziputil.ValidateArchivePassword("abc"); err == nil {
		t.Errorf("expected error for short password")
	}
	if err := ziputil.ValidateArchivePassword("Пароль123"); err == nil {
		t.Errorf("expected error for cyrillic password")
	}
	if err := ziputil.ValidateArchivePassword("pass with space"); err == nil {
		t.Errorf("expected error for password with space")
	}
	if err := ziputil.ValidateArchivePassword("ValidPass123!"); err != nil {
		t.Errorf("expected valid password, got error: %v", err)
	}
}
