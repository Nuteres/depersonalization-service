# Deletion Mode (ModeDelete) and Header-Aware Restoration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a dedicated personal data deletion mode (`ModeDelete`) replacing sensitive data with `[УДАЛЕНО]` across all 8 file formats, with resilient header-aware restoration protecting against column shifts introduced by external AI models.

**Architecture:** Extend `domain.ProcessOptions` with `ProcessingMode ("replace" | "delete")`. In `pkg/formats`, process detected entities to `[УДАЛЕНО]` during deletion mode and record `CellMappings` ("Sheet!Header:Row" -> origVal) and `TargetHeaders` in `pkg/depmap.DepMap`. Implement header-aware column resolution in `RestoreExcel`, `RestoreCSV`, and `RestoreODS`. Wire Svelte 5 UI with segmented `[ Замена | Удаление ]` control, dynamic preview updates, and salt hiding.

**Tech Stack:** Go 1.23, Wails v2, Svelte 5 (Runes), Tailwind v4, Excelize v2, AES-256-GCM / PBKDF2.

**Spec:** [docs/superpowers/specs/2026-10-09-deletion-mode-design.md](file:///Users/nuter/Go/dep_service/docs/superpowers/specs/2026-10-09-deletion-mode-design.md)

## Global Constraints
- **100% Air-Gapped**: Zero outbound network, external HTTP/DNS or telemetry calls.
- **Go Vendor**: All dependencies are vendored (`-mod=vendor`).
- **Semantic Versioning**: Minor bump from 1.6.3 to 1.7.0 (`internal/version/version.go`, `frontend/package.json`, `Makefile`).
- **Changelog & Docs**: Mandatory updates to `CHANGELOG.md`, `docs/USER_GUIDE.md`, `docs/ARCHITECTURE.md`, `README.md`.
- **Quality Gates**: All tests pass (`go test -mod=vendor ./...`), `npm --prefix frontend run check`, `npm --prefix frontend run build`.

---

### Task 1: Domain Models and DepMap Container Extension

**Files:**
- Modify: `internal/domain/options.go:1-23`
- Modify: `pkg/depmap/depmap.go:60-120`
- Test: `pkg/depmap/depmap_test.go`

**Interfaces:**
- Produces:
  - `domain.ProcessingMode`: string type (`domain.ModeReplace = "replace"`, `domain.ModeDelete = "delete"`)
  - `domain.ProcessOptions.Mode`: `domain.ProcessingMode`
  - `depmap.DepMap`: fields `Mode string`, `CellMappings map[string]string`, `TargetHeaders []string`, `OrdinalMappings []string`
  - Methods on `*depmap.DepMap`:
    - `PutCell(sheet, header string, row int, original string)`
    - `GetCell(sheet, header string, row int) (string, bool)`
    - `AddTargetHeader(header string)`
    - `PutOrdinal(original string)`

- [ ] **Step 1: Write failing test for DepMap extension**

Add test in `pkg/depmap/depmap_test.go`:
```go
func TestDepMap_CellAndOrdinalMappings(t *testing.T) {
	dm := New("test.xlsx")
	dm.Mode = "delete"
	dm.PutCell("Лист1", "ФИО", 5, "Иванов Иван Иванович")
	dm.PutCell("Лист1", "Телефон", 5, "+79991234567")
	dm.AddTargetHeader("ФИО")
	dm.AddTargetHeader("Телефон")
	dm.PutOrdinal("Паспорт серии 1234")

	val, ok := dm.GetCell("Лист1", "ФИО", 5)
	if !ok || val != "Иванов Иван Иванович" {
		t.Fatalf("expected 'Иванов Иван Иванович', got %q (found: %v)", val, ok)
	}

	tmpFile := filepath.Join(t.TempDir(), "test.depmap")
	pwd, err := dm.SaveFileWithAutoPassword(tmpFile, "TestPassword123!")
	if err != nil {
		t.Fatalf("SaveFileWithAutoPassword failed: %v", err)
	}

	loaded, err := LoadFile(tmpFile, pwd)
	if err != nil {
		t.Fatalf("LoadFile failed: %v", err)
	}

	if loaded.Mode != "delete" {
		t.Errorf("expected Mode 'delete', got %q", loaded.Mode)
	}
	loadedVal, ok := loaded.GetCell("Лист1", "ФИО", 5)
	if !ok || loadedVal != "Иванов Иван Иванович" {
		t.Errorf("expected loaded cell value 'Иванов Иван Иванович', got %q", loadedVal)
	}
	if len(loaded.TargetHeaders) != 2 || loaded.TargetHeaders[0] != "ФИО" {
		t.Errorf("unexpected TargetHeaders: %v", loaded.TargetHeaders)
	}
	if len(loaded.OrdinalMappings) != 1 || loaded.OrdinalMappings[0] != "Паспорт серии 1234" {
		t.Errorf("unexpected OrdinalMappings: %v", loaded.OrdinalMappings)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./pkg/depmap -run TestDepMap_CellAndOrdinalMappings`
Expected: FAIL (fields/methods `PutCell`, `GetCell`, etc. not defined).

- [ ] **Step 3: Implement domain models and DepMap extension**

In `internal/domain/options.go`:
```go
package domain

type ProcessingMode string

const (
	ModeReplace ProcessingMode = "replace"
	ModeDelete  ProcessingMode = "delete"
)

type ProcessOptions struct {
	InputPath       string           `json:"inputPath"`
	OutputPath      string           `json:"outputPath"`
	Mode            ProcessingMode   `json:"mode"`
	Salt            string           `json:"salt"`
	DisabledCols    []int            `json:"disabledCols"`
	DisabledBySheet map[string][]int `json:"disabledBySheet"`
	CreateKey       bool             `json:"createKey"`
	KeyPassword     string           `json:"keyPassword"`
	KeyPath         string           `json:"keyPath"`
	ShuffleRows     bool             `json:"shuffleRows"`
}
```

In `pkg/depmap/depmap.go`, update `DepMap` struct and add helper methods:
```go
type DepMap struct {
	Version          int               `json:"version"`
	CreatedAt        time.Time         `json:"created_at"`
	OriginalFileName string            `json:"original_file_name"`
	RecordsCount     int               `json:"records_count"`
	Mode             string            `json:"mode,omitempty"`
	Mapping          map[string]string `json:"mapping,omitempty"`
	Entities         map[string]string `json:"entities,omitempty"`
	CellMappings     map[string]string `json:"cell_mappings,omitempty"`
	TargetHeaders    []string          `json:"target_headers,omitempty"`
	OrdinalMappings  []string          `json:"ordinal_mappings,omitempty"`
}

func (dm *DepMap) PutCell(sheet, header string, row int, original string) {
	if dm.CellMappings == nil {
		dm.CellMappings = make(map[string]string)
	}
	key := fmt.Sprintf("%s!%s:%d", strings.TrimSpace(sheet), strings.TrimSpace(header), row)
	dm.CellMappings[key] = original
	dm.RecordsCount = len(dm.CellMappings) + len(dm.Mapping)
}

func (dm *DepMap) GetCell(sheet, header string, row int) (string, bool) {
	if dm.CellMappings == nil {
		return "", false
	}
	key := fmt.Sprintf("%s!%s:%d", strings.TrimSpace(sheet), strings.TrimSpace(header), row)
	val, ok := dm.CellMappings[key]
	return val, ok
}

func (dm *DepMap) AddTargetHeader(header string) {
	h := strings.TrimSpace(header)
	if h == "" {
		return
	}
	for _, existing := range dm.TargetHeaders {
		if existing == h {
			return
		}
	}
	dm.TargetHeaders = append(dm.TargetHeaders, h)
}

func (dm *DepMap) PutOrdinal(original string) {
	dm.OrdinalMappings = append(dm.OrdinalMappings, original)
	dm.RecordsCount = len(dm.CellMappings) + len(dm.Mapping) + len(dm.OrdinalMappings)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -mod=vendor ./pkg/depmap -run TestDepMap_CellAndOrdinalMappings`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add internal/domain/options.go pkg/depmap/depmap.go pkg/depmap/depmap_test.go
git commit -m "feat(depmap): add Mode, CellMappings, TargetHeaders, and OrdinalMappings to DepMap"
```

---

### Task 2: Core Processing with ModeDelete across Format Handlers

**Files:**
- Modify: `pkg/formats/columns.go:440-485`
- Modify: `pkg/formats/excel_handler.go:1-250`
- Modify: `pkg/formats/csv_handler.go:1-200`
- Modify: `pkg/formats/docx_handler.go:1-250`
- Modify: `pkg/formats/pdf_handler.go:1-200`
- Test: `pkg/formats/columns_test.go`
- Test: `pkg/formats/excel_test.go`
- Test: `pkg/formats/docx_test.go`

**Interfaces:**
- Consumes: `domain.ProcessingMode`, `depmap.DepMap`
- Produces:
  - `formats.DeletionMarker = "[УДАЛЕНО]"`
  - `formats.ProcessCell(anon *anonymizer.Anonymizer, colType ColumnType, val string, mode domain.ProcessingMode, dateRange int) string`
  - Excel/CSV/DOCX/PDF handlers accept `"mode": domain.ProcessingMode` in options map.

- [ ] **Step 1: Write failing test for ProcessCell and DeletionMarker**

Add test in `pkg/formats/columns_test.go`:
```go
func TestProcessCell_ModeDelete(t *testing.T) {
	anon := anonymizer.New("test-salt")
	val := "Иванов Иван Иванович"
	
	replaced := ProcessCell(anon, ColFIO, val, domain.ModeReplace, 30)
	if replaced == val || replaced == DeletionMarker {
		t.Fatalf("expected synthetic replacement, got %q", replaced)
	}

	deleted := ProcessCell(anon, ColFIO, val, domain.ModeDelete, 30)
	if deleted != DeletionMarker {
		t.Fatalf("expected %q, got %q", DeletionMarker, deleted)
	}

	empty := ProcessCell(anon, ColFIO, "", domain.ModeDelete, 30)
	if empty != "" {
		t.Fatalf("expected empty string for empty cell, got %q", empty)
	}

	unknown := ProcessCell(anon, ColUnknown, "Simple Text", domain.ModeDelete, 30)
	if unknown != "Simple Text" {
		t.Fatalf("expected untouched text for ColUnknown, got %q", unknown)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./pkg/formats -run TestProcessCell_ModeDelete`
Expected: FAIL (undefined `ProcessCell`, `DeletionMarker`).

- [ ] **Step 3: Implement ProcessCell in columns.go and wire handlers**

In `pkg/formats/columns.go`:
```go
const DeletionMarker = "[УДАЛЕНО]"

func ProcessCell(anon *anonymizer.Anonymizer, colType ColumnType, val string, mode domain.ProcessingMode, dateRange int) string {
	if strings.TrimSpace(val) == "" || colType == ColUnknown {
		return val
	}
	if mode == domain.ModeDelete {
		return DeletionMarker
	}
	return AnonymizeCell(anon, colType, val, dateRange)
}
```

In `excel_handler.go`, `csv_handler.go`, read mode from options:
```go
mode := domain.ModeReplace
if m, ok := options["mode"].(domain.ProcessingMode); ok && m != "" {
    mode = m
} else if s, ok := options["mode"].(string); ok && s != "" {
    mode = domain.ProcessingMode(s)
}
```
When iterating cells and anonymizing:
```go
newVal := ProcessCell(h.Anon, ct, val, mode, dateRange)
if ct != ColUnknown && strings.TrimSpace(val) != "" && depMap != nil {
    if mode == domain.ModeDelete {
        depMap.PutCell(sheetName, headerName, rowIdx, val)
        depMap.AddTargetHeader(headerName)
    } else {
        depMap.Put(newVal, val)
        depMap.AddTargetHeader(headerName)
    }
}
```
In `docx_handler.go` and `odt_handler.go`:
If `mode == domain.ModeDelete`:
Replace snippet with `DeletionMarker`.
If `depMap != nil`, call `depMap.PutOrdinal(originalSnippet)`.

In `pdf_handler.go`:
If `mode == domain.ModeDelete`:
Inject `DeletionMarker` (" [УДАЛЕНО] ") into stream instead of generic blackout text.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -mod=vendor ./pkg/formats -run "TestProcessCell_ModeDelete|TestExcel|TestCSV|TestDOCX"`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add pkg/formats/columns.go pkg/formats/columns_test.go pkg/formats/excel_handler.go pkg/formats/csv_handler.go pkg/formats/docx_handler.go pkg/formats/odt_handler.go pkg/formats/pdf_handler.go
git commit -m "feat(formats): support ModeDelete and recording CellMappings in handlers"
```

---

### Task 3: Header-Aware Resilient Restoration for Tables and Documents

**Files:**
- Modify: `pkg/formats/restore_excel.go:1-213`
- Modify: `pkg/formats/restore_csv.go:1-120`
- Modify: `pkg/formats/restore_ods.go:1-150`
- Modify: `pkg/formats/restore_docx.go:1-150`
- Modify: `internal/service/restorer/restorer.go:1-250`
- Test: `pkg/formats/restore_test.go`

**Interfaces:**
- Consumes: `*depmap.DepMap`
- Produces: Header-Aware `RestoreExcel`, `RestoreCSV`, `RestoreODS`, `RestoreDOCX` resilient to column insertions by AI.

- [ ] **Step 1: Write integration test for column insertion by AI**

Add test in `pkg/formats/restore_test.go`:
```go
func TestRestoreExcel_HeaderAwareColumnShift(t *testing.T) {
	// 1. Create original workbook with headers: ID, FIO, Phone
	// 2. Anonymize with ModeDelete and CreateKey=true -> yields anonymized.xlsx and key.depmap
	// 3. Simulate AI model: Insert 2 new columns between ID and FIO (e.g. AI_Score, AI_Category)
	// 4. Run RestoreExcel on the modified file using key.depmap
	// 5. Verify:
	//    - FIO and Phone are restored to original values
	//    - AI_Score and AI_Category remain intact with AI values
	//    - Restore count matches expected
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./pkg/formats -run TestRestoreExcel_HeaderAwareColumnShift`
Expected: FAIL (unsupported or corrupted column restoration).

- [ ] **Step 3: Implement Header-Aware resolution in restore_excel.go**

In `pkg/formats/restore_excel.go`:
```go
// 1. Scan header row for the sheet using FindHeaderRow
// 2. Build headerColMap := make(map[string]int)
// 3. If dm.Mode == "delete" || len(dm.CellMappings) > 0:
//    For each cell:
//      headerName := headers[colIdx]
//      if orig, ok := dm.GetCell(sheetName, headerName, rowNum); ok {
//         if cellVal == DeletionMarker {
//             applyChange(coord, orig)
//         }
//      }
// 4. If dm.Mode == "replace" && len(dm.TargetHeaders) > 0:
//    Only search for dm.Get(cellVal) in columns whose headerName is in dm.TargetHeaders!
//    Columns added by AI are skipped!
```
Apply the same Header-Aware logic in `restore_csv.go` and `restore_ods.go`.
In `restore_docx.go`:
```go
if len(dm.OrdinalMappings) > 0 {
    // Sequentially substitute [УДАЛЕНО] occurrences with ordinal elements
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -mod=vendor ./pkg/formats -run TestRestoreExcel_HeaderAwareColumnShift`
Expected: PASS

- [ ] **Step 5: Run full restore test suite**

Run: `go test -mod=vendor ./pkg/formats -run TestRestore`
Expected: PASS

- [ ] **Step 6: Commit changes**

```bash
git add pkg/formats/restore_excel.go pkg/formats/restore_csv.go pkg/formats/restore_ods.go pkg/formats/restore_docx.go pkg/formats/restore_test.go internal/service/restorer/restorer.go
git commit -m "feat(restore): implement header-aware column matching resilient to AI column shifts"
```

---

### Task 4: Processor, Preview Service, and Desktop Presentation Layer

**Files:**
- Modify: `internal/service/processor/processor.go:60-320`
- Modify: `internal/service/preview/preview.go:30-220`
- Modify: `internal/desktop/app.go:1-350`
- Modify: `pkg/audit/logger.go:1-100`
- Test: `internal/service/processor/processor_test.go`
- Test: `internal/service/preview/preview_test.go`

**Interfaces:**
- Consumes: `domain.ProcessOptions.Mode`
- Produces:
  - Wails presentation method signatures updated:
    - `ProcessFileWithSheets(opts domain.ProcessOptions) (*domain.ProcessResult, error)`
    - `GetSheetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int, mode string) ([]domain.PreviewRow, error)`
  - Audit record includes `Mode` field.

- [ ] **Step 1: Write test for PreviewService in ModeDelete**

Add test in `internal/service/preview/preview_test.go`:
Verify that when `mode == "delete"` is passed, `PreviewRow.After` contains `[УДАЛЕНО]` for detected columns.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./internal/service/preview -run TestGetSheetRows_ModeDelete`
Expected: FAIL

- [ ] **Step 3: Update Processor, PreviewService, App adapter, and Audit**

- In `processor.go`:
  Add `"mode": opts.Mode` to `options` map passed to format handlers.
  Pass `opts.Mode` to audit record.
- In `preview.go`:
  Add `mode string` parameter to `GetSheetRows`.
  If `mode == "delete"`, call `formats.ProcessCell(anon, ct, val, domain.ModeDelete, 30)`.
- In `app.go`:
  Expose updated `GetSheetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int, mode string)`.
  Ensure `ProcessFile` and `ProcessFileWithSheets` receive and honor `opts.Mode`.
- In `pkg/audit/logger.go`:
  Add `Mode string` to `AuditRecord`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -mod=vendor ./internal/service/... ./pkg/audit/...`
Expected: PASS

- [ ] **Step 5: Commit changes**

```bash
git add internal/service/processor/processor.go internal/service/preview/preview.go internal/desktop/app.go pkg/audit/logger.go internal/service/preview/preview_test.go
git commit -m "feat(service): wire ModeDelete into processor, preview, desktop app, and audit"
```

---

### Task 5: Svelte 5 Frontend UI and Dynamic Preview

**Files:**
- Modify: `frontend/src/lib/types/domain.ts:1-70`
- Modify: `frontend/src/lib/stores/appState.svelte.ts:1-120`
- Modify: `frontend/src/lib/api/wails.ts:1-150`
- Modify: `frontend/src/lib/components/depersonalize/TableView.svelte:1-250`
- Modify: `frontend/src/lib/components/depersonalize/TextPreview.svelte:1-150`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte:1-200`

**Interfaces:**
- Produces:
  - `appState.processingMode = $state<'replace' | 'delete'>('replace')`
  - `TableView.svelte`: clean Segmented Control `[ Замена | Удаление ]`
  - `SaveDialogModal.svelte`: passes `mode: appState.processingMode`, hides salt block when `delete` is active.

- [ ] **Step 1: Update TypeScript types and appState store**

In `frontend/src/lib/types/domain.ts`:
```ts
export type ProcessingMode = 'replace' | 'delete';

export interface ProcessOptions {
  inputPath: string;
  outputPath: string;
  mode?: ProcessingMode;
  salt?: string;
  disabledCols?: number[];
  disabledBySheet?: Record<string, number[]>;
  createKey?: boolean;
  keyPassword?: string;
  keyPath?: string;
  shuffleRows?: boolean;
}
```

In `frontend/src/lib/stores/appState.svelte.ts`:
```ts
processingMode = $state<ProcessingMode>('replace');

setProcessingMode(mode: ProcessingMode): void {
  this.processingMode = mode;
}
```

- [ ] **Step 2: Update Wails API facade**

In `frontend/src/lib/api/wails.ts`:
Update `GetSheetRows` call signature to include `mode?: string`.

- [ ] **Step 3: Add Segmented Control to TableView.svelte**

Above the table / next to the `[ До | После ]` switcher, add:
```svelte
<div class="flex items-center gap-2">
  <span class="text-xs font-medium text-[#78716C]">Режим:</span>
  <div class="flex items-center bg-[#F0EFEB] p-0.5 border border-[#E8E7E3] text-xs">
    <button
      type="button"
      onclick={() => appState.setProcessingMode('replace')}
      class={appState.processingMode === 'replace'
        ? 'px-2.5 py-1 bg-[#1C1917] text-white font-medium shadow-xs'
        : 'px-2.5 py-1 text-[#78716C] hover:text-[#1C1917]'}
    >
      Замена
    </button>
    <button
      type="button"
      onclick={() => appState.setProcessingMode('delete')}
      class={appState.processingMode === 'delete'
        ? 'px-2.5 py-1 bg-[#1C1917] text-white font-medium shadow-xs'
        : 'px-2.5 py-1 text-[#78716C] hover:text-[#1C1917]'}
    >
      Удаление
    </button>
  </div>
</div>
```
When `appState.processingMode === 'delete'`, dynamically render `[УДАЛЕНО]` in table cell preview for detected columns, and reload preview rows with `mode`.

- [ ] **Step 4: Update SaveDialogModal.svelte**

In `SaveDialogModal.svelte`:
- Pass `mode: appState.processingMode` in `ProcessFileWithSheets` call.
- Conditionally render salt block:
```svelte
{#if appState.processingMode === 'replace'}
  <!-- Salt input container -->
{/if}
```

- [ ] **Step 5: Run frontend typechecks and build**

Run: `npm --prefix frontend run check`
Expected: 0 errors
Run: `npm --prefix frontend run build`
Expected: Build succeeds

- [ ] **Step 6: Commit changes**

```bash
git add frontend/src/lib/types/domain.ts frontend/src/lib/stores/appState.svelte.ts frontend/src/lib/api/wails.ts frontend/src/lib/components/depersonalize/TableView.svelte frontend/src/lib/components/depersonalize/TextPreview.svelte frontend/src/lib/components/depersonalize/SaveDialogModal.svelte
git commit -m "feat(ui): add mode switcher [Замена | Удаление], preview updates and salt hiding"
```

---

### Task 6: Documentation, Version Bump, and Build Verification

**Files:**
- Modify: `internal/version/version.go:5` (bump to `1.7.0`)
- Modify: `frontend/package.json:3` (bump to `"1.7.0"`)
- Modify: `Makefile:4` (bump to `1.7.0`)
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`

- [ ] **Step 1: Bump version in all 3 files**

In `internal/version/version.go`: `const Version = "1.7.0"`
In `frontend/package.json`: `"version": "1.7.0"`
In `Makefile`: `VERSION = 1.7.0`

- [ ] **Step 2: Update CHANGELOG.md**

Add entry for `## [1.7.0] - 2026-10-09`:
- `### Добавлено`:
  - Режим удаления персональных данных (`ModeDelete`) с подстановкой маркера `[УДАЛЕНО]` для всех 8 поддерживаемых форматов.
  - Header-Aware механизм сопоставления колонок в ключе `.depmap`, обеспечивающий устойчивое восстановление при добавлении новых столбцов внешними нейросетями (LLM).
  - Переключатель режима `[ Замена | Удаление ]` в интерфейсе с динамическим обновлением предпросмотра.
- `### Изменено`:
  - В модальном окне сохранения блок соли скрывается при активном режиме удаления.
  - Журнал аудита `audit.jsonl` фиксирует режим выполнения операции (`mode`).

- [ ] **Step 3: Update documentation**

Update `docs/USER_GUIDE.md`, `docs/ARCHITECTURE.md`, and `README.md` to reflect `1.7.0` and describe the new Deletion Mode and Header-Aware restoration.

- [ ] **Step 4: Run full test suite and frontend checks**

Run: `go test -mod=vendor ./...`
Expected: All tests PASS
Run: `npm --prefix frontend run check`
Expected: 0 errors
Run: `npm --prefix frontend run build`
Expected: Succeeded

- [ ] **Step 5: Build platform binaries into bin/**

Run: `make build-macos` (or Universal binary)
Verify that `bin/` contains fresh executable.

- [ ] **Step 6: Commit all documentation and build changes**

```bash
git add internal/version/version.go frontend/package.json Makefile CHANGELOG.md docs/USER_GUIDE.md docs/ARCHITECTURE.md README.md
git commit -m "chore(release): bump version to 1.7.0 and update docs and changelog"
```
