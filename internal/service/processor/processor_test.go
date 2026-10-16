package processor_test

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/processor"
	"dep_service/pkg/audit"
	"dep_service/pkg/depmap"
)

func TestProcessor_New(t *testing.T) {
	procDefault := processor.New(nil)
	if procDefault == nil {
		t.Fatalf("expected non-nil Processor from default New(nil)")
	}

	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)
	if proc == nil {
		t.Fatalf("expected non-nil Processor from New(km, insp)")
	}

	var _ *processor.ProcessorService = proc
}

func TestProcessorCancellation_Immediate(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input.csv")
	outPath := filepath.Join(tmpDir, "canceled_out.csv")

	if err := os.WriteFile(inPath, []byte("ФИО,Телефон\nИванов Иван,+79991234567\n"), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath,
		Salt:       "test-salt",
	}

	_, err := proc.Process(ctx, opts, func(p, total, pct int) {})
	if err == nil {
		t.Fatalf("expected error on cancelled context, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}

	// Verify output file does not exist
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Errorf("expected output file to not exist after cancellation, but stat returned: %v", err)
	}
}

func TestProcessorCancellation_DuringProgress(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_large.csv")
	outPath := filepath.Join(tmpDir, "canceled_during.csv")

	// Generate 200 rows of CSV data
	var sb strings.Builder
	sb.WriteString("ФИО,Телефон,Город\n")
	for i := 0; i < 200; i++ {
		sb.WriteString(fmt.Sprintf("Иванов Иван Иванович,%011d,Москва\n", 79991000000+int64(i)))
	}
	if err := os.WriteFile(inPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath,
		Salt:       "test-salt",
	}

	_, err := proc.Process(ctx, opts, func(p, total, pct int) {
		if p > 0 {
			cancel() // Cancel as soon as progress starts
		}
	})

	if err == nil {
		t.Fatalf("expected error on cancellation during processing, got nil")
	}

	// Windows safety check: output file should be closed and removed
	if _, statErr := os.Stat(outPath); !os.IsNotExist(statErr) {
		t.Errorf("expected output file to be removed after cancel, but file exists: %v", statErr)
	}
}

func TestProcessor_Process_CSV_ProgressReporting(t *testing.T) {
	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "sample.csv")
	outPath := filepath.Join(tmpDir, "sample_anonymized.csv")

	csvContent := `ФИО,СНИЛС,ИНН,Телефон,Статус
Иванов Иван Иванович,112-233-445 95,500100732259,+7 999 123-45-67,Активен
Петрова Анна Сергеевна,112-233-445 95,500100732259,+7 916 234-56-78,Активен
Сидоров Сидор Сидорович,112-233-445 95,500100732259,+7 925 345-67-89,Активен
`
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input csv: %v", err)
	}

	var progressReports []struct {
		proc  int
		total int
		pct   int
	}

	reporter := func(p, total, pct int) {
		progressReports = append(progressReports, struct {
			proc  int
			total int
			pct   int
		}{proc: p, total: total, pct: pct})
	}

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath,
		Salt:       "test-salt-csv",
	}

	res, err := proc.Process(context.Background(), opts, reporter)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected res.Success=true")
	}
	if res.RowsCount != 3 {
		t.Errorf("expected RowsCount=3, got %d", res.RowsCount)
	}
	if res.EntitiesCount == 0 {
		t.Errorf("expected EntitiesCount > 0, got %d", res.EntitiesCount)
	}
	if res.OutputPath != outPath {
		t.Errorf("expected OutputPath=%s, got %s", outPath, res.OutputPath)
	}
	if res.DurationMs < 0 {
		t.Errorf("expected DurationMs >= 0, got %d", res.DurationMs)
	}

	// Verify reporter was called and completed at 100%
	if len(progressReports) == 0 {
		t.Fatalf("expected progress reports, got 0")
	}
	lastReport := progressReports[len(progressReports)-1]
	if lastReport.pct != 100 {
		t.Errorf("expected final progress report pct=100, got %d", lastReport.pct)
	}

	// Read and verify anonymized content
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(outBytes), "\ufeff")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse output csv: %v", err)
	}

	if len(rows) != 4 { // header + 3 data rows
		t.Fatalf("expected 4 rows in output, got %d", len(rows))
	}

	// Check header unchanged
	if rows[0][0] != "ФИО" || rows[0][4] != "Статус" {
		t.Errorf("header altered: %v", rows[0])
	}
	// Check row 1 anonymized
	if rows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("expected FIO to be anonymized, but was unchanged: %s", rows[1][0])
	}
	// Check unanonymized column
	if rows[1][4] != "Активен" {
		t.Errorf("expected Status to be unchanged 'Активен', got: %s", rows[1][4])
	}
}

func TestProcessor_Process_KeyGeneration(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "keysample.csv")
	outPath := filepath.Join(tmpDir, "keysample_anonymized.csv")
	keyPath := filepath.Join(tmpDir, "keysample.depmap")

	csvContent := `ФИО,СНИЛС,Город
Смирнов Алексей Викторович,112-233-445 95,Самара
`
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		Salt:        "test-salt-key",
		CreateKey:   true,
		KeyPassword: "SuperSecretPass123!",
		KeyPath:     keyPath,
	}

	res, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("Process with CreateKey failed: %v", err)
	}

	if res.KeyPath != keyPath {
		t.Errorf("expected res.KeyPath=%s, got %s", keyPath, res.KeyPath)
	}

	// Verify key file exists and is valid
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("expected key file to exist: %v", err)
	}

	keyInfo, err := km.InspectKey(keyPath, "SuperSecretPass123!")
	if err != nil {
		t.Fatalf("InspectKey failed: %v", err)
	}
	if !keyInfo.Valid {
		t.Errorf("expected key file to be valid")
	}
	if keyInfo.RecordsCount <= 0 {
		t.Errorf("expected key records count > 0, got %d", keyInfo.RecordsCount)
	}
}

func TestProcessor_SameInputOutputPathRejection(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "safe.csv")
	origContent := "ФИО,Телефон\nИванов Иван,+79991234567\n"

	if err := os.WriteFile(inPath, []byte(origContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: inPath, // Identical path
		Salt:       "test-salt",
	}

	_, err := proc.Process(context.Background(), opts, nil)
	if err == nil {
		t.Fatalf("expected error when InputPath == OutputPath, got nil")
	}

	// Verify input file was NOT truncated or lost
	contentAfter, err := os.ReadFile(inPath)
	if err != nil {
		t.Fatalf("failed to read file after error: %v", err)
	}
	if string(contentAfter) != origContent {
		t.Errorf("input file was altered or truncated! Expected %q, got %q", origContent, string(contentAfter))
	}
}

func TestProcessor_Process_XLSX_MultiSheet(t *testing.T) {
	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "multisheet.xlsx")
	outPath := filepath.Join(tmpDir, "multisheet_anonymized.xlsx")

	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Отдел 1"
	sheet2 := "Отдел 2"
	_ = f.SetSheetName("Sheet1", sheet1)
	_, _ = f.NewSheet(sheet2)

	// Sheet 1 data
	_ = f.SetCellValue(sheet1, "A1", "ФИО")
	_ = f.SetCellValue(sheet1, "B1", "Телефон")
	_ = f.SetCellValue(sheet1, "A2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet1, "B2", "+7 999 111-22-33")

	// Sheet 2 data
	_ = f.SetCellValue(sheet2, "A1", "ФИО")
	_ = f.SetCellValue(sheet2, "B1", "СНИЛС")
	_ = f.SetCellValue(sheet2, "A2", "Петрова Анна Сергеевна")
	_ = f.SetCellValue(sheet2, "B2", "112-233-445 95")

	if err := f.SaveAs(inPath); err != nil {
		t.Fatalf("failed to save initial xlsx: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath,
		Salt:       "salt-multi",
	}

	res, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("Process XLSX multi-sheet failed: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success")
	}
	if res.RowsCount != 2 {
		t.Errorf("expected 2 total rows, got %d", res.RowsCount)
	}

	// Open output workbook and check both sheets
	outF, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatalf("failed to open output xlsx: %v", err)
	}
	defer outF.Close()

	sheets := outF.GetSheetList()
	if len(sheets) < 2 {
		t.Fatalf("expected at least 2 sheets in output, got %v", sheets)
	}

	val1, _ := outF.GetCellValue(sheet1, "A2")
	if val1 == "Иванов Иван Иванович" || val1 == "" {
		t.Errorf("expected sheet1 A2 to be anonymized, got %q", val1)
	}

	val2, _ := outF.GetCellValue(sheet2, "A2")
	if val2 == "Петрова Анна Сергеевна" || val2 == "" {
		t.Errorf("expected sheet2 A2 to be anonymized, got %q", val2)
	}
}

func TestProcessor_UnsupportedFormat(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "file.unsupported_ext")
	if err := os.WriteFile(inPath, []byte("data"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: filepath.Join(tmpDir, "out.bin"),
	}

	_, err := proc.Process(context.Background(), opts, nil)
	if err == nil {
		t.Fatalf("expected error for unsupported format, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected domain.ErrUnsupportedFormat, got: %v", err)
	}
}

func TestProcessor_DisabledColumns(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "disabled_cols.csv")
	outPath := filepath.Join(tmpDir, "disabled_cols_out.csv")

	// Col 1: ФИО, Col 2: Телефон
	csvContent := "ФИО,Телефон\nИванов Иван Иванович,+7 999 123-45-67\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Disable column 1 (ФИО)
	opts := domain.ProcessOptions{
		InputPath:    inPath,
		OutputPath:   outPath,
		Salt:         "test-salt",
		DisabledCols: []int{1},
	}

	_, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out file: %v", err)
	}

	reader := csv.NewReader(strings.NewReader(string(outBytes)))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse output csv: %v", err)
	}

	if rows[1][0] != "Иванов Иван Иванович" {
		t.Errorf("expected disabled col 1 to remain unchanged, got: %s", rows[1][0])
	}
	if rows[1][1] == "+7 999 123-45-67" {
		t.Errorf("expected enabled col 2 (Телефон) to be anonymized, got: %s", rows[1][1])
	}
}

func TestProcessor_EmptyInputPath(t *testing.T) {
	proc := processor.New(nil)
	_, err := proc.Process(context.Background(), domain.ProcessOptions{}, nil)
	if err == nil {
		t.Fatalf("expected error for empty input path, got nil")
	}
}

func TestProcessor_DirectoryInputPath(t *testing.T) {
	proc := processor.New(nil)
	tmpDir := t.TempDir()
	_, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath: tmpDir,
	}, nil)
	if err == nil {
		t.Fatalf("expected error for directory input path, got nil")
	}
	if !errors.Is(err, domain.ErrUnsupportedFormat) {
		t.Errorf("expected domain.ErrUnsupportedFormat, got: %v", err)
	}
}

func TestProcessor_NonexistentInputFile(t *testing.T) {
	proc := processor.New(nil)
	_, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath: "/nonexistent/path/file.csv",
	}, nil)
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
}

func TestProcessor_DefaultOutputPath(t *testing.T) {
	proc := processor.New(nil)
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "autoout.csv")
	expectedOut := filepath.Join(tmpDir, "autoout_anonymized.csv")

	if err := os.WriteFile(inPath, []byte("ФИО,Телефон\nИванов Иван,+79991234567\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	res, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath: inPath,
		// OutputPath left empty
	}, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if res.OutputPath != expectedOut {
		t.Errorf("expected OutputPath=%s, got %s", expectedOut, res.OutputPath)
	}
	if _, err := os.Stat(expectedOut); err != nil {
		t.Errorf("expected output file to exist: %v", err)
	}
}

func TestProcessor_GeneratesRandomSaltWhenEmpty(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input.csv")
	outPath1 := filepath.Join(tmpDir, "out1.csv")
	outPath2 := filepath.Join(tmpDir, "out2.csv")
	outPathStatic := filepath.Join(tmpDir, "out_static.csv")

	csvContent := "ФИО,Телефон\nИванов Иван Иванович,+7 999 123-45-67\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	// Run 1 with empty salt
	res1, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath1,
		Salt:       "",
	}, nil)
	if err != nil {
		t.Fatalf("run 1 failed: %v", err)
	}
	if !res1.Success {
		t.Fatalf("expected run 1 success")
	}

	// Run 2 with empty salt
	res2, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath2,
		Salt:       "",
	}, nil)
	if err != nil {
		t.Fatalf("run 2 failed: %v", err)
	}
	if !res2.Success {
		t.Fatalf("expected run 2 success")
	}

	// Run static with explicit legacy static salt
	_, err = proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPathStatic,
		Salt:       "152fz-secure-salt",
	}, nil)
	if err != nil {
		t.Fatalf("run static failed: %v", err)
	}

	content1, err := os.ReadFile(outPath1)
	if err != nil {
		t.Fatalf("failed to read out1: %v", err)
	}
	content2, err := os.ReadFile(outPath2)
	if err != nil {
		t.Fatalf("failed to read out2: %v", err)
	}
	contentStatic, err := os.ReadFile(outPathStatic)
	if err != nil {
		t.Fatalf("failed to read out_static: %v", err)
	}

	// Two consecutive runs with empty salt must generate different salts, yielding different outputs
	if string(content1) == string(content2) {
		t.Errorf("expected different outputs from successive runs with empty salt, but got identical content: %s", string(content1))
	}

	// Neither run with empty salt should match the static salt output
	if string(content1) == string(contentStatic) {
		t.Errorf("expected run with empty salt to not use static '152fz-secure-salt'")
	}
	if string(content2) == string(contentStatic) {
		t.Errorf("expected run with empty salt to not use static '152fz-secure-salt'")
	}
}

func TestProcessor_AutoGeneratedKeyPassword(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_key.csv")
	outPath := filepath.Join(tmpDir, "out_key.csv")
	keyPath := filepath.Join(tmpDir, "out_key.depmap")

	csvContent := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	res, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		CreateKey:   true,
		KeyPassword: "",
		KeyPath:     keyPath,
	}, nil)
	if err != nil {
		t.Fatalf("Process with auto key password failed: %v", err)
	}

	if !res.KeyPasswordGenerated {
		t.Errorf("expected res.KeyPasswordGenerated=true, got false")
	}
	if res.EffectivePassword == "" {
		t.Errorf("expected non-empty res.EffectivePassword")
	}
	if len(res.EffectivePassword) < 16 {
		t.Errorf("expected effective password length >= 16, got %d (%s)", len(res.EffectivePassword), res.EffectivePassword)
	}

	info, err := km.InspectKey(keyPath, res.EffectivePassword)
	if err != nil {
		t.Fatalf("failed to inspect key with effective password: %v", err)
	}
	if !info.Valid {
		t.Errorf("expected key to be valid when opened with EffectivePassword")
	}

	_, errWrong := km.InspectKey(keyPath, "WrongPassword!999")
	if errWrong == nil {
		t.Errorf("expected InspectKey to fail with wrong password, but it succeeded")
	}
}

func TestProcessor_ExplicitKeyPassword(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_explicit.csv")
	outPath := filepath.Join(tmpDir, "out_explicit.csv")
	keyPath := filepath.Join(tmpDir, "out_explicit.depmap")

	csvContent := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	res, err := proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		CreateKey:   true,
		KeyPassword: "MyCustomPassword456!",
		KeyPath:     keyPath,
	}, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if res.KeyPasswordGenerated {
		t.Errorf("expected res.KeyPasswordGenerated=false for explicit password")
	}
	if res.EffectivePassword != "MyCustomPassword456!" {
		t.Errorf("expected res.EffectivePassword='MyCustomPassword456!', got %q", res.EffectivePassword)
	}

	info, err := km.InspectKey(keyPath, "MyCustomPassword456!")
	if err != nil || !info.Valid {
		t.Fatalf("failed to inspect key with explicit password: %v", err)
	}
}

func TestProcessor_AuditLogging(t *testing.T) {
	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.jsonl")
	al, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)
	proc.SetAuditLogger(al)

	inPath := filepath.Join(tmpDir, "test_audit.csv")
	outPath := filepath.Join(tmpDir, "test_audit_out.csv")
	keyPath := filepath.Join(tmpDir, "test_audit.depmap")

	csvContent := "ФИО,СНИЛС\nИванов Иван Иванович,112-233-445 95\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	// 1. Success case
	_, err = proc.Process(context.Background(), domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: outPath,
		Salt:       "audit-salt",
		CreateKey:  true,
		KeyPath:    keyPath,
	}, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	records, err := al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	r0 := records[0]
	if r0.Operation != "depersonalize" {
		t.Errorf("expected operation 'depersonalize', got %q", r0.Operation)
	}
	if r0.Status != "success" {
		t.Errorf("expected status 'success', got %q", r0.Status)
	}
	if !r0.KeyCreated {
		t.Errorf("expected KeyCreated=true")
	}
	if r0.SourceHash == "" || r0.ResultHash == "" {
		t.Errorf("expected non-empty source and result hashes, got src=%q, res=%q", r0.SourceHash, r0.ResultHash)
	}
	if r0.RecordsCount != 1 {
		t.Errorf("expected RecordsCount=1, got %d", r0.RecordsCount)
	}

	// 2. Error case
	_, _ = proc.Process(context.Background(), domain.ProcessOptions{
		InputPath: filepath.Join(tmpDir, "non_existent.csv"),
	}, nil)

	records, err = al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 audit records, got %d", len(records))
	}
	if records[0].Status != "error" || records[0].Error == "" {
		t.Errorf("expected newest record to be error, got %+v", records[0])
	}

	// 3. Cancel case
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = proc.Process(ctx, domain.ProcessOptions{
		InputPath: inPath,
	}, nil)

	records, err = al.ReadRecent(10)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 audit records, got %d", len(records))
	}
	if records[0].Status != "canceled" {
		t.Errorf("expected newest record to be canceled, got %+v", records[0])
	}
}

func TestProcessor_ShuffleRows(t *testing.T) {
	proc := processor.New(nil)
	tmpDir := t.TempDir()

	inPath := filepath.Join(tmpDir, "input_shuffle.csv")
	outPath := filepath.Join(tmpDir, "output_shuffled.csv")

	var sb strings.Builder
	sb.WriteString("ID,Отдел,Примечание\n")
	for i := 1; i <= 10; i++ {
		sb.WriteString(fmt.Sprintf("%d,Отдел_%d,Заметка_%d\n", i, i, i))
	}
	if err := os.WriteFile(inPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		Salt:        "proc-shuffle-salt-1",
		ShuffleRows: true,
	}

	res, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("proc.Process failed: %v", err)
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
		t.Fatalf("failed reading output csv: %v", err)
	}

	if len(rows) != 11 {
		t.Fatalf("expected 11 rows, got %d", len(rows))
	}

	// Verify Header is preserved at row 0
	if rows[0][0] != "ID" || rows[0][1] != "Отдел" || rows[0][2] != "Примечание" {
		t.Errorf("header row mismatch: %v", rows[0])
	}

	// Verify data rows are shuffled
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
		t.Errorf("expected rows to be shuffled, but IDs remained in sequential order: %v", shuffledIDs)
	}
}

func TestProcessor_ModeDelete(t *testing.T) {
	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.jsonl")
	al, err := audit.NewLogger(auditPath)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	proc.SetAuditLogger(al)

	inPath := filepath.Join(tmpDir, "input.csv")
	outPath := filepath.Join(tmpDir, "output.csv")
	keyPath := filepath.Join(tmpDir, "key.depmap")

	csvContent := "ID,ФИО,Телефон\n1,Иванов Иван Иванович,+7 999 123-45-67\n2,Петров Петр Петрович,+7 999 765-43-21\n"
	if err := os.WriteFile(inPath, []byte(csvContent), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		Mode:        domain.ModeDelete,
		CreateKey:   true,
		KeyPassword: "Password123Secret!",
		KeyPath:     keyPath,
	}

	res, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected res.Success=true")
	}

	// 1. Verify output file contains [УДАЛЕНО]
	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	outStr := string(outBytes)
	if !strings.Contains(outStr, "[УДАЛЕНО]") {
		t.Errorf("expected output to contain '[УДАЛЕНО]', got: %s", outStr)
	}
	if strings.Contains(outStr, "Иванов Иван Иванович") || strings.Contains(outStr, "+7 999 123-45-67") {
		t.Errorf("output still contains sensitive data: %s", outStr)
	}

	// 2. Verify audit record Mode is "delete"
	records, err := al.ReadRecent(1)
	if err != nil {
		t.Fatalf("ReadRecent failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(records))
	}
	if records[0].Mode != "delete" {
		t.Errorf("expected audit record Mode to be 'delete', got %q", records[0].Mode)
	}

	// 3. Verify key file has ModeDelete and CellMappings
	dm, err := depmap.LoadFile(keyPath, "Password123Secret!")
	if err != nil {
		t.Fatalf("failed to load key file: %v", err)
	}
	if dm.Mode != string(domain.ModeDelete) {
		t.Errorf("expected dm.Mode to be 'delete', got %q", dm.Mode)
	}
	if len(dm.CellMappings) == 0 {
		t.Errorf("expected dm.CellMappings to be populated in ModeDelete")
	}
	if len(dm.TargetHeaders) == 0 {
		t.Errorf("expected dm.TargetHeaders to be populated")
	}
}

func TestProcessor_ShuffleRows_IgnoredForTextFormats(t *testing.T) {
	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "sample.txt")
	outPath := filepath.Join(tmpDir, "sample_out.txt")

	content := "Договор с Ивановым Иваном Ивановичем.\nПаспорт 4510 123456.\nДата: 12.05.2024."
	if err := os.WriteFile(inPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:   inPath,
		OutputPath:  outPath,
		Salt:        "test-salt",
		ShuffleRows: true, // Should be safely ignored for text files
		Mode:        domain.ModeReplace,
		Strategy:    domain.StrategySemanticTokens,
	}

	res, err := proc.Process(context.Background(), opts, func(p, total, pct int) {})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got false: %s", res.Error)
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read out file: %v", err)
	}
	outText := string(outBytes)

	// Verify tokens replaced and structure maintained
	if !strings.Contains(outText, "[ФИО_") {
		t.Errorf("expected [ФИО_ token in output, got: %s", outText)
	}
}

func TestProcessor_CreateArchive(t *testing.T) {
	km := keymanager.New()
	insp := inspector.New()
	proc := processor.New(km, insp)

	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "records.csv")
	outPath := filepath.Join(tmpDir, "records_anonymized.csv")

	content := "ФИО,Телефон\nИванов Иван,+79991234567\nПетров Петр,+79997654321\n"
	if err := os.WriteFile(inPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write input file: %v", err)
	}

	opts := domain.ProcessOptions{
		InputPath:       inPath,
		OutputPath:      outPath,
		Salt:            "test-salt",
		Mode:            domain.ModeReplace,
		Strategy:        domain.StrategySemanticTokens,
		CreateArchive:   true,
		ArchivePassword: "ArchivePass123",
	}

	res, err := proc.Process(context.Background(), opts, func(p, total, pct int) {})
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected success, got false: %s", res.Error)
	}

	// 1. Both files must exist on disk
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected uncompressed output file to exist: %v", err)
	}
	expectedZipPath := filepath.Join(tmpDir, "records_anonymized.zip")
	if _, err := os.Stat(expectedZipPath); err != nil {
		t.Errorf("expected zip archive to exist: %v", err)
	}

	// 2. Result fields must be populated
	if res.ArchivePath != expectedZipPath {
		t.Errorf("expected ArchivePath %q, got %q", expectedZipPath, res.ArchivePath)
	}
	if res.ArchivePassword != "ArchivePass123" {
		t.Errorf("expected ArchivePassword ArchivePass123, got %q", res.ArchivePassword)
	}
	if res.ArchivePasswordGenerated {
		t.Errorf("expected ArchivePasswordGenerated to be false for manual password")
	}

	// 3. Test auto-generation when password is empty
	outPath2 := filepath.Join(tmpDir, "records2_anonymized.csv")
	optsAuto := domain.ProcessOptions{
		InputPath:     inPath,
		OutputPath:    outPath2,
		Salt:          "test-salt",
		Mode:          domain.ModeReplace,
		Strategy:      domain.StrategySemanticTokens,
		CreateArchive: true,
	}
	resAuto, err := proc.Process(context.Background(), optsAuto, func(p, total, pct int) {})
	if err != nil {
		t.Fatalf("Process with auto password failed: %v", err)
	}
	if len(resAuto.ArchivePassword) != 16 {
		t.Errorf("expected 16-char auto generated password, got %q", resAuto.ArchivePassword)
	}
	if !resAuto.ArchivePasswordGenerated {
		t.Errorf("expected ArchivePasswordGenerated to be true")
	}
	if _, err := os.Stat(resAuto.ArchivePath); err != nil {
		t.Errorf("expected auto archive to exist: %v", err)
	}
}

func TestProcessor_ValidTargetExtensions(t *testing.T) {
	tests := []struct {
		ext      string
		expected []string
	}{
		{".csv", []string{".csv", ".tsv"}},
		{".tsv", []string{".csv", ".tsv"}},
		{".xlsx", []string{".xlsx", ".xlsm", ".xltx", ".xltm"}},
		{".ods", []string{".ods", ".xlsx"}},
		{".xlsb", []string{".xlsx"}},
		{".xls", []string{".xlsx"}},
		{".docx", []string{".docx"}},
		{".odt", []string{".odt"}},
		{".txt", []string{".txt"}},
		{".pdf", []string{".pdf"}},
		{".unknown", nil},
	}

	for _, tt := range tests {
		got := processor.ValidTargetExtensions(tt.ext)
		if len(got) != len(tt.expected) {
			t.Errorf("ValidTargetExtensions(%q): got %v, want %v", tt.ext, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("ValidTargetExtensions(%q)[%d]: got %q, want %q", tt.ext, i, got[i], tt.expected[i])
			}
		}
	}
}

func TestProcessor_EnforceTargetExtension_XLSX_Not_CSV(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)
	tmpDir := t.TempDir()

	// 1. Create a minimal XLSX file
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ФИО")
	_ = f.SetCellValue(sheet, "A2", "Иванов Иван Иванович")
	inPath := filepath.Join(tmpDir, "test.xlsx")
	if err := f.SaveAs(inPath); err != nil {
		t.Fatalf("failed to save test.xlsx: %v", err)
	}

	// 2. Specify OutputPath ending in .csv (which was the bug causing fake CSV)
	badOutPath := filepath.Join(tmpDir, "output_anonymized.csv")
	opts := domain.ProcessOptions{
		InputPath:  inPath,
		OutputPath: badOutPath,
		Salt:       "test-salt-123",
		Mode:       domain.ModeReplace,
		Strategy:   domain.StrategySemanticTokens,
		CreateKey:  true,
	}

	res, err := proc.Process(context.Background(), opts, nil)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// 3. OutputPath must have been corrected to .xlsx
	if !strings.HasSuffix(res.OutputPath, ".xlsx") {
		t.Errorf("expected OutputPath to end in .xlsx, got %q", res.OutputPath)
	}

	// 4. Must be openable by excelize without error
	outF, err := excelize.OpenFile(res.OutputPath)
	if err != nil {
		t.Fatalf("failed to open output as excel: %v", err)
	}
	defer outF.Close()

	// 5. Verify the cell was anonymized
	val, err := outF.GetCellValue(sheet, "A2")
	if err != nil {
		t.Fatalf("failed to get cell value: %v", err)
	}
	if val == "Иванов Иван Иванович" || val == "" {
		t.Errorf("expected anonymized value in A2, got %q", val)
	}
}

