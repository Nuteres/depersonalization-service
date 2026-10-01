# Внедрение версионирования (v1.1.0), CHANGELOG и актуализация документации Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать простое и надежное версионирование приложения на Go, зафиксировать текущий релиз 1.1.0, создать `CHANGELOG.md` и полностью актуализировать всю документацию проекта.

**Architecture:** Единый источник правды в `internal/version/version.go`, проброс версии через `GetAppInfo()` в интерфейс, синхронизация версий в `package.json` и `Makefile`, структурированный `CHANGELOG.md` по стандарту Keep a Changelog и обновление документации (`USER_GUIDE.md`, `ARCHITECTURE.md`, `README.md`).

**Tech Stack:** Go 1.23, Wails v2, Svelte 5, Markdown.

**Spec:** `docs/superpowers/specs/2026-10-02-versioning-and-docs-design.md`

## Global Constraints
- Никаких внешних зависимостей или сетевых запросов (100% Air-Gapped).
- Тесты Go должны проходить без ошибок (`go test ./...`).
- Проверка типов Svelte/TypeScript (`npm --prefix frontend run check`) должна проходить с 0 ошибок.
- Терминология: использовать «ключ шифрования» вместо «ключ целостности (соль)».

---

### Task 1: Создание Go-пакета версий и интеграция в Wails DesktopApp

**Files:**
- Create: `internal/version/version.go`
- Modify: `internal/desktop/app.go:74-82`
- Test: `internal/desktop/app_test.go:28-48`

**Interfaces:**
- Consumes: `version.Version`
- Produces: `DesktopApp.GetAppInfo() map[string]string` возвращает `{"version": "v1.1.0", "os": ..., "arch": ...}`

- [ ] **Step 1: Написать тест на версию в `internal/desktop/app_test.go`**
Проверить, что `GetAppInfo()["version"]` равен `"v1.1.0"`.

- [ ] **Step 2: Запустить тест и убедиться, что он падает**
Run: `go test -v ./internal/desktop -run TestGetAppInfo`
Expected: FAIL (ожидается `v1.1.0`, получено `v1.0`)

- [ ] **Step 3: Создать `internal/version/version.go` и обновить `internal/desktop/app.go`**
В `internal/version/version.go`:
```go
package version

// Version — текущая версия приложения по SemVer.
const Version = "1.1.0"
```
В `internal/desktop/app.go`:
Использовать `"version": "v" + version.Version`.

- [ ] **Step 4: Запустить тесты и убедиться, что они проходят**
Run: `go test -v ./internal/desktop -run TestGetAppInfo`
Expected: PASS

---

### Task 2: Синхронизация версий в `frontend/package.json` и `Makefile`

**Files:**
- Modify: `frontend/package.json:3`
- Modify: `Makefile:2`

**Interfaces:**
- Consumes: `1.1.0`
- Produces: согласованы файлы конфигурации сборки

- [ ] **Step 1: Обновить версию в `frontend/package.json`**
Заменить `"version": "1.0.0"` на `"version": "1.1.0"`.

- [ ] **Step 2: Обновить версию в `Makefile`**
Заменить `VERSION = 1.0.0` на `VERSION = 1.1.0`.

- [ ] **Step 3: Проверить сборку фронтенда**
Run: `npm --prefix frontend run check && npm --prefix frontend run build`
Expected: 0 errors, build succeeds.

---

### Task 3: Создание `CHANGELOG.md`

**Files:**
- Create: `CHANGELOG.md`

**Interfaces:**
- Производит общепринятую спецификацию изменений Keep a Changelog 1.0.0 с историей версий `1.1.0` и `1.0.0`.

- [ ] **Step 1: Создать `CHANGELOG.md`**
Зафиксировать изменения:
- `## [1.1.0] - 2026-10-02`
  - Добавлено: ресайзер колонок таблицы, динамический бесконечный скролл, кнопки копирования путей в проводнике.
  - Изменено: переход на Svelte 5 Runes, очистка подвалов предпросмотра, редизайн модального окна успеха (ликвидация зелёного на зелёном), замена «ключ целостности (соль)» на «ключ шифрования».
  - Исправлено: сбивание колонок таблицы при первом открытии (`table-layout: fixed`), предупреждение при восстановлении данных.
- `## [1.0.0] - 2026-09-30`
  - Первоначальный стабильный релиз (8 форматов, AES-256-GCM, двусторонняя деперсонализация).

---

### Task 4: Актуализация руководства пользователя `docs/USER_GUIDE.md`

**Files:**
- Modify: `docs/USER_GUIDE.md`

- [ ] **Step 1: Обновить терминологию и описание новых функций в `docs/USER_GUIDE.md`**
  - Заменить устаревшие термины «соль хеширования» / «ключ целостности» на «ключ шифрования» / «случайный ключ».
  - Добавить инструкции по интерактивному изменению ширины колонок (перетаскивание разделителя в шапке).
  - Описать динамическую бесконечную подгрузку строк предпросмотра при прокрутке.
  - Описать обновленный экран успешного сохранения (кнопки копирования путей, переход к файлам).

---

### Task 5: Актуализация архитектурной документации `docs/ARCHITECTURE.md`

**Files:**
- Modify: `docs/ARCHITECTURE.md`

- [ ] **Step 1: Обновить `docs/ARCHITECTURE.md`**
  - Добавить раздел о подсистеме версионирования (`internal/version`).
  - Описать архитектуру Svelte 5 Runes (`$state`, `$state.raw`, `$derived`, `$effect`) в `tableState.svelte.ts`.
  - Задокументировать математику и логику работы ресайзеров колонок и бесконечного скролла в `TableView.svelte`.

---

### Task 6: Актуализация `README.md`

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Обновить `README.md`**
  - Добавить бейдж версии `v1.1.0`.
  - Добавить ссылку на `CHANGELOG.md`.
  - Обновить описание возможностей (ресайзеры колонок, бесконечный скролл, "ключ шифрования").

---

### Task 7: Комплексная верификация и сборка бинарников

**Files:**
- Build binaries: `bin/depersonalizer-darwin-universal`, `bin/depersonalizer-windows-amd64.exe`, `bin/depersonalizer-altlinux-amd64`

- [ ] **Step 1: Запустить тесты Go**
Run: `go test -v ./...`
Expected: Все тесты проходят.

- [ ] **Step 2: Запустить проверку фронтенда**
Run: `npm --prefix frontend run check && npm --prefix frontend run build`
Expected: 0 errors.

- [ ] **Step 3: Собрать бинарники под macOS, Windows и Alt Linux**
Run: `make build-all`
Expected: Исполняемые файлы в `bin/` успешно пересобраны с версией 1.1.0.
