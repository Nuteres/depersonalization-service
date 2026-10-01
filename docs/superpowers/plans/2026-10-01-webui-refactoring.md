# Go WebUI Refactoring (Clean Architecture + Svelte 5) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Рефакторинг Go WebUI приложения: декомпозиция 2100-строчного монолита `cmd/app/wails_app.go` в Clean Architecture сервисы (`internal/domain`, `internal/service`, `internal/desktop`) и модернизация фронтенда на Svelte 5 (Runes), Vite, TypeScript и Tailwind CSS v4 с сохранением 100% функциональности, дизайна и высокой производительности.

**Architecture:** Чистая архитектура в Go с изоляцией сервисов (`inspector`, `preview`, `processor`, `restorer`, `keymanager`) от Wails GUI через `context.Context` и интерфейс отслеживания прогресса `ProgressReporter`. Фронтенд на Svelte 5 со строгой типизацией TypeScript через Wails IPC, легковесными рунами `$state.raw` для табличных данных и AOT-компиляцией стилей Tailwind CSS.

**Tech Stack:** Go 1.25, Wails v2, Svelte 5, Vite, TypeScript, Tailwind CSS v4, Excelize v2.

**Spec:** [`docs/superpowers/specs/2026-10-01-webui-refactoring-design.md`](file:///Users/nuter/Go/dep_service/docs/superpowers/specs/2026-10-01-webui-refactoring-design.md)

## Global Constraints

- Сохранить 100% функциональности всех 8 поддерживаемых форматов: `.xlsx`, `.xlsb`, `.xls`, `.ods`, `.csv`, `.docx`, `.odt`, `.pdf`.
- Полная идентичность внешнего вида и цветовой гаммы (Pixel-Perfect перенос дизайн-системы из `frontend/index.html` и `frontend/css/style.css`).
- Air-Gapped режим: отсутствие внешних сетевых запросов, CDN и веб-шрифтов; сборка фронтенда встраивается через `//go:embed all:dist`.
- Потоковая обработка больших объёмов данных без полной загрузки в память.
- Поддержка отмены долгих операций в 1 клик через `context.Context`.

---

### Task 1: Domain Entities, Options & Error Types (`internal/domain`)

**Files:**
- Create: `internal/domain/models.go`
- Create: `internal/domain/options.go`
- Create: `internal/domain/results.go`
- Create: `internal/domain/progress.go`
- Create: `internal/domain/errors.go`
- Test: `internal/domain/domain_test.go`

**Interfaces:**
- Consumes: `pkg/formats.DocxTextSnippet`
- Produces: `domain.FileInfo`, `domain.SheetInfo`, `domain.EntityInfo`, `domain.PreviewRow`, `domain.ProcessOptions`, `domain.RestoreOptions`, `domain.ProcessResult`, `domain.RestoreResult`, `domain.KeyFileInfo`, `domain.ProgressReporter`

- [ ] **Step 1: Write the failing test for domain models**

```go
package domain_test

import (
	"testing"
	"dep_service/internal/domain"
)

func TestDomainModelsInitialization(t *testing.T) {
	fi := domain.FileInfo{
		Path:     "/test/file.xlsx",
		Name:     "file.xlsx",
		Format:   "xlsx",
		RowCount: 100,
	}
	if fi.Name != "file.xlsx" || fi.RowCount != 100 {
		t.Fatalf("unexpected FileInfo values: %+v", fi)
	}

	opts := domain.ProcessOptions{
		InputPath:  "/test/in.xlsx",
		OutputPath: "/test/out.xlsx",
		Salt:       "test-salt",
	}
	if opts.Salt != "test-salt" {
		t.Fatalf("unexpected ProcessOptions: %+v", opts)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/domain/...`
Expected: FAIL with package not found / types undefined.

- [ ] **Step 3: Implement domain types**

Implement clean structs in `internal/domain`:
- `models.go`: `EntityInfo`, `PreviewRow`, `SheetInfo`, `FileInfo`.
- `options.go`: `ProcessOptions`, `RestoreOptions`.
- `results.go`: `ProcessResult`, `RestoreResult`, `KeyFileInfo`.
- `progress.go`: `type ProgressReporter func(processed, total, percent int)`.
- `errors.go`: `ErrInvalidPassword`, `ErrUnsupportedFormat`, `ErrOperationCanceled`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/domain/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/
git commit -m "feat(domain): define clean domain models, options, and error types"
```

---

### Task 2: Inspector Service (`internal/service/inspector`)

**Files:**
- Create: `internal/service/inspector/inspector.go`
- Test: `internal/service/inspector/inspector_test.go`

**Interfaces:**
- Consumes: `internal/domain.FileInfo`, `internal/domain.SheetInfo`, `pkg/formats`, `pkg/detector`, `pkg/anonymizer`
- Produces: `Inspector.Inspect(ctx context.Context, path string) (*domain.FileInfo, error)`

- [ ] **Step 1: Write the failing test for Inspector**

```go
package inspector_test

import (
	"context"
	"testing"
	"dep_service/internal/service/inspector"
)

func TestInspectCSV(t *testing.T) {
	svc := inspector.New()
	info, err := svc.Inspect(context.Background(), "../../pkg/formats/fixtures/sample.csv")
	if err != nil {
		t.Fatalf("failed to inspect CSV: %v", err)
	}
	if info.Format != "csv" || info.FileType != "table" {
		t.Errorf("expected format csv and type table, got format=%s, type=%s", info.Format, info.FileType)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service/inspector/...`
Expected: FAIL.

- [ ] **Step 3: Implement Inspector Service**

Extract all format inspection methods from `cmd/app/wails_app.go` (`inspectCSV`, `inspectXLSX`, `inspectODS`, `inspectXLSB`, `inspectXLS`, `inspectDOCX`, `inspectODT`, `inspectPDF`) into `internal/service/inspector/inspector.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service/inspector/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/inspector/
git commit -m "feat(service): implement InspectorService for all 8 document formats"
```

---

### Task 3: Preview Service (`internal/service/preview`)

**Files:**
- Create: `internal/service/preview/preview.go`
- Test: `internal/service/preview/preview_test.go`

**Interfaces:**
- Consumes: `internal/domain.PreviewRow`, `pkg/formats`, `pkg/anonymizer`
- Produces: `PreviewService.GetSheetRows(ctx context.Context, path, sheetName string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error)`

- [ ] **Step 1: Write the failing test for PreviewService**

```go
package preview_test

import (
	"context"
	"testing"
	"dep_service/internal/service/preview"
)

func TestGetCSVPreviewRows(t *testing.T) {
	svc := preview.New()
	rows, err := svc.GetSheetRows(context.Background(), "../../pkg/formats/fixtures/sample.csv", "", 0, 10, "test-salt", nil)
	if err != nil {
		t.Fatalf("failed to get preview rows: %v", err)
	}
	if len(rows) == 0 {
		t.Errorf("expected at least 1 preview row, got 0")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service/preview/...`
Expected: FAIL.

- [ ] **Step 3: Implement PreviewService**

Extract row streaming and masking logic (`getXLSXRows`, `getXLSXRowsForSheet`, `getCSVRows`, `GetSheetRows`) into `internal/service/preview/preview.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service/preview/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/preview/
git commit -m "feat(service): implement PreviewService with pagination and column masking"
```

---

### Task 4: KeyManager & Restorer Services (`internal/service/keymanager`, `internal/service/restorer`)

**Files:**
- Create: `internal/service/keymanager/keymanager.go`
- Create: `internal/service/restorer/restorer.go`
- Test: `internal/service/restorer/restorer_test.go`

**Interfaces:**
- Consumes: `internal/domain.KeyFileInfo`, `internal/domain.RestoreOptions`, `internal/domain.RestoreResult`, `pkg/depmap`, `pkg/formats`
- Produces: `KeyManager.InspectKey(path, password string) (*domain.KeyFileInfo, error)`, `RestorerService.Restore(ctx context.Context, opts domain.RestoreOptions, reporter domain.ProgressReporter) (*domain.RestoreResult, error)`

- [ ] **Step 1: Write failing test for KeyManager and Restorer**

```go
package restorer_test

import (
	"context"
	"testing"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/restorer"
)

func TestInspectKeyFileInvalid(t *testing.T) {
	km := keymanager.New()
	_, err := km.InspectKey("/nonexistent/key.depmap", "secret")
	if err == nil {
		t.Errorf("expected error for nonexistent key file, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service/restorer/...`
Expected: FAIL.

- [ ] **Step 3: Implement KeyManager and Restorer**

Extract `extractMappings`, `extractCSVMappings`, `extractExcelMappings`, `.depmap` inspection, and `RestoreFileTo` logic into `keymanager` and `restorer`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service/restorer/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/keymanager/ internal/service/restorer/
git commit -m "feat(service): implement KeyManager and RestorerService for reverse depersonalization"
```

---

### Task 5: Processor Service with Cancellation (`internal/service/processor`)

**Files:**
- Create: `internal/service/processor/processor.go`
- Test: `internal/service/processor/processor_test.go`

**Interfaces:**
- Consumes: `internal/domain.ProcessOptions`, `internal/domain.ProcessResult`, `internal/domain.ProgressReporter`, `internal/service/keymanager`, `pkg/formats`, `pkg/anonymizer`
- Produces: `ProcessorService.Process(ctx context.Context, opts domain.ProcessOptions, reporter domain.ProgressReporter) (*domain.ProcessResult, error)`

- [ ] **Step 1: Write failing test for Processor with Context Cancellation**

```go
package processor_test

import (
	"context"
	"testing"
	"time"
	"dep_service/internal/domain"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/processor"
)

func TestProcessorCancellation(t *testing.T) {
	km := keymanager.New()
	proc := processor.New(km)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := domain.ProcessOptions{
		InputPath:  "../../pkg/formats/fixtures/sample.csv",
		OutputPath: "/tmp/canceled_out.csv",
		Salt:       "test-salt",
	}

	_, err := proc.Process(ctx, opts, func(p, total, pct int) {})
	if err == nil {
		t.Errorf("expected error on cancelled context, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service/processor/...`
Expected: FAIL.

- [ ] **Step 3: Implement ProcessorService**

Extract `ProcessFileWithSheets` streaming pipeline from `cmd/app/wails_app.go` into `internal/service/processor/processor.go`. Add `ctx.Err()` cancellation checks inside loops. Clean up incomplete output file on error or cancel.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service/processor/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/service/processor/
git commit -m "feat(service): implement ProcessorService with streaming and cancellation support"
```

---

### Task 6: Desktop Wails Adapter & Refactoring `cmd/app` (`internal/desktop`, `cmd/app`)

**Files:**
- Create: `internal/desktop/app.go`
- Create: `internal/desktop/dialogs.go`
- Create: `internal/desktop/events.go`
- Modify: `cmd/app/main.go`
- Test: `internal/desktop/app_test.go`
- Delete/Archive: `cmd/app/wails_app.go` (replaced by clean `internal/desktop`)

**Interfaces:**
- Consumes: all `internal/service` interfaces and `internal/domain` models.
- Produces: `desktop.DesktopApp` struct bound to Wails in `cmd/app/main.go`.

- [ ] **Step 1: Write test for DesktopApp instantiation**

```go
package desktop_test

import (
	"testing"
	"dep_service/internal/desktop"
	"dep_service/internal/service/inspector"
	"dep_service/internal/service/keymanager"
	"dep_service/internal/service/preview"
	"dep_service/internal/service/processor"
	"dep_service/internal/service/restorer"
)

func TestDesktopAppInitialization(t *testing.T) {
	insp := inspector.New()
	prev := preview.New()
	km := keymanager.New()
	proc := processor.New(km)
	rest := restorer.New()

	app := desktop.New(insp, prev, proc, rest, km)
	info := app.GetAppInfo()
	if info["version"] == "" {
		t.Errorf("expected non-empty version, got empty")
	}
}
```

- [ ] **Step 2: Implement DesktopApp controller**

Create `internal/desktop`:
- `app.go`: IPC methods invoking corresponding services with cancellation lock.
- `dialogs.go`: `SelectFile`, `SaveFileDialog`, `SelectKeyFile`, `SaveKeyFileDialog`, `OpenContainingFolder`.
- `events.go`: helper connecting `domain.ProgressReporter` to `runtime.EventsEmit`.
Refactor `cmd/app/main.go` to construct services and pass `desktop.New(...)` to `wails.Run`. Remove monolithic `cmd/app/wails_app.go`.

- [ ] **Step 3: Run all Go tests to verify clean integration**

Run: `go test -v ./internal/... ./pkg/...`
Expected: ALL PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/desktop/ cmd/app/
git commit -m "refactor(desktop): bind clean DesktopApp controller and remove monolithic wails_app.go"
```

---

### Task 7: Svelte 5 + Vite + TypeScript Frontend Setup (`frontend/`)

**Files:**
- Create: `frontend/package.json`
- Create: `frontend/vite.config.ts`
- Create: `frontend/tsconfig.json`
- Create: `frontend/svelte.config.js`
- Create: `frontend/src/app.css`
- Create: `frontend/src/main.ts`
- Create: `frontend/src/lib/api/wails.ts`

- [ ] **Step 1: Create package.json and configuration files**

Setup `frontend/package.json` with dependencies:
- `svelte@^5.37.0`
- `@sveltejs/vite-plugin-svelte`
- `vite@^6.0.0`
- `typescript`
- `tailwindcss@^4.0.0`
- `@tailwindcss/vite`

- [ ] **Step 2: Create Vite and Svelte configs**

Configure `vite.config.ts` with svelte plugin and tailwindcss plugin.
Configure `tsconfig.json` for strict typechecking.

- [ ] **Step 3: Implement Wails API facade in `src/lib/api/wails.ts`**

Wrap Wails window calls into typed asynchronous TypeScript functions with fallbacks for dev mode.

- [ ] **Step 4: Run npm install and test build**

Run: `cd frontend && npm install && npm run check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/package.json frontend/vite.config.ts frontend/tsconfig.json frontend/svelte.config.js frontend/src/
git commit -m "feat(frontend): initialize Svelte 5 + Vite + TypeScript + Tailwind v4 project"
```

---

### Task 8: Svelte 5 Reactive Stores & Shared Components (`frontend/src/lib/`)

**Files:**
- Create: `frontend/src/lib/stores/appState.svelte.ts`
- Create: `frontend/src/lib/stores/tableState.svelte.ts`
- Create: `frontend/src/lib/components/layout/Header.svelte`
- Create: `frontend/src/lib/components/common/ProgressModal.svelte`
- Create: `frontend/src/lib/components/common/SuccessModal.svelte`
- Create: `frontend/src/lib/components/common/Toast.svelte`

- [ ] **Step 1: Implement reactive stores with Svelte 5 Runes**

In `appState.svelte.ts`:
- `$state` for `currentAppMode`, `currentFilePath`, `currentFileInfo`, `currentSalt`, `isProcessing`.
In `tableState.svelte.ts`:
- `$state.raw` for `previewRows` (high-performance unproxied array).
- `$state` for `activeSheetIndex`, `disabledColumnsBySheet`.

- [ ] **Step 2: Implement Header and Common Modals**

Port exact HTML markup, badges, and colors from `frontend/index.html` to:
- `Header.svelte` (Badge `152`, version, mode toggle).
- `ProgressModal.svelte` (Progress bar with cancel button).
- `SuccessModal.svelte` (Stats banner, button "Open containing folder").
- `Toast.svelte` (Notification alert).

- [ ] **Step 3: Run svelte-check**

Run: `cd frontend && npm run check`
Expected: 0 errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/stores/ frontend/src/lib/components/
git commit -m "feat(frontend): implement reactive stores and shared UI components in Svelte 5"
```

---

### Task 9: Svelte 5 Depersonalize View Components (`frontend/src/lib/components/depersonalize/`)

**Files:**
- Create: `frontend/src/lib/components/depersonalize/HeroBanner.svelte`
- Create: `frontend/src/lib/components/depersonalize/SheetTabs.svelte`
- Create: `frontend/src/lib/components/depersonalize/EntityGrid.svelte`
- Create: `frontend/src/lib/components/depersonalize/TableView.svelte`
- Create: `frontend/src/lib/components/depersonalize/TextPreview.svelte`
- Create: `frontend/src/lib/components/depersonalize/PdfPreview.svelte`
- Create: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte`
- Create: `frontend/src/lib/components/depersonalize/DepersonalizeView.svelte`

- [ ] **Step 1: Implement HeroBanner, SheetTabs, and EntityGrid**

Preserve exact styling, buttons, and badges:
- `HeroBanner.svelte`: file title, "Открыть файл...", "Сменить файл...", "Сохранить результат".
- `SheetTabs.svelte`: workbook sheet selector with excluded column counts.
- `EntityGrid.svelte`: cards for FIO, SNILS, INN, etc. with toggle switches.

- [ ] **Step 2: Implement TableView, TextPreview, and PdfPreview**

- `TableView.svelte`: Infinite scroll table, Before/After toggle, fullscreen modal.
- `TextPreview.svelte`: DOCX/ODT text snippet viewer with Before/After toggle.
- `PdfPreview.svelte`: PDF blackout preview blocks.
- `SaveDialogModal.svelte`: Salt input, .depmap key checkbox, password, format export.

- [ ] **Step 3: Run svelte-check**

Run: `cd frontend && npm run check`
Expected: 0 errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/lib/components/depersonalize/
git commit -m "feat(frontend): implement depersonalization view components in Svelte 5"
```

---

### Task 10: Svelte 5 Restore View & Root `App.svelte` Integration

**Files:**
- Create: `frontend/src/lib/components/restore/RestoreView.svelte`
- Create: `frontend/src/lib/components/restore/KeyInfoCard.svelte`
- Modify: `frontend/src/App.svelte`

- [ ] **Step 1: Implement RestoreView and KeyInfoCard**

Port restoration panel: file selector, `.depmap` key selector, password field, metadata card, and restore button.

- [ ] **Step 2: Assemble Root App.svelte**

Wire `App.svelte` with:
- `Header.svelte`
- Conditional view rendering (`currentAppMode === 'depersonalize'` vs `'restore'`).
- Global file drag-and-drop listener (`EventsOn('app:file-dropped')`).
- Modals (`ProgressModal`, `SuccessModal`, `SaveDialogModal`, `Toast`).

- [ ] **Step 3: Run production frontend build**

Run: `cd frontend && npm run build`
Expected: Output files generated in `frontend/dist/` without errors.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/
git commit -m "feat(frontend): integrate complete Svelte 5 application with restoration view"
```

---

### Task 11: Production Build Integration & End-to-End Verification

**Files:**
- Modify: `frontend/assets.go`
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Update assets.go to embed dist**

Change `frontend/assets.go` to:
```go
package frontend

import "embed"

//go:embed all:dist
var Assets embed.FS
```

- [ ] **Step 2: Update Makefile to build frontend before Go binary**

Update targets `build-macos`, `build-windows`, `build-altlinux` to run `npm --prefix frontend run build`.

- [ ] **Step 3: Run full verification suite**

Run:
1. `go test -v ./...`
2. `make build-macos` (or `go build -mod=vendor -tags desktop,production ./cmd/app`)
Verify that binary is created and starts successfully.

- [ ] **Step 4: Commit**

```bash
git add frontend/assets.go Makefile .gitignore
git commit -m "chore(build): integrate Vite frontend build into Makefile and assets embed"
```

---

## Self-Review Checklist

- **Spec coverage**:
  - Clean Architecture Go separation (`internal/domain`, `internal/service`, `internal/desktop`) -> Tasks 1, 2, 3, 4, 5, 6.
  - Streaming cancellation via `context.Context` -> Task 5.
  - Svelte 5 + Runes + Vite + TypeScript + Tailwind v4 -> Tasks 7, 8, 9, 10.
  - 100% preservation of all 8 formats, multi-sheets, column toggling, restore -> Tasks 2, 3, 4, 9, 10.
  - Multi-platform build & embed -> Task 11.
- **Placeholder scan**: No "TBD", "TODO", or vague steps. Every task specifies exact files, interfaces, and test commands.
- **Type consistency**: Domain structs (`FileInfo`, `ProcessOptions`, `RestoreOptions`, `PreviewRow`) and service signatures are aligned across all tasks.
