package desktop_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dep_service/internal/desktop"
	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
)

func TestMockDocumentsWithDesktopApp(t *testing.T) {
	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km, insp)
	rest := restorer.New(km)
	app := desktop.New(insp, prev, proc, rest, km)

	mockDir := filepath.Join("..", "..", "mock_documents")
	if _, err := os.Stat(mockDir); err != nil {
		t.Skip("mock_documents directory not found, skipping mock verification test")
	}

	var files []string
	err := filepath.Walk(mockDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && !strings.HasPrefix(info.Name(), ".") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk mock_documents: %v", err)
	}

	if len(files) == 0 {
		t.Fatalf("no files found in mock_documents")
	}

	t.Logf("Found %d mock files to verify with DesktopApp", len(files))

	tempDir := t.TempDir()

	for _, file := range files {
		relPath, _ := filepath.Rel(mockDir, file)
		t.Run(relPath, func(t *testing.T) {
			// 1. Test DesktopApp.InspectFile
			info, err := app.InspectFile(file)
			if err != nil {
				t.Fatalf("InspectFile failed: %v", err)
			}
			if info == nil {
				t.Fatalf("InspectFile returned nil FileInfo")
			}
			if len(info.Entities) == 0 {
				t.Fatalf("InspectFile found 0 entities in %s", relPath)
			}
			t.Logf("[%s] Type=%s Format=%s Entities=%d", relPath, info.FileType, info.Format, len(info.Entities))

			// 2. If table, test GetSheetRows (used by frontend preview grid)
			if info.FileType == "table" {
				rows, err := app.GetTableRows(file, 0, 10, "test-salt", nil)
				if err != nil {
					t.Fatalf("GetTableRows failed: %v", err)
				}
				if len(rows) == 0 {
					t.Fatalf("GetTableRows returned empty preview rows")
				}
				t.Logf("  Preview rows: %d (Headers: %d)", len(rows), len(info.Headers))
			} else {
				// If text, verify previews
				if len(info.ParagraphPreviews) == 0 && len(info.TextSnippets) == 0 {
					t.Fatalf("Text file has no paragraph previews or snippets")
				}
				t.Logf("  Paragraph previews: %d, Snippets: %d", len(info.ParagraphPreviews), len(info.TextSnippets))
			}

			// 3. Test ProcessFileTo (Full anonymization pipeline)
			baseName := filepath.Base(file)
			ext := filepath.Ext(baseName)
			outExt := ext
			if ext == ".xls" || ext == ".xlsb" {
				outExt = ".xlsx"
			}
			outPath := filepath.Join(tempDir, "anon_"+baseName+outExt)
			keyPath := filepath.Join(tempDir, "key_"+baseName+".depmap")
			password := "MockTestPass2026!"

			procRes, err := app.ProcessFileTo(
				file,
				outPath,
				"test-salt-salt",
				nil,
				true,
				password,
				keyPath,
				false,
				string(domain.StrategySemanticTokens),
			)
			if err != nil {
				t.Fatalf("ProcessFileTo failed: %v", err)
			}
			if procRes == nil {
				t.Fatalf("ProcessFileTo returned nil result")
			}
			if _, err := os.Stat(outPath); err != nil {
				t.Fatalf("Output anonymized file does not exist: %v", err)
			}
			if strings.ToLower(ext) != ".pdf" {
				if _, err := os.Stat(keyPath); err != nil {
					t.Fatalf("Key file (.depmap) was not created: %v", err)
				}
			}

			// 4. Test RestoreFile if format supports restoration
			switch strings.ToLower(ext) {
			case ".csv", ".tsv", ".xlsx", ".ods", ".docx", ".odt":
				restPath := filepath.Join(tempDir, "restored_"+baseName)
				restRes, err := app.RestoreFileTo(outPath, keyPath, password, restPath)
				if err != nil {
					t.Fatalf("RestoreFileTo failed: %v", err)
				}
				if restRes == nil {
					t.Fatalf("RestoreFile returned nil result")
				}
				if _, err := os.Stat(restPath); err != nil {
					t.Fatalf("Restored file does not exist: %v", err)
				}
				t.Logf("  Successfully anonymized -> depmap key created -> restored (%d items)", restRes.RestoredCells)
			}
		})
	}
}
