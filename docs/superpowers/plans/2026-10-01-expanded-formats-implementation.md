# Expanded Formats (.xlsb, .ods, .xls, .docx, .odt) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement full de-identification, reverse restoration (`.depmap`), inspection, and adaptive UI preview for 5 new document formats: `.ods` (OpenDocument Spreadsheet), `.xlsb` (Excel Binary Workbook), `.xls` (Excel 97–2003 BIFF8), `.docx` (Microsoft Word OpenXML), and `.odt` (OpenDocument Text).

**Architecture:** Modally separate tabular processing (`SpreadsheetHandler`) from textual document processing (`TextDocumentHandler`). For OpenDocument and OpenXML (`.ods`, `.docx`, `.odt`), use pure Go standard library `archive/zip` and `encoding/xml` to stream and substitute text nodes while preserving 100% of formatting, styles, equations, tables, and images. For binary spreadsheets (`.xlsb`, `.xls`), read data via pure Go libraries and serialize into modern `.xlsx` with format conversion options in the save dialog.

**Tech Stack:** Go 1.25, `archive/zip`, `encoding/xml`, `excelize/v2`, `github.com/TsubasaBE/go-xlsb`, `github.com/extrame/xls`, Wails v2, vanilla ES6+ JS / Tailwind CSS.

**Spec:** `docs/superpowers/specs/2026-10-01-expanded-formats-design.md`

## Global Constraints

- **On-premise / Air-gapped:** 0 external HTTP/HTTPS calls during build or runtime; all dependencies vendored in `vendor/`.
- **Pure Go / Standalone:** No external runtime dependencies (no LibreOffice, Python, Java, or CGO tools required).
- **Format Integrity:** In `.docx`, `.odt`, and `.ods`, fonts, tables, styles, and non-PII content remain 100% untouched byte-for-byte.
- **Bi-directional Reversibility:** All structured formats must support lossless `.depmap` key generation and reverse restoration.
- **Export Format Flexibility:** Users can choose output format for ODS (`.ods`, `.xlsx`, `.csv`) and XLSX (`.xlsx`, `.csv`); XLS and XLSB save to `.xlsx`; DOCX and ODT save to `.docx` or `.odt`.

---

### Task 1: OpenDocument Spreadsheet (.ods) Handler & Restorer

**Files:**
- Create: `pkg/formats/ods_handler.go`
- Create: `pkg/formats/restore_ods.go`
- Test: `pkg/formats/ods_test.go`

**Interfaces:**
- Consumes: `anonymizer.Anonymizer`, `detector.DetectColumnType`, `depmap.DepMap`
- Produces:
  - `type ODSHandler struct { Anon *anonymizer.Anonymizer }`
  - `func (h *ODSHandler) SupportedExtensions() []string` (returns `[]string{".ods"}`)
  - `func (h *ODSHandler) Inspect(r io.ReaderAt, size int64) ([]ODSSheetInfo, error)`
  - `func (h *ODSHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error)`
  - `func RestoreODS(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error)`

- [ ] **Step 1: Write failing tests for ODS inspection, anonymization, and reverse restoration**

Create `pkg/formats/ods_test.go`:
```go
package formats

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"dep_service/pkg/anonymizer"
)

func createTestODS(t *testing.T) []byte {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	// mimetype (must be uncompressed in standard ODF)
	mimeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mimeHeader)
	if err != nil {
		t.Fatalf("failed to create mimetype header: %v", err)
	}
	mw.Write([]byte("application/vnd.oasis.opendocument.spreadsheet"))

	// content.xml with 1 sheet, headers and data
	contentXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
 xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0"
 xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
 <office:body>
  <office:spreadsheet>
   <table:table table:name="Сотрудники">
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>ФИО</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>СНИЛС</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>Телефон</text:p></table:table-cell>
    </table:table-row>
    <table:table-row>
     <table:table-cell office:value-type="string"><text:p>Иванов Иван Иванович</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>112-233-445 95</text:p></table:table-cell>
     <table:table-cell office:value-type="string"><text:p>+7 916 123-45-67</text:p></table:table-cell>
    </table:table-row>
   </table:table>
  </office:spreadsheet>
 </office:body>
</office:document-content>`

	cw, err := zw.Create("content.xml")
	if err != nil {
		t.Fatalf("failed to create content.xml: %v", err)
	}
	cw.Write([]byte(contentXML))

	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	return buf.Bytes()
}

func TestODSHandler_Lifecycle(t *testing.T) {
	rawODS := createTestODS(t)
	anon := anonymizer.New("test-salt-ods")
	handler := &ODSHandler{Anon: anon}

	// 1. Inspect
	reader := bytes.NewReader(rawODS)
	sheets, err := handler.Inspect(reader, int64(len(rawODS)))
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if len(sheets) != 1 || sheets[0].Name != "Сотрудники" {
		t.Fatalf("unexpected sheets: %+v", sheets)
	}
	if len(sheets[0].Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(sheets[0].Columns))
	}

	// 2. Process / Anonymize
	outBuf := new(bytes.Buffer)
	opts := map[string]interface{}{}
	depMap, err := handler.Process(context.Background(), reader, int64(len(rawODS)), outBuf, opts)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if depMap == nil || len(depMap.Entities) == 0 {
		t.Fatalf("expected non-empty depMap, got %+v", depMap)
	}

	anonymizedBytes := outBuf.Bytes()
	if bytes.Contains(anonymizedBytes, []byte("Иванов Иван Иванович")) {
		t.Errorf("anonymized ODS still contains original FIO")
	}

	// 3. Restore
	restoredBuf := new(bytes.Buffer)
	anonReader := bytes.NewReader(anonymizedBytes)
	count, err := RestoreODS(context.Background(), anonReader, int64(len(anonymizedBytes)), restoredBuf, depMap)
	if err != nil {
		t.Fatalf("RestoreODS failed: %v", err)
	}
	if count < 2 {
		t.Errorf("expected at least 2 restored cells, got %d", count)
	}
	if !strings.Contains(restoredBuf.String(), "Иванов Иван Иванович") {
		t.Errorf("restored ODS does not contain original FIO")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/formats -run TestODSHandler_Lifecycle`
Expected: Compilation failure with "undefined: ODSHandler, RestoreODS"

- [ ] **Step 3: Implement ODSHandler and RestoreODS**

Create `pkg/formats/ods_handler.go`:
- Implement `ODSHandler` extracting `content.xml`, parsing `<table:table>`, `<table:table-row>`, `<table:table-cell>` using `encoding/xml`.
- Replaces PII cells with synthetic values using `h.Anon` and populates `depmap.DepMap`.
- Writes output ZIP archive preserving `mimetype` uncompressed and packing updated `content.xml` with all other original files.

Create `pkg/formats/restore_ods.go`:
- Implement `RestoreODS` reading `content.xml`, checking cell texts against `dm.LookupOriginal`, replacing with originals, and writing restored `.ods`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/formats -run TestODSHandler`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/formats/ods_handler.go pkg/formats/restore_ods.go pkg/formats/ods_test.go
git commit -m "feat(ods): implement OpenDocument Spreadsheet anonymizer and restorer"
```

---

### Task 2: Binary Excel (.xlsb & .xls) Handlers

**Files:**
- Create: `pkg/formats/xlsb_handler.go`
- Create: `pkg/formats/xls_handler.go`
- Test: `pkg/formats/xls_test.go`
- Test: `pkg/formats/xlsb_test.go`
- Modify: `go.mod`, `go.sum`, `vendor/modules.txt`

**Interfaces:**
- Consumes: `github.com/TsubasaBE/go-xlsb`, `github.com/extrame/xls`, `excelize/v2`, `anonymizer.Anonymizer`
- Produces:
  - `type XLSBHandler struct { Anon *anonymizer.Anonymizer }`
  - `type XLSHandler struct { Anon *anonymizer.Anonymizer }`
  - `func (h *XLSBHandler) Inspect(path string) ([]SheetInfo, error)`
  - `func (h *XLSBHandler) ProcessToXLSX(ctx context.Context, path string, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error)`
  - `func (h *XLSHandler) Inspect(path string) ([]SheetInfo, error)`
  - `func (h *XLSHandler) ProcessToXLSX(ctx context.Context, path string, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error)`

- [ ] **Step 1: Add dependencies and update vendor**

Run:
```bash
go get github.com/TsubasaBE/go-xlsb github.com/extrame/xls
go mod vendor
```

- [ ] **Step 2: Write tests for XLS and XLSB handlers**

Create `pkg/formats/xls_test.go` and `pkg/formats/xlsb_test.go` verifying row reading, PII column detection, anonymization, and writing valid `.xlsx` files that can be reopened with `excelize`.

- [ ] **Step 3: Implement XLSBHandler and XLSHandler**

- In `pkg/formats/xlsb_handler.go`: open workbook with `xlsb.Open(path)`, iterate sheets and rows, detect headers, anonymize cells, stream into `excelize.NewFile()`, save to writer.
- In `pkg/formats/xls_handler.go`: open workbook with `xls.Open(path, "utf-8")`, iterate sheets and rows, anonymize, stream into `excelize.NewFile()`, save to writer.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -v ./pkg/formats -run 'TestXLS|TestXLSB'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/formats/xlsb_handler.go pkg/formats/xls_handler.go pkg/formats/xls_test.go pkg/formats/xlsb_test.go go.mod go.sum vendor/
git commit -m "feat(excel): add pure Go handlers for binary Excel (.xls and .xlsb)"
```

---

### Task 3: Microsoft Word (.docx) Text Processor & Restorer

**Files:**
- Create: `pkg/formats/docx_handler.go`
- Create: `pkg/formats/restore_docx.go`
- Test: `pkg/formats/docx_test.go`

**Interfaces:**
- Consumes: `anonymizer.Anonymizer`, `detector.DetectPIIEntities`, `depmap.DepMap`
- Produces:
  - `type DOCXHandler struct { Anon *anonymizer.Anonymizer }`
  - `type DocxTextSnippet struct { Original string; Anonymized string; Type string }`
  - `func (h *DOCXHandler) Inspect(r io.ReaderAt, size int64) (paragraphCount int, entities []EntityCount, snippets []DocxTextSnippet, error)`
  - `func (h *DOCXHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error)`
  - `func RestoreDOCX(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error)`

- [ ] **Step 1: Write failing tests for DOCX**

Create `pkg/formats/docx_test.go` with synthetic Word document containing:
- Paragraph with full name: `<w:p><w:r><w:t>Договор подписал Иванов Иван Иванович</w:t></w:r></w:p>`
- Paragraph with split run: `<w:p><w:r><w:t>Паспорт: 45</w:t></w:r><w:r><w:t>10 123890</w:t></w:r></w:p>`
- Table cell with phone: `<w:tbl>...<w:tc><w:p><w:r><w:t>+7 916 123-45-67</w:t></w:r></w:p></w:tc></w:tbl>`
Verify that inspecting detects the entities, processing replaces them with synthetic values while keeping Word XML valid, and `RestoreDOCX` restores original values verbatim.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/formats -run TestDOCX`
Expected: FAIL

- [ ] **Step 3: Implement DOCXHandler and RestoreDOCX**

- In `pkg/formats/docx_handler.go`:
  - Open zip with `zip.NewReader`.
  - Process `word/document.xml`, `word/header*.xml`, `word/footer*.xml`.
  - Scan paragraph runs, concatenate text, find regex/checksum PII matches, replace text in `<w:t>` nodes, record replacements in `.depmap`.
  - Repack archive with untouched media/styles.
- In `pkg/formats/restore_docx.go`:
  - Open zip, scan text nodes in XML documents, substitute reverse map tokens, repack.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/formats -run TestDOCX`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/formats/docx_handler.go pkg/formats/restore_docx.go pkg/formats/docx_test.go
git commit -m "feat(docx): implement Microsoft Word OpenXML anonymizer and restorer"
```

---

### Task 4: OpenDocument Text (.odt) Processor & Restorer

**Files:**
- Create: `pkg/formats/odt_handler.go`
- Create: `pkg/formats/restore_odt.go`
- Test: `pkg/formats/odt_test.go`

**Interfaces:**
- Consumes: `anonymizer.Anonymizer`, `depmap.DepMap`
- Produces:
  - `type ODTHandler struct { Anon *anonymizer.Anonymizer }`
  - `func (h *ODTHandler) Inspect(r io.ReaderAt, size int64) (paragraphCount int, entities []EntityCount, snippets []DocxTextSnippet, error)`
  - `func (h *ODTHandler) Process(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, options map[string]interface{}) (*depmap.DepMap, error)`
  - `func RestoreODT(ctx context.Context, r io.ReaderAt, size int64, w io.Writer, dm *depmap.DepMap) (int, error)`

- [ ] **Step 1: Write failing tests for ODT**

Create `pkg/formats/odt_test.go` creating a valid `.odt` ZIP with `mimetype` and `content.xml` containing `<text:p>` and `<text:span>` with Russian PII (ФИО, СНИЛС, паспорт). Verify inspect, process, and restore.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./pkg/formats -run TestODT`
Expected: FAIL

- [ ] **Step 3: Implement ODTHandler and RestoreODT**

- In `pkg/formats/odt_handler.go`: parse `content.xml`, walk `<text:p>`, replace detected PII with deterministic synthetic values, record pairs in `depmap.DepMap`, write valid ODT zip.
- In `pkg/formats/restore_odt.go`: substitute synthetic values back from `.depmap`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./pkg/formats -run TestODT`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/formats/odt_handler.go pkg/formats/restore_odt.go pkg/formats/odt_test.go
git commit -m "feat(odt): implement OpenDocument Text anonymizer and restorer"
```

---

### Task 5: Wails IPC Backend Integration

**Files:**
- Modify: `cmd/app/wails_app.go`
- Test: `cmd/app/wails_app_test.go`

**Interfaces:**
- Updates:
  - `FileInfo`: adds `FileType` (`"spreadsheet" | "text" | "pdf"`), `Format`, `SupportedExportFormats []string`, `TextSnippets []TextSnippet`, `ParagraphCount int`.
  - `InspectFile(path)`: routes `.xlsx`, `.xlsb`, `.ods`, `.xls`, `.csv`, `.docx`, `.odt`, `.pdf`.
  - `ProcessFileTo(inputPath, salt, destPath, options)`: handles format conversion (e.g. `.ods` -> `.xlsx`, `.xlsb` -> `.xlsx`), disabled entity categories, and `.depmap` generation.
  - `RestoreFileTo(inputPath, keyPath, password, destPath)`: handles `.xlsx`, `.csv`, `.ods`, `.docx`, `.odt`.

- [ ] **Step 1: Write failing integration tests for all new formats**

In `cmd/app/wails_app_test.go`:
- Test inspecting and processing `.ods`.
- Test inspecting and processing `.docx`.
- Test inspecting and processing `.odt`.
- Test reverse restoration of `.ods` and `.docx` using `.depmap`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test -v ./cmd/app -run 'TestApp_ExpandedFormats'`
Expected: FAIL

- [ ] **Step 3: Implement routing and handlers in wails_app.go**

- Expand `InspectFile` to detect extension and invoke `inspectODS`, `inspectXLSB`, `inspectXLS`, `inspectDOCX`, `inspectODT`.
- Expand `ProcessFileTo` to handle format conversion and invoke the right processor.
- Expand `RestoreFileTo` to route `.ods`, `.docx`, `.odt` to their respective restorers.

- [ ] **Step 4: Run all app tests to verify they pass**

Run: `go test -v ./cmd/app`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/app/wails_app.go cmd/app/wails_app_test.go
git commit -m "feat(app): integrate expanded formats into Wails IPC backend"
```

---

### Task 6: Frontend UI Adaptation

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/js/state.js`
- Modify: `frontend/js/ui_common.js`
- Modify: `frontend/js/depersonalize.js`
- Modify: `frontend/js/restore.js`

- [ ] **Step 1: Update file dialog filters and state**

In `frontend/js/state.js` and `frontend/js/ui_common.js`:
- Add support for `.ods`, `.xlsb`, `.xls`, `.docx`, `.odt` in file dialog filters and drag-and-drop detectors.

- [ ] **Step 2: Add text document adaptive view in depersonalize.js**

- If `fileInfo.fileType === 'text'`:
  - Hide sheet tabs and column table.
  - Display entity category toggles with counters (`ФИО: 3`, `Паспорт: 1`, `Телефон: 2`, etc.).
  - Display side-by-side or stacked snippet cards showing original vs anonymized context phrases.

- [ ] **Step 3: Add format export selector in save modal**

- In save modal (`frontend/js/depersonalize.js` and `frontend/index.html`):
  - Populate `<select id="exportFormatSelect">` dynamically based on `fileInfo.supportedExportFormats`.
  - For `.ods`: options `.ods`, `.xlsx`, `.csv`.
  - For `.xls` / `.xlsb`: pre-selected `.xlsx` with info badge.
  - For `.docx`: options `.docx`, `.odt`.
  - For `.odt`: options `.odt`, `.docx`.

- [ ] **Step 4: Validate JavaScript syntax**

Run: `node --check frontend/js/*.js`
Expected: All files check clean without errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/
git commit -m "feat(ui): add adaptive text document preview and format export selector"
```

---

### Task 7: Full System Verification & Multi-Platform Build

**Files:**
- Verify: all unit tests across `pkg/anonymizer`, `pkg/detector`, `pkg/depmap`, `pkg/formats`, `cmd/app`
- Verify: binary compilation for macOS, Windows, and Alt Linux

- [ ] **Step 1: Run complete test suite**

Run: `go test -v ./...`
Expected: 100% tests pass.

- [ ] **Step 2: Build desktop binaries**

Run:
```bash
make build-macos
make build-windows
```
Expected: `bin/depersonalizer` (macOS Universal) and `bin/depersonalizer-windows-amd64.exe` built successfully.

- [ ] **Step 3: Update documentation**

Update `README.md` with the full list of supported formats (`.xlsx`, `.xlsb`, `.ods`, `.xls`, `.csv`, `.docx`, `.odt`, `.pdf`) and reversible restoration.

- [ ] **Step 4: Commit**

```bash
git add README.md bin/
git commit -m "chore: verify expanded format builds and update documentation"
```
