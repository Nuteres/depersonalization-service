# Legacy Code Analysis and Cleanup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Провести аудит, удалить выявленный неиспользуемый и легаси-код из кодовой базы Go и фронтенда, синхронизировать устаревшие версии в сборочных скриптах, обновить документацию и пересобрать бинарные дистрибутивы под все поддерживаемые платформы с гарантией сохранности функциональности и стабильности.

**Architecture:** Приложение построено по модульной сервисной архитектуре (`internal/service/` + `pkg/formats` + `internal/desktop`). Ранее использовавшиеся или временные заготовки (`pkg/engine`, устаревшие регулярные выражения парсинга PDF, старые обёртки до поддержки мультистраничности, заброшенные отладочные файлы тестов) удаляются, а все вызовы и тесты верифицируются через полный цикл компиляции и тестирования (`go test ./...`, `npm run check`, `npm run build`).

**Tech Stack:** Go 1.23+, Wails v2, Svelte 5, TypeScript, Tailwind v4, Swift (macOS bundler), Docker (Alt Linux).

**Spec:** Запрос пользователя на анализ и удаление легаси-кода без ущерба функциональности и стабильности, правила версионирования и сборки из `AGENTS.md`.

## Global Constraints

- 100% Air-Gapped: запрещены внешние сетевые запросы и телеметрия.
- Go Vendor: все зависимости завендорены в `vendor/`, сборка строго с `-mod=vendor`.
- Криптографическая и функциональная обратная совместимость: не трогать логику миграции устаревших путей аудита (`pkg/audit/audit.go`) и расшифровки ключей legacy v1 (`pkg/depmap/depmap.go`).
- Единый источник версии: `internal/version/version.go`, синхронизированный с `frontend/package.json` и `Makefile`.
- Обязательная сборка свежих бинарников в `bin/` (`make build-windows`, `make build-macos`, `make build-altlinux`).

## Review Focus

1. Проверка того, что удаление `pkg/engine` не ломает сборку ни одного пакета сервисов или бинарников.
2. Проверка того, что удаление `getXLSXRows` в `internal/service/preview` не нарушает предварительный просмотр одностраничных и многостраничных таблиц (`GetSheetRows` и `GetTableRows` должны работать без изменений).
3. Проверка того, что удаление неиспользуемых regex (`rePage`, `reLen`, `reLit`) в `internal/service/inspector` и `unescapePDFString` в `pkg/formats/pdf_handler.go` не затрагивает извлечение текста и предпросмотр документов PDF.
4. Проверка того, что удаление `MatchResult` из `pkg/detector` не ломает детекторы сущностей.
5. Проверка того, что синхронизация версий в `build/tools/build_syso.go` и `build/tools/bundle_macos.swift` корректно генерирует ресурсы и манифесты без ошибок линковщика.

---

### Task 1: Удаление неиспользуемого пакета-заготовки `pkg/engine`

**Files:**
- Delete: `pkg/engine/engine.go`
- Delete: `pkg/engine/`

**Interfaces:**
- Consumes: None (0 imports across the codebase)
- Produces: Cleaned architecture without obsolete stub orchestrator

- [ ] **Step 1: Verify no dependencies import `pkg/engine`**
Run: `git grep "dep_service/pkg/engine"`
Expected: No Go imports found.

- [ ] **Step 2: Delete `pkg/engine/engine.go`**
Run: `rm -rf pkg/engine`

- [ ] **Step 3: Run Go tests to confirm zero breakage**
Run: `go test -mod=vendor ./...`
Expected: PASS across all packages.

- [ ] **Step 4: Commit**
```bash
git add pkg/engine
git commit -m "refactor: remove legacy unused pkg/engine stub"
```

---

### Task 2: Удаление неиспользуемого отладочного теста `cmd/app/dump_excel_test.go`

**Files:**
- Delete: `cmd/app/dump_excel_test.go`

**Interfaces:**
- Consumes: Hardcoded missing path `../../ШК_ПАО РТК_В2С.xlsx`
- Produces: Cleaned test suite without skipped local debug scripts

- [ ] **Step 1: Verify test content is obsolete debug script**
Verify: contains only `t.Skip("sample excel file not found in root")` and `fmt.Printf` row dumping.

- [ ] **Step 2: Remove `cmd/app/dump_excel_test.go`**
Run: `rm cmd/app/dump_excel_test.go`

- [ ] **Step 3: Run cmd/app tests**
Run: `go test -mod=vendor ./cmd/app`
Expected: PASS (`wails_app_test.go` passes).

- [ ] **Step 4: Commit**
```bash
git add cmd/app/dump_excel_test.go
git commit -m "test: remove obsolete manual dump_excel_test debug file"
```

---

### Task 3: Очистка мертвого кода в `internal/service/inspector`, `internal/service/preview`, `pkg/formats` и `pkg/detector`

**Files:**
- Modify: `internal/service/inspector/inspector.go:40-43`
- Modify: `internal/service/preview/preview.go:106-113`
- Modify: `pkg/formats/pdf_handler.go:2011-2020`
- Modify: `pkg/detector/detector.go:24-30`

**Interfaces:**
- Consumes: None
- Produces: Code without dead regexes, uncalled helpers, or unused stub structs

- [ ] **Step 1: Remove unused regexes `rePage`, `reLen`, `reLit` in `internal/service/inspector/inspector.go`**
Remove lines 40-42 (`rePage`, `reLen`, `reLit`).

- [ ] **Step 2: Remove obsolete `getXLSXRows` helper in `internal/service/preview/preview.go`**
Remove lines 106-112 (`func getXLSXRows`).

- [ ] **Step 3: Remove dead helper `unescapePDFString` in `pkg/formats/pdf_handler.go`**
Remove lines 2011-2019 (`func unescapePDFString`).

- [ ] **Step 4: Remove unused `MatchResult` struct in `pkg/detector/detector.go`**
Remove lines 24-29 (`type MatchResult struct`).

- [ ] **Step 5: Run tests across all affected packages**
Run: `go test -mod=vendor ./internal/service/inspector ./internal/service/preview ./pkg/formats ./pkg/detector`
Expected: PASS.

- [ ] **Step 6: Commit**
```bash
git add internal/service/inspector/inspector.go internal/service/preview/preview.go pkg/formats/pdf_handler.go pkg/detector/detector.go
git commit -m "refactor: eliminate dead unexported functions, regexes and stub structs"
```

---

### Task 4: Синхронизация версий в утилитах сборки Windows и macOS

**Files:**
- Modify: `build/tools/build_syso.go:90-95`
- Modify: `build/tools/bundle_macos.swift:119-123`

**Interfaces:**
- Consumes: `internal/version/version.go` ("1.10.1")
- Produces: Correct metadata in generated Windows PE manifest and macOS Info.plist

- [ ] **Step 1: Update version in `build/tools/build_syso.go`**
Update `assemblyIdentity version` to `1.10.1.0`.

- [ ] **Step 2: Update version in `build/tools/bundle_macos.swift`**
Update `CFBundleShortVersionString` and `CFBundleVersion` to `1.10.1`.

- [ ] **Step 3: Verify build_syso runs without error**
Run: `go run build/tools/build_syso.go`
Expected: "Successfully generated cmd/app/icon_windows_amd64.syso".

- [ ] **Step 4: Commit**
```bash
git add build/tools/build_syso.go build/tools/bundle_macos.swift cmd/app/icon_windows_amd64.syso
git commit -m "build: update embedded application version in Windows and macOS build tools"
```

---

### Task 5: Инкремент версии до v1.10.1, актуализация документации и CHANGELOG.md

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: SemVer PATCH bump for refactoring and dead code eradication
- Produces: Synchronized version 1.10.1 and updated documentation

- [ ] **Step 1: Bump version in `internal/version/version.go`, `frontend/package.json`, `Makefile`**
Set `1.10.1`.

- [ ] **Step 2: Update `CHANGELOG.md`**
Add section `## [1.10.1] - 2026-10-15` with `### Удалено` and `### Изменено`.

- [ ] **Step 3: Update `docs/ARCHITECTURE.md` and `README.md`**
Reflect removal of `pkg/engine` stub, bump version badges.

- [ ] **Step 4: Commit**
```bash
git add internal/version/version.go frontend/package.json Makefile CHANGELOG.md docs/ARCHITECTURE.md README.md
git commit -m "chore(release): bump version to 1.10.1 and record legacy cleanup in changelog"
```

---

### Task 6: Полный цикл валидации и сборка свежих исполняемых бинарных файлов (Build Artifacts)

**Files:**
- Build: `bin/depersonalizer-windows-amd64.exe`
- Build: `bin/depersonalizer-darwin-universal`
- Build: `bin/Depersonalizer.app`
- Build: `bin/depersonalizer-altlinux-amd64` (через Docker)

**Interfaces:**
- Consumes: Validated clean codebase
- Produces: Production binaries in `bin/`

- [ ] **Step 1: Frontend checks and build**
Run: `npm --prefix frontend run check && npm --prefix frontend run build`
Expected: 0 errors, 0 warnings, successful Vite build.

- [ ] **Step 2: Full Go test suite**
Run: `go test -mod=vendor -v ./...`
Expected: ALL PASS.

- [ ] **Step 3: Build Windows binary**
Run: `make build-windows`
Expected: `bin/depersonalizer-windows-amd64.exe` created.

- [ ] **Step 4: Build macOS Universal binary and .app bundle**
Run: `make build-macos`
Expected: `bin/depersonalizer-darwin-universal` and `bin/Depersonalizer.app` created.

- [ ] **Step 5: Build Alt Linux binary via Docker**
Run: `make build-altlinux`
Expected: `bin/depersonalizer-altlinux-amd64` created.

- [ ] **Step 6: Remove scratch analysis scripts**
Run: `rm -rf /Users/nuter/.gemini/antigravity/brain/319d6ce5-70e6-4770-88cb-25b68c61b212/scratch/find_dead_*.go /Users/nuter/.gemini/antigravity/brain/319d6ce5-70e6-4770-88cb-25b68c61b212/scratch/find_dead_*.mjs`

- [ ] **Step 7: Final git status check and verification**
Verify working directory is clean and binaries in `bin/` are up to date.
