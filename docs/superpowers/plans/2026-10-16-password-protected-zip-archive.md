# Password-Protected ZIP Archive Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement password-protected universal ZIP archive creation during file depersonalization, saving both the clean document and the encrypted archive, with flexible manual or auto-generated password support.

**Architecture:** A new backend package `pkg/ziputil` wraps `github.com/yeka/zip` with PKWARE standard encryption (`zip.StandardEncryption`) to guarantee cross-platform compatibility (Windows Explorer, macOS, Linux). `Processor.Process` creates the archive alongside the depersonalized file when requested. Svelte 5 frontend provides an interactive toggle, password controls (generate, show/hide, copy), and displays the archive path and password in the completion modal.

**Tech Stack:** Go 1.25, `github.com/yeka/zip`, Wails v2, Svelte 5 (runes `$state`, `$derived`), Tailwind CSS v4.

**Spec:** `docs/superpowers/specs/2026-10-16-password-protected-zip-archive-design.md`

## Global Constraints

- 100% Air-Gapped: Zero external HTTP/WebSocket network calls, zero CDN requests, zero telemetry.
- Go Vendor: All Go dependencies must be vendored into `vendor/` and built with `-mod=vendor`.
- SemVer: Version bumped to `1.12.0` across `internal/version/version.go`, `frontend/package.json`, and `Makefile`.
- Encryption Algorithm: Strictly PKWARE Standard Encryption (`zip.StandardEncryption` / ZipCrypto) for universal out-of-the-box extraction in Windows Explorer and macOS.
- File Preservation: Keep both the uncompressed depersonalized file and the password-protected `.zip` archive on disk.
- Archive Contents: Archive must contain strictly the depersonalized file; the `.depmap` key file is never placed in the archive.
- Verification Gates: `npm --prefix frontend run check`, `npm --prefix frontend run build`, `go test ./...`, and binary build in `bin/`.

## Review Focus

- Empty or invalid password handling: when `archivePassword` is empty, auto-generate 16-character secure password; when supplied, validate ASCII and min length 4.
- Non-ASCII / Cyrillic password input: reject in `ValidateArchivePassword` to prevent encoding corruptions in Windows CP1251/CP866 Explorer.
- Context cancellation during processing: ensure temporary `.tmp` or partial `.zip` files are cleaned up safely.
- Output path collision / defaulting: verify default `.zip` name replaces extension with `.zip` or appends `.zip` without clobbering source.
- Frontend copy-to-clipboard: verify archive password can be copied to clipboard from both `SaveDialogModal` and `SuccessModal`.

---

### Task 1: Create `pkg/ziputil` Package and Vendor Dependencies

**Files:**
- Create: `pkg/ziputil/ziputil.go`
- Create: `pkg/ziputil/ziputil_test.go`
- Modify: `go.mod`, `go.sum`, `vendor/*`

**Interfaces:**
- Produces:
  - `ziputil.CreatePasswordZip(srcFilePath, destZipPath, password string) error`
  - `ziputil.ValidateArchivePassword(password string) error`

- [ ] **Step 1: Write the failing test for `pkg/ziputil`**

```go
package ziputil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dep_service/pkg/ziputil"
	"github.com/yeka/zip"
)

func TestCreatePasswordZip_SuccessAndExtract(t *testing.T) {
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "report_anonymized.xlsx")
	content := []byte("dummy excel content for zip testing")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("failed to write src file: %v", err)
	}

	zipFile := filepath.Join(tmpDir, "report_anonymized.zip")
	password := "Secret123"

	err := ziputil.CreatePasswordZip(srcFile, zipFile, password)
	if err != nil {
		t.Fatalf("CreatePasswordZip failed: %v", err)
	}

	r, err := zip.OpenReader(zipFile)
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	defer r.Close()

	if len(r.File) != 1 {
		t.Fatalf("expected 1 file in zip, got %d", len(r.File))
	}

	f := r.File[0]
	if f.Name != "report_anonymized.xlsx" {
		t.Errorf("expected entry name report_anonymized.xlsx, got %s", f.Name)
	}
	if !f.IsEncrypted() {
		t.Errorf("expected file to be encrypted")
	}

	f.SetPassword(password)
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("failed to open encrypted entry: %v", err)
	}
	defer rc.Close()

	buf := make([]byte, len(content))
	n, _ := rc.Read(buf)
	if string(buf[:n]) != string(content) {
		t.Errorf("expected content %q, got %q", string(content), string(buf[:n]))
	}
}

func TestValidateArchivePassword(t *testing.T) {
	if err := ziputil.ValidateArchivePassword("abc"); err == nil {
		t.Errorf("expected error for short password")
	}
	if err := ziputil.ValidateArchivePassword("Пароль123"); err == nil {
		t.Errorf("expected error for cyrillic password")
	}
	if err := ziputil.ValidateArchivePassword("pass with space"); err == nil {
		t.Errorf("expected error for password with space")
	}
	if err := ziputil.ValidateArchivePassword("ValidPass123!"); err != nil {
		t.Errorf("expected valid password, got error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/ziputil/...`  
Expected: FAIL (package or dependency not found)

- [ ] **Step 3: Add dependency, vendor it, and implement `pkg/ziputil/ziputil.go`**

1. Run `go get github.com/yeka/zip@v0.0.0-20231116150916-03d6312748a9`
2. Run `go mod tidy && go mod vendor`
3. Implement `pkg/ziputil/ziputil.go`:
   - `ValidateArchivePassword(password string) error`: checks length >= 4, only printable ASCII characters (33 <= b <= 126), no spaces.
   - `CreatePasswordZip(srcFilePath, destZipPath, password string) error`:
     - Creates temporary file in target directory: `destZipPath + ".tmp"`.
     - Opens source file for reading.
     - Creates zip writer via `zip.NewWriter(tmpFile)`.
     - Calls `w.Encrypt(filepath.Base(srcFilePath), password, zip.StandardEncryption)`.
     - Copies content with `io.Copy`.
     - Closes zip writer and file.
     - Atomically renames temporary file to `destZipPath`.
     - On error, cleans up temporary file.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -mod=vendor ./pkg/ziputil/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum vendor/ pkg/ziputil/
git commit -m "feat(ziputil): add universal password-protected ZIP archive creator"
```

---

### Task 2: Extend Domain Models and Processor Integration

**Files:**
- Modify: `internal/domain/options.go`
- Modify: `internal/domain/results.go`
- Modify: `internal/service/processor/processor.go:400-475`
- Test: `internal/service/processor/processor_test.go`

**Interfaces:**
- Consumes: `ziputil.CreatePasswordZip`, `ziputil.ValidateArchivePassword`, `depmap.GenerateRandomPassword`
- Produces: `domain.ProcessOptions.CreateArchive`, `domain.ProcessResult.ArchivePath`, `domain.ProcessResult.ArchivePassword`

- [ ] **Step 1: Write the failing test for archiving in processor**

Add test to `internal/service/processor/processor_test.go`:
```go
func TestProcessor_CreateArchive(t *testing.T) {
    // Tests that when opts.CreateArchive is true, an encrypted zip is created next to output,
    // both files exist, and result contains archivePath and archivePassword.
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./internal/service/processor/... -run TestProcessor_CreateArchive`  
Expected: FAIL (fields not defined on ProcessOptions/ProcessResult)

- [ ] **Step 3: Update domain options and results**

In `internal/domain/options.go`:
```go
type ProcessOptions struct {
    ...
    CreateArchive   bool   `json:"createArchive"`
    ArchivePassword string `json:"archivePassword"`
    ArchivePath     string `json:"archivePath,omitempty"`
}
```

In `internal/domain/results.go`:
```go
type ProcessResult struct {
    ...
    ArchivePath              string `json:"archivePath,omitempty"`
    ArchivePassword          string `json:"archivePassword,omitempty"`
    ArchivePasswordGenerated bool   `json:"archivePasswordGenerated,omitempty"`
}
```

- [ ] **Step 4: Integrate archive creation in `processor.go`**

In `internal/service/processor/processor.go` inside `Process()` right after key file handling:
```go
var effectiveArchivePassword string
var archivePasswordGenerated bool
var finalArchivePath string

if opts.CreateArchive && err == nil {
    if ctx.Err() != nil {
        cleanup()
        return nil, ctx.Err()
    }

    archivePath := strings.TrimSpace(opts.ArchivePath)
    if archivePath == "" {
        archivePath = strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".zip"
    }

    pwd := strings.TrimSpace(opts.ArchivePassword)
    if pwd == "" {
        genPwd, err := depmap.GenerateRandomPassword(16)
        if err != nil {
            cleanup()
            return nil, fmt.Errorf("failed to generate archive password: %w", err)
        }
        effectiveArchivePassword = genPwd
        archivePasswordGenerated = true
    } else {
        if err := ziputil.ValidateArchivePassword(pwd); err != nil {
            cleanup()
            return nil, fmt.Errorf("invalid archive password: %w", err)
        }
        effectiveArchivePassword = pwd
    }

    if err := ziputil.CreatePasswordZip(outPath, archivePath, effectiveArchivePassword); err != nil {
        cleanup()
        return nil, fmt.Errorf("failed to create password-protected archive: %w", err)
    }
    finalArchivePath = archivePath
}
```
Populate `finalArchivePath`, `effectiveArchivePassword`, and `archivePasswordGenerated` in `res = &domain.ProcessResult{...}`.

- [ ] **Step 5: Run tests to verify it passes**

Run: `go test -mod=vendor ./internal/service/processor/... -v`  
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/domain/ internal/service/processor/
git commit -m "feat(processor): integrate password-protected ZIP creation in processing pipeline"
```

---

### Task 3: Expose Desktop App Handlers in `internal/desktop/app.go`

**Files:**
- Modify: `internal/desktop/app.go`
- Test: `internal/desktop/app_test.go`

**Interfaces:**
- Consumes: `runtime.SaveFileDialog`, `depmap.GenerateRandomPassword`, `ziputil.ValidateArchivePassword`
- Produces:
  - `DesktopApp.SaveArchiveFileDialog(defaultFilename string) (string, error)`
  - `DesktopApp.GenerateArchivePassword() (string, error)`
  - `DesktopApp.ValidateArchivePassword(password string) (bool, string)`
  - Updated `ProcessFileWithSheets` to pass archive parameters.

- [ ] **Step 1: Write test for desktop app archive helper methods**

Test `GenerateArchivePassword` and `ValidateArchivePassword` in `internal/desktop/app_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -mod=vendor ./internal/desktop/... -run TestArchivePassword`  
Expected: FAIL (methods undefined)

- [ ] **Step 3: Implement desktop handlers in `internal/desktop/app.go`**

Implement:
1. `SaveArchiveFileDialog(defaultFilename string) (string, error)`: opens Wails SaveFileDialog with filter `ZIP-архивы (*.zip)`.
2. `GenerateArchivePassword() (string, error)`: returns `depmap.GenerateRandomPassword(16)`.
3. `ValidateArchivePassword(password string) (bool, string)`: validates via `ziputil.ValidateArchivePassword`.
4. Update `ProcessFileWithOptions` and `ProcessFileWithSheets` to support archive fields.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -mod=vendor ./internal/desktop/... -v`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/desktop/
git commit -m "feat(desktop): expose archive file dialog and password generation to frontend"
```

---

### Task 4: Svelte 5 Frontend Integration (`SaveDialogModal` and `SuccessModal`)

**Files:**
- Modify: `frontend/src/lib/types/domain.ts`
- Modify: `frontend/src/lib/api/wails.ts`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte`
- Modify: `frontend/src/lib/components/common/SuccessModal.svelte`

**Interfaces:**
- Consumes: `SaveArchiveFileDialog`, `GenerateArchivePassword`, `ValidateArchivePassword`
- Produces: UI controls in `SaveDialogModal` and display card in `SuccessModal`

- [ ] **Step 1: Update TypeScript domain types and Wails API wrappers**

In `frontend/src/lib/types/domain.ts`:
- Add `createArchive?: boolean; archivePassword?: string; archivePath?: string;` to `ProcessOptions`.
- Add `archivePath?: string; archivePassword?: string; archivePasswordGenerated?: boolean;` to `ProcessResult` / `SuccessResult`.

In `frontend/src/lib/api/wails.ts`:
- Export `SaveArchiveFileDialog`, `GenerateArchivePassword`, `ValidateArchivePassword`.

- [ ] **Step 2: Update `SaveDialogModal.svelte`**

- Add state runes:
  - `createArchive = $state(false)`
  - `archivePassword = $state('')`
  - `showArchivePassword = $state(false)`
  - `archivePath = $state('')`
  - `archivePasswordError = $state('')`
- Implement handlers:
  - `handleGenerateArchivePassword()`: calls `GenerateArchivePassword()`, sets `archivePassword`, clears error.
  - `handleSelectArchivePath()`: calls `SaveArchiveFileDialog()`.
  - `handleCopyArchivePassword()`: copies password to clipboard.
- Validate `archivePassword` when non-empty: ASCII characters, min length 4, no spaces.
- Add UI block:
  - Checkbox «📦 Упаковать в ZIP-архив с паролем» with badge «Для e-mail / СБ».
  - Collapsible container when `createArchive` is true:
    - Input for archive password with eye button, 🎲 generate button, 📋 copy button.
    - Path selector with «Обзор...» button.
    - Helpful hint: «Если оставить пустым — пароль будет сгенерирован автоматически».
- Pass `createArchive`, `archivePassword`, `archivePath` into `ProcessFileWithOptions` and `ProcessFileWithSheets`.

- [ ] **Step 3: Update `SuccessModal.svelte`**

- Add reactive state for showing archive password (`showArchivePassword = $state(false)`), copied state (`copiedArchivePassword = $state(false)`), and copied archive path.
- Add handler `copyArchivePassword()` and `copyArchivePath()`.
- Add conditional card when `appState.successResult?.archivePath` is present:
  - Archive file path with «Открыть папку» and «Копировать путь» buttons.
  - Archive password banner:
    - Password value (or `••••••••` if hidden).
    - Toggle eye button.
    - Prominent «Скопировать пароль» button with toast notification.
    - Badge «Сообщите пароль получателю архива».

- [ ] **Step 4: Run frontend typecheck and build**

Run: `npm --prefix frontend run check && npm --prefix frontend run build`  
Expected: PASS with 0 errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/
git commit -m "feat(ui): add password-protected archive settings and success modal controls"
```

---

### Task 5: Documentation and Semantic Version Bump (v1.12.0)

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`

- [ ] **Step 1: Bump version to 1.12.0**

- `internal/version/version.go`: `const Version = "1.12.0"`
- `frontend/package.json`: `"version": "1.12.0"`
- `Makefile`: `VERSION = 1.12.0`

- [ ] **Step 2: Update `CHANGELOG.md`**

Add section `## [1.12.0] - 2026-10-16`:
- `### Добавлено`:
  - Функция создания защищённого паролем универсального ZIP-архива для безопасной отправки деперсонализированных файлов по электронной почте.
  - Гибкая настройка пароля архива (ручной ввод от 4 символов ASCII или автогенерация 16-значного криптостойкого пароля).
  - Карточка архива с паролем и кнопкой быстрого копирования в окне успешного завершения обработки.
  - Новый пакет `pkg/ziputil` на базе `github.com/yeka/zip` (PKWARE Standard Encryption).

- [ ] **Step 3: Update `docs/USER_GUIDE.md`, `docs/ARCHITECTURE.md`, `README.md`**

- `docs/USER_GUIDE.md`: пошаговый сценарий отправки по почте с использованием архивации с паролем.
- `docs/ARCHITECTURE.md`: описание слоя `pkg/ziputil` и обновлённого конвейера `Processor`.
- `README.md`: упоминание защиты и архивации с паролем в списке возможностей.

- [ ] **Step 4: Commit**

```bash
git add internal/version/version.go frontend/package.json Makefile CHANGELOG.md docs/ README.md
git commit -m "docs(release): bump version to 1.12.0 and document password-protected archiving"
```

---

### Task 6: Comprehensive Verification and Production Builds

**Files:**
- Output: `bin/dep_service` (macOS), `bin/dep_service.exe` (Windows)

- [ ] **Step 1: Run complete Go test suite**

Run: `go test -mod=vendor ./...`  
Expected: PASS for all packages.

- [ ] **Step 2: Run frontend verification**

Run: `npm --prefix frontend run check && npm --prefix frontend run build`  
Expected: PASS.

- [ ] **Step 3: Build fresh macOS and Windows binaries into `bin/`**

Run:
```bash
make build-macos
make build-windows
```
Expected:
- `bin/dep_service` built successfully.
- `bin/dep_service.exe` built successfully.

- [ ] **Step 4: Commit build outputs or artifacts**

```bash
git add bin/
git commit -m "build: compile fresh v1.12.0 binaries for macOS and Windows"
```
