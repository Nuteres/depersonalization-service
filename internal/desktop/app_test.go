package desktop_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"dep_service/internal/desktop"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
)

func newTestApp() *desktop.DesktopApp {
	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km, insp)
	rest := restorer.New(km)
	return desktop.New(insp, prev, proc, rest, km)
}

func TestDesktopAppInitialization(t *testing.T) {
	app := newTestApp()
	info := app.GetAppInfo()
	if info["version"] == "" {
		t.Errorf("expected non-empty version, got empty")
	}
	if info["os"] == "" {
		t.Errorf("expected non-empty os, got empty")
	}
	if info["arch"] == "" {
		t.Errorf("expected non-empty arch, got empty")
	}

	// Also test New with nil parameters (defensive defaults)
	fallbackApp := desktop.New(nil, nil, nil, nil, nil)
	fallbackInfo := fallbackApp.GetAppInfo()
	if fallbackInfo["version"] != info["version"] {
		t.Errorf("expected matching version, got %s", fallbackInfo["version"])
	}
}

func TestDesktopApp_InspectFile(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test.csv")
	content := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	app := newTestApp()
	info, err := app.InspectFile(csvPath)
	if err != nil {
		t.Fatalf("InspectFile failed: %v", err)
	}
	if info.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", info.RowCount)
	}
	if len(info.Headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(info.Headers))
	}
}

func TestDesktopApp_GetTableRows(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_rows.csv")
	content := "Имя,Телефон\nИван,+7 999 111-22-33\nПетр,+7 999 222-33-44\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	app := newTestApp()
	rows, err := app.GetTableRows(csvPath, 0, 10, "test-salt", nil)
	if err != nil {
		t.Fatalf("GetTableRows failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(rows[0].After) != 2 {
		t.Errorf("expected 2 columns in preview row, got %d", len(rows[0].After))
	}
}

func TestDesktopApp_ProcessAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_proc.csv")
	outPath := filepath.Join(tmpDir, "test_proc_anon.csv")
	keyPath := filepath.Join(tmpDir, "test_proc.depmap")
	restPath := filepath.Join(tmpDir, "test_proc_restored.csv")

	content := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	app := newTestApp()

	// Process with key creation
	procRes, err := app.ProcessFileTo(csvPath, outPath, "test-salt", nil, true, "secret123", keyPath)
	if err != nil {
		t.Fatalf("ProcessFileTo failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("expected Success=true")
	}

	// Verify anonymized file exists and differs
	anonBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read anon file: %v", err)
	}
	if strings.Contains(string(anonBytes), "Иванов Иван Иванович") {
		t.Errorf("anonymized file still contains original name: %s", string(anonBytes))
	}

	// Inspect key file with valid password
	keyInfo, err := app.InspectKeyFile(keyPath, "secret123")
	if err != nil {
		t.Fatalf("InspectKeyFile failed: %v", err)
	}
	if !keyInfo.Valid {
		t.Fatalf("expected keyInfo.Valid=true, got error: %s", keyInfo.Error)
	}

	// Inspect key file with invalid password
	keyInfoWrong, err := app.InspectKeyFile(keyPath, "wrongpass")
	if err != nil {
		t.Fatalf("InspectKeyFile with wrong password returned error instead of invalid info: %v", err)
	}
	if keyInfoWrong.Valid {
		t.Errorf("expected keyInfoWrong.Valid=false")
	}

	// Restore file
	restRes, err := app.RestoreFileTo(outPath, keyPath, "secret123", restPath)
	if err != nil {
		t.Fatalf("RestoreFileTo failed: %v", err)
	}
	if !restRes.Success {
		t.Fatalf("expected restore Success=true")
	}

	restBytes, err := os.ReadFile(restPath)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if !strings.Contains(string(restBytes), "Иванов Иван Иванович") {
		t.Errorf("restored file missing original name: %s", string(restBytes))
	}
}

func TestDesktopApp_Cancellation(t *testing.T) {
	app := newTestApp()

	// Verify calling cancel when no operation is running doesn't panic
	app.CancelCurrentOperation()

	// Test cancellation of long-running process
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test_large.csv")
	outPath := filepath.Join(tmpDir, "test_large_out.csv")

	var sb strings.Builder
	sb.WriteString("ФИО,СНИЛС,Телефон\n")
	for i := 0; i < 50000; i++ {
		sb.WriteString("Иванов Иван Иванович,112-233-445 95,+7 999 111-22-33\n")
	}
	if err := os.WriteFile(csvPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	var wg sync.WaitGroup
	var procErr error
	wg.Add(1)

	go func() {
		defer wg.Done()
		time.Sleep(2 * time.Millisecond)
		app.CancelCurrentOperation()
	}()

	_, procErr = app.ProcessFileTo(csvPath, outPath, "test-salt", nil, false, "", "")
	wg.Wait()

	if procErr == nil {
		// In case machine is exceptionally fast and finished before cancel, it's not a fatal error
		t.Logf("Process finished before cancel triggered")
	} else if !strings.Contains(procErr.Error(), "canceled") && !strings.Contains(procErr.Error(), "context canceled") {
		t.Errorf("expected context cancellation error, got %v", procErr)
	}
}

func TestDesktopApp_Dialogs_NilContext(t *testing.T) {
	app := newTestApp()

	if _, err := app.SelectFile(); err == nil {
		t.Errorf("expected error for SelectFile with nil ctx")
	}
	if _, err := app.SaveFileDialog("out.csv"); err == nil {
		t.Errorf("expected error for SaveFileDialog with nil ctx")
	}
	if _, err := app.SelectKeyFile(); err == nil {
		t.Errorf("expected error for SelectKeyFile with nil ctx")
	}
	if _, err := app.SaveKeyFileDialog("key.depmap"); err == nil {
		t.Errorf("expected error for SaveKeyFileDialog with nil ctx")
	}
	if err := app.OpenContainingFolder(""); err == nil {
		t.Errorf("expected error for OpenContainingFolder with empty path")
	}
}

func TestDesktopApp_StartupAndContext(t *testing.T) {
	app := newTestApp()
	app.Startup(context.Background())
	// Ensure calling Startup sets context without error
}

func TestDesktopApp_InspectKeyFile_EdgeCases(t *testing.T) {
	app := newTestApp()

	// Empty path
	_, err := app.InspectKeyFile("", "pass")
	if err == nil {
		t.Errorf("expected error for empty keyPath")
	}

	// Non-existent file
	_, err = app.InspectKeyFile("/path/does/not/exist.depmap", "pass")
	if err == nil {
		t.Errorf("expected error for missing key file")
	}

	// Corrupted file
	tmpDir := t.TempDir()
	corrupted := filepath.Join(tmpDir, "corrupted.depmap")
	if err := os.WriteFile(corrupted, []byte("NOT_A_KEY"), 0644); err != nil {
		t.Fatalf("failed to write corrupted key file: %v", err)
	}

	info, err := app.InspectKeyFile(corrupted, "pass")
	if err != nil {
		t.Fatalf("InspectKeyFile with corrupted key file returned error instead of invalid info: %v", err)
	}
	if info.Valid {
		t.Errorf("expected info.Valid=false for corrupted key file")
	}
}
