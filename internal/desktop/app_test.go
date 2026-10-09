package desktop_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	"dep_service/internal/version"
	"dep_service/pkg/audit"
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
	expectedVersion := "v" + version.Version
	if info["version"] != expectedVersion {
		t.Errorf("expected %s, got %s", expectedVersion, info["version"])
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
	procRes, err := app.ProcessFileTo(csvPath, outPath, "test-salt", nil, true, "secret123", keyPath, false)
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

	_, procErr = app.ProcessFileTo(csvPath, outPath, "test-salt", nil, false, "", "", false)
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
	if _, err := app.SaveAuditFileDialog("audit.jsonl"); err == nil {
		t.Errorf("expected error for SaveAuditFileDialog with nil ctx")
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

func TestDesktopApp_AuditIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.jsonl")
	al, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km, insp)
	rest := restorer.New(km)
	app := desktop.New(insp, prev, proc, rest, km, al)

	csvPath := filepath.Join(tmpDir, "input.csv")
	outPath := filepath.Join(tmpDir, "output.csv")
	keyPath := filepath.Join(tmpDir, "output.depmap")
	restPath := filepath.Join(tmpDir, "restored.csv")

	content := "ФИО,Телефон\nИванов Иван,+7 999 123-45-67\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write csv: %v", err)
	}

	// 1. Process
	procRes, err := app.ProcessFileTo(csvPath, outPath, "audit-salt", nil, true, "secret", keyPath, false)
	if err != nil {
		t.Fatalf("ProcessFileTo failed: %v", err)
	}
	if !procRes.Success {
		t.Fatalf("ProcessFileTo reported failure")
	}

	// Check GetAuditRecords
	records, err := app.GetAuditRecords(10)
	if err != nil {
		t.Fatalf("GetAuditRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Operation != "depersonalize" || records[0].Status != "success" {
		t.Errorf("unexpected record: %+v", records[0])
	}

	// 2. Restore
	restRes, err := app.RestoreFileTo(outPath, keyPath, "secret", restPath)
	if err != nil {
		t.Fatalf("RestoreFileTo failed: %v", err)
	}
	if !restRes.Success {
		t.Fatalf("RestoreFileTo reported failure")
	}

	// Check GetAuditRecords with limit 1 and limit 10
	records, err = app.GetAuditRecords(1)
	if err != nil {
		t.Fatalf("GetAuditRecords failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record for limit=1, got %d", len(records))
	}
	if records[0].Operation != "restore" {
		t.Errorf("expected newest record to be 'restore', got %s", records[0].Operation)
	}

	allRecords, err := app.GetAuditRecords(10)
	if err != nil {
		t.Fatalf("GetAuditRecords failed: %v", err)
	}
	if len(allRecords) != 2 {
		t.Fatalf("expected 2 records, got %d", len(allRecords))
	}

	// 3. Export
	exportPath := filepath.Join(tmpDir, "exported_audit.jsonl")
	if err := app.ExportAuditLog(exportPath); err != nil {
		t.Fatalf("ExportAuditLog failed: %v", err)
	}

	exportData, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read export: %v", err)
	}
	if !strings.Contains(string(exportData), "depersonalize") || !strings.Contains(string(exportData), "restore") {
		t.Errorf("exported audit log missing expected entries: %s", string(exportData))
	}
}

func TestDesktopApp_ProcessFileWithSheets_ShuffleRows(t *testing.T) {
	app := newTestApp()
	tmpDir := t.TempDir()

	inPath := filepath.Join(tmpDir, "app_shuffle.csv")
	outPath := filepath.Join(tmpDir, "app_shuffled.csv")

	var sb strings.Builder
	sb.WriteString("ID,Отдел\n")
	for i := 1; i <= 10; i++ {
		sb.WriteString(fmt.Sprintf("%d,Отдел_%d\n", i, i))
	}
	if err := os.WriteFile(inPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	res, err := app.ProcessFileWithSheets(inPath, outPath, "app-salt", nil, nil, false, "", "", true)
	if err != nil {
		t.Fatalf("ProcessFileWithSheets with shuffleRows failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected res.Success to be true")
	}

	outData, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed reading output: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(outData), "\ufeff")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed reading csv: %v", err)
	}

	if len(rows) != 11 {
		t.Fatalf("expected 11 rows, got %d", len(rows))
	}

	// Verify Header
	if rows[0][0] != "ID" || rows[0][1] != "Отдел" {
		t.Errorf("header changed: %v", rows[0])
	}

	// Verify rows are shuffled
	shuffledIDs := make([]string, 10)
	for i := 1; i <= 10; i++ {
		shuffledIDs[i-1] = rows[i][0]
	}
	sequential := true
	for i := 1; i <= 10; i++ {
		if shuffledIDs[i-1] != fmt.Sprintf("%d", i) {
			sequential = false
			break
		}
	}
	if sequential {
		t.Errorf("expected shuffled IDs, got sequential: %v", shuffledIDs)
	}
}

func TestDesktopApp_WailsArgumentTypes(t *testing.T) {
	appType := reflect.TypeOf(&desktop.DesktopApp{})

	// Test ProcessFileWithSheets: 9th arg must be bool for Wails JSON unmarshaling
	m, ok := appType.MethodByName("ProcessFileWithSheets")
	if !ok {
		t.Fatalf("ProcessFileWithSheets not found on DesktopApp")
	}
	if m.Type.NumIn() != 10 && m.Type.NumIn() != 11 {
		t.Fatalf("expected 10 or 11 inputs (including receiver), got %d", m.Type.NumIn())
	}
	lastParam := m.Type.In(9)
	if lastParam.Kind() != reflect.Bool {
		t.Errorf("expected 9th parameter to be bool for Wails RPC compatibility, got %v", lastParam)
	}

	// Verify json.Unmarshal into the parameter type succeeds when passed a boolean JSON value
	rawJSON := []byte("false")
	val := reflect.New(lastParam).Interface()
	if err := json.Unmarshal(rawJSON, val); err != nil {
		t.Errorf("failed to unmarshal JSON bool into method parameter type: %v", err)
	}

	// Test ProcessFileTo: 8th arg must be bool
	mTo, ok := appType.MethodByName("ProcessFileTo")
	if !ok {
		t.Fatalf("ProcessFileTo not found on DesktopApp")
	}
	if mTo.Type.NumIn() != 9 && mTo.Type.NumIn() != 10 {
		t.Fatalf("expected 9 or 10 inputs (including receiver), got %d", mTo.Type.NumIn())
	}
	lastParamTo := mTo.Type.In(8)
	if lastParamTo.Kind() != reflect.Bool {
		t.Errorf("expected last parameter of ProcessFileTo to be bool, got %v", lastParamTo)
	}

	// Test GetSheetRows: 7th arg must be string for mode
	mPrev, ok := appType.MethodByName("GetSheetRows")
	if !ok {
		t.Fatalf("GetSheetRows not found on DesktopApp")
	}
	if mPrev.Type.NumIn() != 8 {
		t.Fatalf("expected 8 inputs (including receiver), got %d", mPrev.Type.NumIn())
	}
	modeParam := mPrev.Type.In(7)
	if modeParam.Kind() != reflect.String {
		t.Errorf("expected 7th parameter of GetSheetRows to be string, got %v", modeParam)
	}
}

func TestDesktopApp_ModeDelete(t *testing.T) {
	app := newTestApp()
	tmpDir := t.TempDir()

	csvPath := filepath.Join(tmpDir, "test_app_delete.csv")
	outPath := filepath.Join(tmpDir, "test_app_delete_out.csv")
	content := "ID,ФИО,Телефон\n1,Иванов Иван Иванович,+7 999 123-45-67\n"
	if err := os.WriteFile(csvPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test csv: %v", err)
	}

	// 1. GetSheetRows with mode="delete"
	rows, err := app.GetSheetRows(csvPath, "", 0, 10, "test-salt", nil, "delete")
	if err != nil {
		t.Fatalf("GetSheetRows failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].After[1] != "[УДАЛЕНО]" {
		t.Errorf("expected After[1] to be [УДАЛЕНО], got %q", rows[0].After[1])
	}

	// 2. ProcessFileWithSheets with mode="delete"
	res, err := app.ProcessFileWithSheets(csvPath, outPath, "test-salt", nil, nil, false, "", "", false, "delete")
	if err != nil {
		t.Fatalf("ProcessFileWithSheets failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected Success=true")
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out file: %v", err)
	}
	if !strings.Contains(string(outBytes), "[УДАЛЕНО]") {
		t.Errorf("expected out file to contain '[УДАЛЕНО]', got %s", string(outBytes))
	}
}


