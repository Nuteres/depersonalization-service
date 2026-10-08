package formats_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/formats"
)

func TestShuffleRows_CSV(t *testing.T) {
	ctx := context.Background()

	// 10 data rows with distinct IDs
	var sb strings.Builder
	sb.WriteString("ID,Отдел,Комментарий\n")
	for i := 1; i <= 10; i++ {
		sb.WriteString(fmt.Sprintf("%d,Отдел_%d,Заметка_%d\n", i, i, i))
	}
	inputData := sb.String()

	anon1 := anonymizer.New("salt-alpha-152fz")
	handler1 := formats.NewCSVHandler(anon1)

	// 1. Process with shuffle_rows = true
	var outBuf1 bytes.Buffer
	optsShuffle := map[string]interface{}{
		"shuffle_rows": true,
	}
	if err := handler1.Process(ctx, strings.NewReader(inputData), &outBuf1, optsShuffle); err != nil {
		t.Fatalf("Process with shuffle failed: %v", err)
	}

	reader1 := csv.NewReader(&outBuf1)
	rows1, err := reader1.ReadAll()
	if err != nil {
		t.Fatalf("failed reading shuffled CSV output: %v", err)
	}

	if len(rows1) != 11 {
		t.Fatalf("expected 11 rows (1 header + 10 data), got %d", len(rows1))
	}

	// Verify Header remains at row 0
	expectedHeader := []string{"ID", "Отдел", "Комментарий"}
	if !reflect.DeepEqual(rows1[0], expectedHeader) {
		t.Errorf("header row was modified or moved: got %v, expected %v", rows1[0], expectedHeader)
	}

	// Verify that data rows were actually shuffled (not in 1..10 sequential order)
	shuffledIDs := make([]string, 10)
	for i := 1; i <= 10; i++ {
		shuffledIDs[i-1] = rows1[i][0]
	}
	originalIDs := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	if reflect.DeepEqual(shuffledIDs, originalIDs) {
		t.Errorf("expected rows to be shuffled, but IDs are still in original sequential order: %v", shuffledIDs)
	}

	// Verify multiset integrity: all IDs 1..10 must exist
	idCounts := make(map[string]int)
	for _, id := range shuffledIDs {
		idCounts[id]++
	}
	for _, id := range originalIDs {
		if idCounts[id] != 1 {
			t.Errorf("expected ID %s exactly once, got %d", id, idCounts[id])
		}
	}

	// 2. Determinism: same salt must produce identical row ordering
	var outBuf1Repeat bytes.Buffer
	if err := handler1.Process(ctx, strings.NewReader(inputData), &outBuf1Repeat, optsShuffle); err != nil {
		t.Fatalf("Process repeat failed: %v", err)
	}
	reader1Repeat := csv.NewReader(&outBuf1Repeat)
	rows1Repeat, _ := reader1Repeat.ReadAll()
	if !reflect.DeepEqual(rows1, rows1Repeat) {
		t.Errorf("shuffle is not deterministic with same salt: %v vs %v", rows1, rows1Repeat)
	}

	// 3. Different salt produces different row ordering
	anon2 := anonymizer.New("salt-beta-different")
	handler2 := formats.NewCSVHandler(anon2)
	var outBuf2 bytes.Buffer
	if err := handler2.Process(ctx, strings.NewReader(inputData), &outBuf2, optsShuffle); err != nil {
		t.Fatalf("Process with different salt failed: %v", err)
	}
	reader2 := csv.NewReader(&outBuf2)
	rows2, _ := reader2.ReadAll()
	shuffledIDs2 := make([]string, 10)
	for i := 1; i <= 10; i++ {
		shuffledIDs2[i-1] = rows2[i][0]
	}
	if reflect.DeepEqual(shuffledIDs, shuffledIDs2) {
		t.Errorf("expected different salt to produce different shuffle order, got same: %v", shuffledIDs)
	}

	// 4. When shuffle_rows = false (or omitted), order must be preserved
	var outBufNoShuffle bytes.Buffer
	if err := handler1.Process(ctx, strings.NewReader(inputData), &outBufNoShuffle, nil); err != nil {
		t.Fatalf("Process without shuffle failed: %v", err)
	}
	readerNoShuffle := csv.NewReader(&outBufNoShuffle)
	rowsNoShuffle, _ := readerNoShuffle.ReadAll()
	noShuffleIDs := make([]string, 10)
	for i := 1; i <= 10; i++ {
		noShuffleIDs[i-1] = rowsNoShuffle[i][0]
	}
	if !reflect.DeepEqual(noShuffleIDs, originalIDs) {
		t.Errorf("expected original order when shuffle_rows is false, got: %v", noShuffleIDs)
	}
}

func TestShuffleRows_Excel(t *testing.T) {
	ctx := context.Background()

	// Create test Excel workbook with 10 data rows
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Data"
	f.SetSheetName("Sheet1", sheet)

	// Header row
	headers := []string{"ID", "Отдел", "Примечание"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i := 1; i <= 10; i++ {
		rowNum := i + 1
		cell1, _ := excelize.CoordinatesToCellName(1, rowNum)
		cell2, _ := excelize.CoordinatesToCellName(2, rowNum)
		cell3, _ := excelize.CoordinatesToCellName(3, rowNum)
		_ = f.SetCellValue(sheet, cell1, fmt.Sprintf("%d", i))
		_ = f.SetCellValue(sheet, cell2, fmt.Sprintf("Отдел_%d", i))
		_ = f.SetCellValue(sheet, cell3, fmt.Sprintf("Заметка_%d", i))
	}

	var inBuf bytes.Buffer
	if err := f.Write(&inBuf); err != nil {
		t.Fatalf("failed to write input excel: %v", err)
	}

	anon1 := anonymizer.New("salt-excel-alpha")
	handler1 := formats.NewExcelHandler(anon1)

	// 1. Process with shuffle_rows = true
	var outBuf1 bytes.Buffer
	optsShuffle := map[string]interface{}{
		"shuffle_rows": true,
	}
	if err := handler1.Process(ctx, bytes.NewReader(inBuf.Bytes()), &outBuf1, optsShuffle); err != nil {
		t.Fatalf("Process excel with shuffle failed: %v", err)
	}

	outXl1, err := excelize.OpenReader(&outBuf1)
	if err != nil {
		t.Fatalf("failed to open output excel: %v", err)
	}
	defer outXl1.Close()

	rows1, err := outXl1.GetRows(sheet)
	if err != nil {
		t.Fatalf("failed to read rows from output excel: %v", err)
	}

	if len(rows1) != 11 {
		t.Fatalf("expected 11 rows in output excel, got %d", len(rows1))
	}

	// Verify Header at row 1 (index 0)
	if !reflect.DeepEqual(rows1[0], headers) {
		t.Errorf("excel header was modified: got %v, expected %v", rows1[0], headers)
	}

	// Verify data rows are shuffled
	shuffledIDs := make([]string, 10)
	for i := 1; i <= 10; i++ {
		shuffledIDs[i-1] = rows1[i][0]
	}
	originalIDs := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	if reflect.DeepEqual(shuffledIDs, originalIDs) {
		t.Errorf("expected excel rows to be shuffled, but IDs are still sequential: %v", shuffledIDs)
	}

	// Verify multiset integrity
	idCounts := make(map[string]int)
	for _, id := range shuffledIDs {
		idCounts[id]++
	}
	for _, id := range originalIDs {
		if idCounts[id] != 1 {
			t.Errorf("expected ID %s exactly once, got %d", id, idCounts[id])
		}
	}

	// 2. Determinism: same salt -> identical rows
	var outBuf1Repeat bytes.Buffer
	if err := handler1.Process(ctx, bytes.NewReader(inBuf.Bytes()), &outBuf1Repeat, optsShuffle); err != nil {
		t.Fatalf("Process repeat failed: %v", err)
	}
	outXl1Repeat, _ := excelize.OpenReader(&outBuf1Repeat)
	defer outXl1Repeat.Close()
	rows1Repeat, _ := outXl1Repeat.GetRows(sheet)
	if !reflect.DeepEqual(rows1, rows1Repeat) {
		t.Errorf("excel shuffle is not deterministic: %v vs %v", rows1, rows1Repeat)
	}

	// 3. Different salt -> different shuffle
	anon2 := anonymizer.New("salt-excel-beta")
	handler2 := formats.NewExcelHandler(anon2)
	var outBuf2 bytes.Buffer
	if err := handler2.Process(ctx, bytes.NewReader(inBuf.Bytes()), &outBuf2, optsShuffle); err != nil {
		t.Fatalf("Process with different salt failed: %v", err)
	}
	outXl2, _ := excelize.OpenReader(&outBuf2)
	defer outXl2.Close()
	rows2, _ := outXl2.GetRows(sheet)
	shuffledIDs2 := make([]string, 10)
	for i := 1; i <= 10; i++ {
		shuffledIDs2[i-1] = rows2[i][0]
	}
	if reflect.DeepEqual(shuffledIDs, shuffledIDs2) {
		t.Errorf("expected different salt to produce different excel shuffle order, got same: %v", shuffledIDs)
	}
}
