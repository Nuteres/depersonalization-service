package audit_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"dep_service/pkg/audit"
)

func TestAuditLogger_LogAndReadRecent(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "sub", "audit.jsonl")

	logger, err := audit.NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	rec1 := audit.AuditRecord{
		Operation:    "depersonalize",
		Mode:         "replace",
		SourceFile:   "source1.xlsx",
		SourceHash:   "hash1",
		ResultFile:   "res1.xlsx",
		ResultHash:   "reshash1",
		RecordsCount: 10,
		Entities:     []string{"FIO", "PHONE"},
		KeyCreated:   true,
		Status:       "success",
	}

	rec2 := audit.AuditRecord{
		Operation:    "restore",
		SourceFile:   "res1.xlsx",
		SourceHash:   "reshash1",
		ResultFile:   "restored1.xlsx",
		ResultHash:   "restoredhash1",
		RecordsCount: 10,
		Status:       "success",
	}

	rec3 := audit.AuditRecord{
		Operation:    "depersonalize",
		SourceFile:   "invalid.pdf",
		Status:       "error",
		Error:        "unsupported format",
	}

	if err := logger.Log(rec1); err != nil {
		t.Fatalf("failed to log rec1: %v", err)
	}
	if err := logger.Log(rec2); err != nil {
		t.Fatalf("failed to log rec2: %v", err)
	}
	if err := logger.Log(rec3); err != nil {
		t.Fatalf("failed to log rec3: %v", err)
	}

	// Read with limit = 2 (should return rec3 and rec2)
	recent, err := logger.ReadRecent(2)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recent))
	}
	if recent[0].Operation != "depersonalize" || recent[0].Status != "error" {
		t.Errorf("expected first recent record to be rec3, got %+v", recent[0])
	}
	if recent[1].Operation != "restore" || recent[1].Status != "success" {
		t.Errorf("expected second recent record to be rec2, got %+v", recent[1])
	}

	// Verify auto-populated fields
	if recent[0].Timestamp == "" {
		t.Errorf("expected non-empty timestamp")
	}
	if recent[0].Username == "" {
		t.Errorf("expected non-empty username")
	}
	if recent[0].Hostname == "" {
		t.Errorf("expected non-empty hostname")
	}

	// Read all records (limit = 0 or >= 3)
	all, err := logger.ReadRecent(0)
	if err != nil {
		t.Fatalf("ReadRecent(0) failed: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 records, got %d", len(all))
	}
	if all[0].Status != "error" || all[1].Operation != "restore" || all[2].SourceFile != "source1.xlsx" || all[2].Mode != "replace" {
		t.Errorf("unexpected ordering or content of records in all: %+v", all)
	}
}

func TestAuditLogger_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping file permission checks on Windows")
	}

	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, "secure_dir")
	logPath := filepath.Join(logDir, "audit.jsonl")

	logger, err := audit.NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	// Check directory permissions (0700)
	dirInfo, err := os.Stat(logDir)
	if err != nil {
		t.Fatalf("failed to stat log dir: %v", err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Errorf("expected dir mode 0700, got %o", dirInfo.Mode().Perm())
	}

	if err := logger.Log(audit.AuditRecord{Operation: "depersonalize", Status: "success"}); err != nil {
		t.Fatalf("failed to log: %v", err)
	}

	// Check file permissions (0600)
	fileInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("failed to stat log file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Errorf("expected file mode 0600, got %o", fileInfo.Mode().Perm())
	}
}

func TestComputeFileSHA256(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Known content
	filePath := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello 152-fz compliance audit trail\n")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	expectedHasher := sha256.New()
	expectedHasher.Write(content)
	expectedHash := hex.EncodeToString(expectedHasher.Sum(nil))

	actualHash, err := audit.ComputeFileSHA256(filePath)
	if err != nil {
		t.Fatalf("ComputeFileSHA256 failed: %v", err)
	}
	if actualHash != expectedHash {
		t.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	// 2. Empty file
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	emptyHash, err := audit.ComputeFileSHA256(emptyFile)
	if err != nil {
		t.Fatalf("ComputeFileSHA256 for empty file failed: %v", err)
	}
	expectedEmptyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if emptyHash != expectedEmptyHash {
		t.Errorf("expected empty file hash %s, got %s", expectedEmptyHash, emptyHash)
	}

	// 3. Non-existent file
	if _, err := audit.ComputeFileSHA256(filepath.Join(tmpDir, "missing.txt")); err == nil {
		t.Errorf("expected error for non-existent file")
	}

	// 4. Empty path string
	if _, err := audit.ComputeFileSHA256(""); err == nil {
		t.Errorf("expected error for empty path")
	}
}

func TestAuditLogger_ExportTo(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.jsonl")
	exportPath := filepath.Join(tmpDir, "export.jsonl")

	logger, err := audit.NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	_ = logger.Log(audit.AuditRecord{Operation: "depersonalize", SourceFile: "a.csv", Status: "success"})
	_ = logger.Log(audit.AuditRecord{Operation: "restore", SourceFile: "a_anon.csv", Status: "success"})

	if err := logger.ExportTo(exportPath); err != nil {
		t.Fatalf("ExportTo failed: %v", err)
	}

	origBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read orig log: %v", err)
	}
	exportedBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read exported log: %v", err)
	}

	if string(origBytes) != string(exportedBytes) {
		t.Errorf("exported content does not match original")
	}
}

func TestAuditLogger_ReadRecent_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "does_not_exist.jsonl")

	logger, err := audit.NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	records, err := logger.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed on non-existent file: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestAuditLogger_ThreadSafety(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "concurrent.jsonl")

	logger, err := audit.NewLogger(logPath)
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	const nGoroutines = 20
	const nPerGoroutine = 10
	var wg sync.WaitGroup
	wg.Add(nGoroutines)

	for i := 0; i < nGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < nPerGoroutine; j++ {
				_ = logger.Log(audit.AuditRecord{
					Operation:    "depersonalize",
					SourceFile:   "test.csv",
					RecordsCount: id*100 + j,
					Status:       "success",
				})
			}
		}(i)
	}

	wg.Wait()

	records, err := logger.ReadRecent(0)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != nGoroutines*nPerGoroutine {
		t.Errorf("expected %d records, got %d", nGoroutines*nPerGoroutine, len(records))
	}
}

func TestDefaultLogger(t *testing.T) {
	logger, err := audit.DefaultLogger()
	if err != nil {
		t.Fatalf("DefaultLogger failed: %v", err)
	}
	if logger == nil {
		t.Fatalf("expected non-nil logger")
	}
	expectedSuffix := filepath.Join(".depersonalizer", "audit.jsonl")
	if !filepath.IsAbs(logger.Path()) || !strings.HasSuffix(logger.Path(), expectedSuffix) {
		t.Errorf("expected path ending with %s, got %s", expectedSuffix, logger.Path())
	}
}

func TestDefaultAuditPath_Isolated(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir)
	}

	// 1. Fresh environment: returns ~/.depersonalizer/audit.jsonl
	path, err := audit.DefaultAuditPath()
	if err != nil {
		t.Fatalf("DefaultAuditPath failed: %v", err)
	}
	expected := filepath.Join(tmpDir, ".depersonalizer", "audit.jsonl")
	if path != expected {
		t.Errorf("expected %s, got %s", expected, path)
	}

	// 2. Legacy exists, default does not: migrates file to ~/.depersonalizer/audit.jsonl
	tmpDir2 := t.TempDir()
	t.Setenv("HOME", tmpDir2)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", tmpDir2)
	}
	legacyDir := filepath.Join(tmpDir2, ".dep_service")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatalf("failed to create legacy dir: %v", err)
	}
	legacyFile := filepath.Join(legacyDir, "audit.jsonl")
	testData := []byte(`{"operation":"depersonalize","status":"success"}` + "\n")
	if err := os.WriteFile(legacyFile, testData, 0600); err != nil {
		t.Fatalf("failed to write legacy file: %v", err)
	}

	migratedPath, err := audit.DefaultAuditPath()
	if err != nil {
		t.Fatalf("DefaultAuditPath with legacy failed: %v", err)
	}
	expectedMigrated := filepath.Join(tmpDir2, ".depersonalizer", "audit.jsonl")
	if migratedPath != expectedMigrated {
		t.Errorf("expected %s, got %s", expectedMigrated, migratedPath)
	}
	content, err := os.ReadFile(migratedPath)
	if err != nil {
		t.Fatalf("failed to read migrated file: %v", err)
	}
	if string(content) != string(testData) {
		t.Errorf("expected content %q, got %q", string(testData), string(content))
	}
}

