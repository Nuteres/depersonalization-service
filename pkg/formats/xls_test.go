package formats

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
)

func getTestXLSPath(t *testing.T) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(TestXLSBase64)
	if err != nil {
		t.Fatalf("failed to decode base64 fixture: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "Table.xls")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("failed to write temp xls: %v", err)
	}
	return path
}

func TestXLSHandler_SupportedExtensions(t *testing.T) {
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)
	exts := h.SupportedExtensions()
	if len(exts) != 1 || exts[0] != ".xls" {
		t.Fatalf("unexpected supported extensions: %v", exts)
	}
}

func TestXLSHandler_Inspect(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	sheets, err := h.Inspect(path)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if len(sheets) != 1 {
		t.Fatalf("expected 1 sheet, got %d", len(sheets))
	}

	s := sheets[0]
	if s.Name != "Table" {
		t.Errorf("expected sheet name 'Table', got %q", s.Name)
	}
	if s.RowCount != 12 {
		t.Errorf("expected 12 rows, got %d", s.RowCount)
	}
	if len(s.Columns) != 3 || s.Columns[0] != "Code" || s.Columns[1] != "Name" || s.Columns[2] != "Description" {
		t.Errorf("unexpected columns: %v", s.Columns)
	}
}

func TestXLSHandler_Lifecycle(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"columns": map[string]string{
			"Name": "FIO",
		},
	}

	dm, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	if dm == nil || len(dm.Entities) == 0 {
		t.Fatalf("expected non-empty depmap, got %+v", dm)
	}

	// Verify that "name1" was recorded in depmap
	foundName1 := false
	for anonVal, origVal := range dm.Mapping {
		if origVal == "name1" {
			foundName1 = true
			if anonVal == origVal {
				t.Errorf("anonymized Name is identical to original")
			}
		}
	}
	if !foundName1 {
		t.Errorf("original value 'name1' not found in depmap")
	}

	// Verify output XLSX opens with excelize
	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX with excelize: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Table")
	if err != nil {
		t.Fatalf("failed to get rows from sheet 'Table': %v", err)
	}

	if len(outRows) != 12 {
		t.Fatalf("expected 12 rows in output XLSX, got %d", len(outRows))
	}

	// Header row intact
	if outRows[0][0] != "Code" || outRows[0][1] != "Name" || outRows[0][2] != "Description" {
		t.Errorf("unexpected headers: %v", outRows[0])
	}

	// Row 1 data
	if outRows[1][0] != "code1" {
		t.Errorf("Code column should be unaltered, got %q", outRows[1][0])
	}
	if outRows[1][1] == "name1" {
		t.Errorf("Name column was not anonymized: %q", outRows[1][1])
	}
	if outRows[1][2] != "description1" {
		t.Errorf("Description column should be unaltered, got %q", outRows[1][2])
	}
}

func TestXLSHandler_DisabledColumns(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"columns": map[string]string{
			"Name": "FIO",
		},
		"disabled_columns": []int{2}, // Disable column 2 (Name)
	}

	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Table")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	// Name should NOT be anonymized because column 2 is disabled
	if outRows[1][1] != "name1" {
		t.Errorf("expected 'name1' to be preserved when disabled, got %q", outRows[1][1])
	}
}

func TestXLSHandler_DisabledColumnsBySheet(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"columns": map[string]string{
			"Name": "FIO",
		},
		"disabled_columns_by_sheet": map[string][]int{
			"Table": {2},
		},
	}

	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Table")
	if err != nil {
		t.Fatalf("failed to get rows: %v", err)
	}

	if outRows[1][1] != "name1" {
		t.Errorf("expected 'name1' to be preserved when disabled by sheet, got %q", outRows[1][1])
	}
}

func TestXLSHandler_ProgressCallback(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	progressCalls := 0
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"progress_callback": func(n int) {
			progressCalls++
		},
	}

	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	if progressCalls == 0 {
		t.Errorf("expected progress_callback to be called at least once")
	}
}

func TestXLSHandler_ContextCancellation(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	outBuf := new(bytes.Buffer)
	_, err := h.ProcessToXLSX(ctx, path, outBuf, nil)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("expected context canceled error, got: %v", err)
	}
}

func TestXLSHandler_InvalidFile(t *testing.T) {
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	_, err := h.Inspect("non_existent_file.xls")
	if err == nil {
		t.Fatalf("expected error inspecting nonexistent file")
	}

	outBuf := new(bytes.Buffer)
	_, err = h.ProcessToXLSX(context.Background(), "non_existent_file.xls", outBuf, nil)
	if err == nil {
		t.Fatalf("expected error processing nonexistent file")
	}
}

func TestXLSHandler_MissingSheetError(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"sheet": "NonExistentSheet",
	}
	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected sheet not found error, got: %v", err)
	}
}

func TestXLSHandler_StrategySemanticTokens(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("test-salt")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	dm := depmap.New("Table.xls")
	opts := map[string]interface{}{
		"columns": map[string]string{
			"Name": "FIO",
		},
		"strategy": domain.StrategySemanticTokens,
		"salt":     "test-salt",
		"depmap":   dm,
	}

	resDm, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	if resDm.Mode != string(domain.StrategySemanticTokens) {
		t.Errorf("expected depmap mode to be %q, got %q", domain.StrategySemanticTokens, resDm.Mode)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Table")
	if err != nil {
		t.Fatalf("failed to get rows from sheet 'Table': %v", err)
	}

	if len(outRows) < 2 {
		t.Fatalf("expected at least 2 rows in output XLSX, got %d", len(outRows))
	}

	// First data row (row index 1), Name column (column index 1)
	expectedToken := "[ФИО_0001]"
	if outRows[1][1] != expectedToken {
		t.Errorf("expected semantic token %q in Name column, got %q", expectedToken, outRows[1][1])
	}
}

func TestXLSHandler_ShuffleRows(t *testing.T) {
	path := getTestXLSPath(t)
	anon := anonymizer.New("salt-for-shuffle")
	h := NewXLSHandler(anon)

	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{
		"shuffle_rows": true,
	}

	_, err := h.ProcessToXLSX(context.Background(), path, outBuf, opts)
	if err != nil {
		t.Fatalf("ProcessToXLSX failed: %v", err)
	}

	outXl, err := excelize.OpenReader(bytes.NewReader(outBuf.Bytes()))
	if err != nil {
		t.Fatalf("failed to open output XLSX: %v", err)
	}
	defer outXl.Close()

	outRows, err := outXl.GetRows("Table")
	if err != nil {
		t.Fatalf("failed to get rows from sheet 'Table': %v", err)
	}

	if len(outRows) != 12 {
		t.Fatalf("expected 12 rows (1 header + 11 data), got %d", len(outRows))
	}

	// Header row intact
	if outRows[0][0] != "Code" || outRows[0][1] != "Name" || outRows[0][2] != "Description" {
		t.Errorf("unexpected headers: %v", outRows[0])
	}

	// Verify data rows are shuffled
	shuffledCodes := make([]string, 11)
	for i := 1; i <= 11; i++ {
		shuffledCodes[i-1] = outRows[i][0]
	}
	originalCodes := []string{"code1", "code2", "code3", "code4", "code5", "code6", "code7", "code8", "code9", "code10", "code11"}

	isSameOrder := true
	for i := range originalCodes {
		if shuffledCodes[i] != originalCodes[i] {
			isSameOrder = false
			break
		}
	}
	if isSameOrder {
		t.Errorf("expected codes to be shuffled, but they are in sequential order: %v", shuffledCodes)
	}

	// Multiset check
	counts := make(map[string]int)
	for _, c := range shuffledCodes {
		counts[c]++
	}
	for _, c := range originalCodes {
		if counts[c] != 1 {
			t.Errorf("expected code %q to appear exactly once, got %d", c, counts[c])
		}
	}
}

