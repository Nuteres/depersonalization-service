package domain_test

import (
	"errors"
	"testing"

	"dep_service/internal/domain"
)

func TestDomainModelsInitialization(t *testing.T) {
	fi := domain.FileInfo{
		Path:                   "/test/file.xlsx",
		Name:                   "file.xlsx",
		Extension:              ".xlsx",
		FileType:               "table",
		Format:                 "xlsx",
		SupportedExportFormats: []string{"xlsx"},
		Size:                   1024,
		RowCount:               100,
		ParagraphCount:         0,
		PageCount:              1,
		Entities: []domain.EntityInfo{
			{
				ColumnIndex: 1,
				ColumnName:  "ФИО",
				Type:        "FIO",
				Description: "Полное имя",
				Action:      "replace",
			},
		},
		Headers: []string{"ID", "ФИО"},
		PreviewRows: []domain.PreviewRow{
			{
				Index:  1,
				Before: []string{"1", "Иванов Иван"},
				After:  []string{"1", "Петров Петр"},
			},
		},
		Sheets: []domain.SheetInfo{
			{
				Name:     "Sheet1",
				Index:    0,
				RowCount: 100,
				Headers:  []string{"ID", "ФИО"},
				Entities: []domain.EntityInfo{
					{
						ColumnIndex: 1,
						ColumnName:  "ФИО",
						Type:        "FIO",
						Description: "Полное имя",
						Action:      "replace",
					},
				},
				PreviewRows: []domain.PreviewRow{
					{
						Index:  1,
						Before: []string{"1", "Иванов Иван"},
						After:  []string{"1", "Петров Петр"},
					},
				},
			},
		},
		ActiveSheet: "Sheet1",
		TextSnippets: []domain.DocxTextSnippet{
			{
				Original:   "Текст до",
				Anonymized: "Текст после",
				Type:       "FIO",
			},
		},
		PDFTextSnippet:     "PDF до",
		PDFRedactedSnippet: "PDF после",
	}

	if fi.Name != "file.xlsx" || fi.RowCount != 100 {
		t.Fatalf("unexpected FileInfo values: %+v", fi)
	}

	if len(fi.Entities) != 1 || fi.Entities[0].ColumnName != "ФИО" {
		t.Fatalf("unexpected EntityInfo in FileInfo: %+v", fi.Entities)
	}

	if len(fi.Sheets) != 1 || fi.Sheets[0].Name != "Sheet1" {
		t.Fatalf("unexpected SheetInfo in FileInfo: %+v", fi.Sheets)
	}

	opts := domain.ProcessOptions{
		InputPath:        "/test/in.xlsx",
		OutputPath:       "/test/out.xlsx",
		Salt:             "test-salt",
		DisabledCols:     []int{1, 2},
		DisabledBySheet:  map[string][]int{"Sheet1": {1}},
		CreateKey:        true,
		KeyPassword:      "secret123",
		KeyPath:          "/test/key.depmap",
	}
	if opts.Salt != "test-salt" || !opts.CreateKey || opts.DisabledBySheet["Sheet1"][0] != 1 {
		t.Fatalf("unexpected ProcessOptions: %+v", opts)
	}

	restoreOpts := domain.RestoreOptions{
		InputPath:  "/test/out.xlsx",
		KeyPath:    "/test/key.depmap",
		Password:   "secret123",
		OutputPath: "/test/restored.xlsx",
	}
	if restoreOpts.Password != "secret123" {
		t.Fatalf("unexpected RestoreOptions: %+v", restoreOpts)
	}

	procRes := domain.ProcessResult{
		Success:       true,
		InputPath:     "/test/in.xlsx",
		OutputPath:    "/test/out.xlsx",
		KeyPath:       "/test/key.depmap",
		RowsCount:     100,
		EntitiesCount: 5,
		DurationMs:    42,
		Message:       "OK",
	}
	if !procRes.Success || procRes.RowsCount != 100 {
		t.Fatalf("unexpected ProcessResult: %+v", procRes)
	}

	keyInfo := domain.KeyFileInfo{
		Path:             "/test/key.depmap",
		OriginalFileName: "in.xlsx",
		CreatedAt:        "2026-10-01T12:00:00Z",
		RecordsCount:     10,
		Valid:            true,
	}
	if !keyInfo.Valid || keyInfo.RecordsCount != 10 {
		t.Fatalf("unexpected KeyFileInfo: %+v", keyInfo)
	}

	restoreRes := domain.RestoreResult{
		Success:       true,
		InputPath:     "/test/out.xlsx",
		KeyPath:       "/test/key.depmap",
		OutputPath:    "/test/restored.xlsx",
		RestoredCells: 15,
		DurationMs:    25,
	}
	if !restoreRes.Success || restoreRes.RestoredCells != 15 {
		t.Fatalf("unexpected RestoreResult: %+v", restoreRes)
	}

	var reporterCalled bool
	var reporter domain.ProgressReporter = func(processed, total, percent int) {
		if processed == 50 && total == 100 && percent == 50 {
			reporterCalled = true
		}
	}
	reporter(50, 100, 50)
	if !reporterCalled {
		t.Fatalf("expected ProgressReporter to be callable")
	}

	if !errors.Is(domain.ErrInvalidPassword, domain.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword")
	}
	if !errors.Is(domain.ErrUnsupportedFormat, domain.ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat")
	}
	if !errors.Is(domain.ErrOperationCanceled, domain.ErrOperationCanceled) {
		t.Fatalf("expected ErrOperationCanceled")
	}
}
