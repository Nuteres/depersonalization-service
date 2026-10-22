# Legacy Code Cleanup and Architecture Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Очистить проект от выявленного мертвого и легаси-кода, устранить рассинхронизацию регулярных выражений ПДн между сервисами через вынос единого детектора в `pkg/detector`, исправить поддержку восстановления `.txt` в веб-интерфейсе, дедуплицировать методы запуска операций в `DesktopApp` и `SaveDialogModal`, актуализировать документацию и пересобрать бинарные дистрибутивы.

**Architecture:** Приложение построено на базе Go 1.23, Wails v2 и Svelte 5. В рамках плана:
1. Удаляются неиспользуемые артефакты (`KeyInfoCard.svelte`, `MaxInteractiveFileSize`, `ResolveColumnTypes`, `FormatHandler`, diff-экстракторы в `keymanager.go`).
2. В `pkg/detector` создается единый модуль распознавания ПДн в неструктурированном тексте (`TextMatch`, `FindTextMatches`), избавляющий `inspector.go`, `pdf_handler.go`, `docx_handler.go`, `odt_handler.go` и `txt_handler.go` от дублирования 14+ регулярок.
3. В `internal/desktop/app.go` устраняется дублирование между `ProcessFileWithSheets` и `ProcessFileWithOptions`.
4. В `frontend/src/lib/stores/restoreState.svelte.ts` восстанавливается поддержка формата `.txt`.
5. Версия инкрементируется до v1.14.2 с обновлением CHANGELOG.md и пересборкой бинарников.

**Tech Stack:** Go 1.23, Wails v2, Svelte 5, TypeScript, Tailwind v4, Swift (macOS bundler).

**Spec:** Запрос пользователя на анализ и рефакторинг легаси-кода без потери функциональности, требования регламентов `AGENTS.md`.

## Global Constraints

- 100% Air-Gapped: строгий запрет на внешние сетевые вызовы и телеметрию.
- Go Vendor: все зависимости завендорены в `vendor/`, компиляция исключительно с `-mod=vendor`.
- Обратная совместимость: запрещено изменять логику миграции устаревших путей аудита (`pkg/audit/audit.go`) и расшифровку контейнеров legacy v1 (`pkg/depmap/depmap.go`).
- Единый источник версии: `internal/version/version.go`, синхронизированный с `frontend/package.json` и `Makefile`.
- Обязательная сборка свежих исполняемых файлов в `bin/` (`make build-macos`, `make build-windows`).

## Review Focus

1. Проверка того, что удаление `KeyInfoCard.svelte` не ломает сборку фронтенда и рендеринг карточки ключа в `RestoreView.svelte`.
2. Проверка того, что добавление `txt` в `restoreState.svelte.ts` позволяет выбирать и восстанавливать файлы `.txt` без предупреждений.
3. Проверка того, что вынос регулярных выражений в `pkg/detector/text.go` дает идентичные или более точные результаты во всех существующих тестах DOCX, ODT, TXT, PDF и Inspector.
4. Проверка того, что удаление `ExtractMappings` из `KeyManager` не влияет на создание `.depmap` ключей для CSV, XLSX, DOCX, ODS, ODT, XLS, XLSB.
5. Проверка того, что упрощение `ProcessFileWithSheets` в `internal/desktop/app.go` сохраняет корректную отмену операций и работу прогресс-бара.

---

### Task 1: Очистка мертвого кода и исправление поддержки `.txt` в UI восстановления

**Files:**
- Delete: `frontend/src/lib/components/restore/KeyInfoCard.svelte`
- Modify: `frontend/src/lib/stores/restoreState.svelte.ts:5, 68`
- Modify: `internal/domain/models.go:61-63`
- Modify: `pkg/formats/columns.go:579-583`
- Modify: `pkg/formats/formats.go:12-16`
- Modify: `build/tools/bundle_macos.swift:141`

- [ ] **Step 1: Delete unused `KeyInfoCard.svelte`**
```bash
rm frontend/src/lib/components/restore/KeyInfoCard.svelte
```

- [ ] **Step 2: Add `txt` to `SUPPORTED_EXTS` in `restoreState.svelte.ts`**
Update line 5 to include `'txt'`, and update message on line 68: `(.xlsx, .ods, .csv, .docx, .odt, .txt)`.

- [ ] **Step 3: Remove dead Go symbols**
- Remove `MaxInteractiveFileSize` in `internal/domain/models.go`.
- Remove unreferenced `ResolveColumnTypes` wrapper in `pkg/formats/columns.go`.
- Remove obsolete `FormatHandler` interface in `pkg/formats/formats.go`.
- Update fallback version in `build/tools/bundle_macos.swift` from `"1.10.3"` to `"1.14.2"`.

- [ ] **Step 4: Verify frontend and backend compile**
Run: `npm --prefix frontend run check && go test -mod=vendor ./internal/domain ./pkg/formats`
Expected: PASS.

- [ ] **Step 5: Commit Task 1**
```bash
git add frontend/ internal/domain/ pkg/formats/ build/tools/
git commit -m "refactor: remove dead code and enable .txt in restore state"
```

---

### Task 2: Дедупликация логики выполнения в `internal/desktop/app.go` и `SaveDialogModal.svelte`

**Files:**
- Modify: `internal/desktop/app.go:190-264`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte:282-320`

- [ ] **Step 1: Refactor `ProcessFileWithSheets` in `internal/desktop/app.go`**
Make `ProcessFileWithSheets` an adapter that constructs `domain.ProcessOptions` and delegates to `a.ProcessFileWithOptions(opts)`. This eliminates 55 lines of duplicated mutex locking, context cancellation, and goroutine cleanup.

- [ ] **Step 2: Unify processing call in `SaveDialogModal.svelte`**
Use `ProcessFileWithOptions` for non-PDF/non-archive processing as well, removing redundant imports and branch divergence.

- [ ] **Step 3: Run desktop tests and frontend check**
Run: `npm --prefix frontend run check && go test -mod=vendor ./internal/desktop ./cmd/app`
Expected: PASS.

- [ ] **Step 4: Commit Task 2**
```bash
git add internal/desktop/app.go frontend/src/lib/components/depersonalize/SaveDialogModal.svelte
git commit -m "refactor(desktop): deduplicate ProcessFileWithSheets via ProcessFileWithOptions"
```

---

### Task 3: Удаление устаревшего diff-экстрактора в `KeyManager`

**Files:**
- Modify: `internal/service/keymanager/keymanager.go:73-213`
- Modify: `internal/service/keymanager/keymanager_test.go`
- Modify: `internal/service/processor/processor.go:473-488`

- [ ] **Step 1: Remove dead `ExtractMappings` fallback in `processor.go`**
In `internal/service/processor/processor.go:473-488`, remove the unreachable `else` block calling `p.km.ExtractMappings`. Since `opts.CreateKey` guarantees `directDepMap != nil`, simplify key saving to:
```go
savedPwd, err = directDepMap.SaveFileWithAutoPassword(keyPath, opts.KeyPassword)
```

- [ ] **Step 2: Remove diff methods from `keymanager.go` and obsolete tests**
Remove `ExtractMappings`, `extractCSVMappings`, and `extractExcelMappings` from `keymanager.go`.
Remove `TestKeyManager_ExtractMappings_CSV` and `TestKeyManager_ExtractMappings_Excel` from `keymanager_test.go`.

- [ ] **Step 3: Run keymanager and processor tests**
Run: `go test -mod=vendor -v ./internal/service/keymanager ./internal/service/processor`
Expected: PASS.

- [ ] **Step 4: Commit Task 3**
```bash
git add internal/service/keymanager/ internal/service/processor/
git commit -m "refactor(keymanager): eliminate legacy post-hoc diff mapping extractor"
```

---

### Task 4: Централизация регулярных выражений и поиска ПДн в `pkg/detector`

**Files:**
- Create: `pkg/detector/text.go`
- Create: `pkg/detector/text_test.go`
- Modify: `pkg/formats/docx_handler.go`
- Modify: `pkg/formats/odt_handler.go`
- Modify: `pkg/formats/txt_handler.go`
- Modify: `pkg/formats/pdf_handler.go`
- Modify: `internal/service/inspector/inspector.go`

- [ ] **Step 1: Create `pkg/detector/text.go`**
Define canonical Russian PII regexes and helper structures:
- `TextMatch { Start, End, Value, EntityType }`
- `FindTextMatches(text string) []TextMatch`
- Helpers for address trimming and validation, FIO initials validation, clean digits.

- [ ] **Step 2: Write unit tests in `pkg/detector/text_test.go`**
Cover FIO, SNILS, Passport, INN, Phone, Bank Card, Accounts, Dates, Addresses.

- [ ] **Step 3: Update `docx_handler.go`, `odt_handler.go`, `txt_handler.go`**
Replace local `findDocxSensitiveMatches` and local regex definitions with `detector.FindTextMatches` and `detector.TextMatch`.

- [ ] **Step 4: Update `pdf_handler.go` and `inspector.go`**
Remove duplicate `reSNILSFormatted`, `rePassport...`, `reINN...`, `rePhone...`, `reCard16`, `reFIO...` regexes in `pdf_handler.go` and `inspector.go`, routing text scanning through `detector.FindTextMatches` or sharing the package-level regexes.

- [ ] **Step 5: Run full test suite across all formats and services**
Run: `go test -mod=vendor -v ./pkg/detector ./pkg/formats ./internal/service/...`
Expected: ALL PASS.

- [ ] **Step 6: Commit Task 4**
```bash
git add pkg/detector/ pkg/formats/ internal/service/
git commit -m "refactor(detector): centralize unstructured text PII regexes and matcher"
```

---

### Task 5: Инкремент версии (v1.14.2), обновление документации и CHANGELOG.md

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`

- [ ] **Step 1: Bump version to 1.14.2**
Update `internal/version/version.go`, `frontend/package.json`, `Makefile`.

- [ ] **Step 2: Update `CHANGELOG.md`**
Add `## [1.14.2] - 2026-10-22` with `### Исправлено`, `### Изменено`, `### Удалено`.

- [ ] **Step 3: Update `docs/ARCHITECTURE.md` and `README.md`**
Reflect unified `pkg/detector/text.go` architecture and removal of legacy diff extraction.

- [ ] **Step 4: Commit Task 5**
```bash
git add internal/version/version.go frontend/package.json Makefile CHANGELOG.md docs/ARCHITECTURE.md README.md
git commit -m "chore(release): bump version to 1.14.2 and record refactoring in changelog"
```

---

### Task 6: Полный цикл верификации и сборка свежих исполняемых бинарных файлов (Build Artifacts)

**Files:**
- Build: `bin/depersonalizer-windows-amd64.exe`
- Build: `bin/depersonalizer-darwin-universal`
- Build: `bin/Depersonalizer.app`

- [ ] **Step 1: Frontend quality check and build**
Run: `npm --prefix frontend run check && npm --prefix frontend run build`
Expected: 0 errors, 0 warnings.

- [ ] **Step 2: Full Go test suite**
Run: `go test -mod=vendor -v ./...`
Expected: ALL PASS.

- [ ] **Step 3: Build Windows binary**
Run: `make build-windows`
Expected: `bin/depersonalizer-windows-amd64.exe` built.

- [ ] **Step 4: Build macOS Universal binary and .app bundle**
Run: `make build-macos`
Expected: `bin/depersonalizer-darwin-universal` and `bin/Depersonalizer.app` built.

- [ ] **Step 5: Clean up scratch directory and verify git status**
Ensure scratch scripts are removed and repository status is clean.
