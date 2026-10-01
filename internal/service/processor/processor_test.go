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

	csvContent := `ФИО,СНИЛС,ИНН,Телефон,Город
Иванов Иван Иванович,112-233-445 95,500100732259,+7 999 123-45-67,Москва
Петрова Анна Сергеевна,112-233-445 95,500100732259,+7 916 234-56-78,Санкт-Петербург
Сидоров Сидор Сидорович,112-233-445 95,500100732259,+7 925 345-67-89,Казань
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
	reader := csv.NewReader(strings.NewReader(string(outBytes)))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse output csv: %v", err)
	}

	if len(rows) != 4 { // header + 3 data rows
		t.Fatalf("expected 4 rows in output, got %d", len(rows))
	}

	// Check header unchanged
	if rows[0][0] != "ФИО" || rows[0][4] != "Город" {
		t.Errorf("header altered: %v", rows[0])
	}
	// Check row 1 anonymized
	if rows[1][0] == "Иванов Иван Иванович" {
		t.Errorf("expected FIO to be anonymized, but was unchanged: %s", rows[1][0])
	}
	// Check unanonymized column
	if rows[1][4] != "Москва" {
		t.Errorf("expected City to be unchanged 'Москва', got: %s", rows[1][4])
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
		KeyPassword: "supersecretpass",
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

	keyInfo, err := km.InspectKey(keyPath, "supersecretpass")
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
