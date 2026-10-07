# План реализации: Соответствие 152-ФЗ РФ и Приказу Роскомнадзора № 140 (Версия 1.6.0)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать соответствие актуальным требованиям 152-ФЗ и Приказа Роскомнадзора от 19.06.2025 № 140 (ликвидация хардкодных мастер-паролей, безопасная случайная соль, журнал учета действий по обезличиванию, перемешивание строк, абстракция криптопровайдеров, обновление документации и бамп версии до 1.6.0) при строгом сохранении чистого интерфейса в стиле Notion/Linear.

**Architecture:** Система расширяется новым пакетом `pkg/audit` для локального неизменяемого журналирования операций, интерфейсом `CipherProvider` в `pkg/depmap` с поддержкой безопасной автогенерации паролей, алгоритмом перемешивания строк (shuffling) в `pkg/formats`, а также интеграцией в Go-сервисы и Wails IPC без утяжеления Svelte 5 интерфейса.

**Tech Stack:** Go 1.23, Wails v2, Svelte 5 (Runes), Tailwind v4, AES-256-GCM, PBKDF2-HMAC-SHA256.

**Spec:** `docs/superpowers/specs/2026-10-08-152fz-compliance-and-security-design.md`

## Global Constraints

- 100% Air-Gapped: никаких внешних HTTP/DNS-запросов, внешних CDN или сетевых портов наружу.
- Все зависимости Go завендорены (`vendor/`, флаг `-mod=vendor`).
- UI: минималистичный дизайн Notion/Linear, без длинных пояснений в скобках и визуального мусора.
- Обратная совместимость: ранее созданные контейнеры `.depmap` должны открываться штатно.
- Обязательное прохождение: `go test ./...`, `npm --prefix frontend run check`, `npm --prefix frontend run build`.
- Актуализация документации: замена Приказа № 996 на Приказ Роскомнадзора от 19.06.2025 № 140.

---

### Task 1: Безопасность ключей `.depmap` и архитектура криптопровайдера

**Files:**
- Create: `pkg/depmap/provider.go`
- Modify: `pkg/depmap/depmap.go`
- Test: `pkg/depmap/depmap_test.go`
- Test: `pkg/depmap/provider_test.go`

**Interfaces:**
- Produces:
  ```go
  type CipherProvider interface {
      Name() string
      Encrypt(plaintext []byte, password string) (salt, nonce, ciphertext []byte, err error)
      Decrypt(salt, nonce, ciphertext []byte, password string) (plaintext []byte, err error)
  }
  func GenerateRandomPassword(length int) (string, error)
  ```
- Consumes: `crypto/rand`, `crypto/aes`, `crypto/cipher`, `depmap.DerivePBKDF2Key`

- [ ] **Step 1: Написать падающий тест для `GenerateRandomPassword` и генерации безопасного пароля**

```go
// in pkg/depmap/depmap_test.go
func TestGenerateRandomPassword(t *testing.T) {
	pwd1, err := GenerateRandomPassword(16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pwd1) != 16 {
		t.Fatalf("expected length 16, got %d", len(pwd1))
	}
	pwd2, _ := GenerateRandomPassword(16)
	if pwd1 == pwd2 {
		t.Fatalf("passwords should be unique")
	}
}
```

- [ ] **Step 2: Запустить тест и убедиться, что он падает**

Run: `go test -mod=vendor ./pkg/depmap -run TestGenerateRandomPassword`
Expected: FAIL (функция не объявлена).

- [ ] **Step 3: Реализовать `provider.go` и обновить `depmap.go`**
  - Создать `CipherProvider` (реализация `AES256GCMProvider`).
  - Добавить `GenerateRandomPassword(length int) (string, error)`.
  - В `SaveFile(path string, password string)`: если пароль пустой, возвращать ошибку или генерировать пароль и возвращать его наружу через расширенный метод `SaveFileWithAutoPassword(path string, password string) (effectivePassword string, err error)`.
  - В `LoadFile(path string, password string)`: сначала пробовать переданный пароль, и ТОЛЬКО для легаси-файлов при пустом пароле пробовать `DefaultMasterSecret`.

- [ ] **Step 4: Прогнать тесты и убедиться в успешном прохождении**

Run: `go test -mod=vendor ./pkg/depmap`
Expected: PASS.

- [ ] **Step 5: Зафиксировать изменения в git**

```bash
git add pkg/depmap/
git commit -m "feat(depmap): add cipher provider and secure random password generation"
```

---

### Task 2: Случайная соль по умолчанию и исключение статичного fallback

**Files:**
- Modify: `internal/service/processor/processor.go`
- Modify: `internal/domain/domain.go`
- Test: `internal/service/processor/processor_test.go`

**Interfaces:**
- Consumes: `anonymizer.GenerateSalt(16)`
- Produces: `EffectivePassword string` в `domain.ProcessResult`

- [ ] **Step 1: Написать тест на случайную соль при пустом значении**

В `internal/service/processor/processor_test.go`:
```go
func TestProcessor_GeneratesRandomSaltWhenEmpty(t *testing.T) {
	// Проверяет, что при opts.Salt == "" генерируется случайная 32-hex соль, а не "152fz-secure-salt"
}
```

- [ ] **Step 2: Запустить тест и убедиться в падении**

Run: `go test -mod=vendor ./internal/service/processor -run TestProcessor_GeneratesRandomSaltWhenEmpty`

- [ ] **Step 3: Обновить логику генерации соли и пароля в `processor.go`**
  - Если `strings.TrimSpace(opts.Salt) == ""`: вызывать `anonymizer.GenerateSalt(16)`.
  - Если `opts.CreateKey` и `strings.TrimSpace(opts.KeyPassword) == ""`: вызывать `depmap.GenerateRandomPassword(16)`, сохранять контейнер с этим паролем и возвращать его в `domain.ProcessResult.EffectivePassword` и `KeyPasswordGenerated = true`.

- [ ] **Step 4: Запустить тесты процессора**

Run: `go test -mod=vendor ./internal/service/processor/...`
Expected: PASS.

- [ ] **Step 5: Зафиксировать изменения в git**

```bash
git add internal/service/processor/ internal/domain/
git commit -m "feat(processor): enforce crypto random salt and handle auto-generated key password"
```

---

### Task 3: Модуль локального журнала аудита (`pkg/audit`) по Приказу РКН № 140

**Files:**
- Create: `pkg/audit/audit.go`
- Create: `pkg/audit/audit_test.go`
- Modify: `internal/service/processor/processor.go`
- Modify: `internal/service/restorer/restorer.go`
- Modify: `internal/desktop/app.go`
- Modify: `internal/desktop/dialogs.go`

**Interfaces:**
- Produces:
  ```go
  type AuditRecord struct {
      Timestamp    string   `json:"timestamp"`
      Username     string   `json:"username"`
      Hostname     string   `json:"hostname"`
      Operation    string   `json:"operation"` // "depersonalize" | "restore"
      SourceFile   string   `json:"source_file"`
      SourceHash   string   `json:"source_hash"`
      ResultFile   string   `json:"result_file"`
      ResultHash   string   `json:"result_hash"`
      RecordsCount int      `json:"records_count"`
      Entities     []string `json:"entities,omitempty"`
      KeyCreated   bool     `json:"key_created"`
      Status       string   `json:"status"` // "success" | "canceled" | "error"
      Error        string   `json:"error,omitempty"`
  }
  type AuditLogger struct { ... }
  func NewLogger(path string) (*AuditLogger, error)
  func (l *AuditLogger) Log(r AuditRecord) error
  func (l *AuditLogger) ReadRecent(limit int) ([]AuditRecord, error)
  ```

- [ ] **Step 1: Написать тесты для `pkg/audit`**

Тест создания записей, проверки прав доступа файла (`0600`), чтения последних N записей, вычисления хэшей файлов.

- [ ] **Step 2: Запустить тесты и убедиться в падении**

Run: `go test -mod=vendor ./pkg/audit`
Expected: FAIL (пакет отсутствует).

- [ ] **Step 3: Реализовать `pkg/audit/audit.go`**
  - Поддержка безопасного создания каталога `~/.dep_service` с правами `0700`.
  - Запись в `audit.jsonl` в режиме append с синхронизацией `Sync()`.
  - Метод быстрого вычисления SHA-256 хэша файла без загрузки всего файла в RAM (`io.Copy` через `bufio.Reader` в `sha256.New()`).

- [ ] **Step 4: Интегрировать аудит в `Processor`, `Restorer` и `DesktopApp`**
  - В `Processor.Process`: запись аудита при успехе, ошибке или отмене.
  - В `Restorer.Restore`: запись аудита операции восстановления.
  - В `DesktopApp`: методы Wails:
    - `GetAuditRecords(limit int) ([]audit.AuditRecord, error)`
    - `ExportAuditLog(destPath string) error`

- [ ] **Step 5: Прогнать тесты аудита и desktop-приложения**

Run: `go test -mod=vendor ./pkg/audit ./internal/desktop/...`
Expected: PASS.

- [ ] **Step 6: Зафиксировать изменения в git**

```bash
git add pkg/audit/ internal/service/ internal/desktop/
git commit -m "feat(audit): implement 152-FZ Order 140 compliance audit logger"
```

---

### Task 4: Метод перемешивания строк (Shuffling)

**Files:**
- Modify: `internal/domain/domain.go`
- Modify: `pkg/formats/excel_handler.go`
- Modify: `pkg/formats/csv_handler.go`
- Modify: `internal/service/processor/processor.go`
- Test: `pkg/formats/formats_test.go` / `excel_test.go`

**Interfaces:**
- Produces: `ShuffleRows bool` в `domain.ProcessOptions` и поддержка в хэндлерах форматов.

- [ ] **Step 1: Написать тест для перемешивания строк данных**

Проверить, что заголовок таблицы остается на месте (строка 0), а порядок строк данных переставляется детерминированно в зависимости от соли.

- [ ] **Step 2: Запустить тест и убедиться в падении**

Run: `go test -mod=vendor ./pkg/formats -run TestShuffleRows`

- [ ] **Step 3: Реализовать опцию `shuffle_rows` в `excel_handler.go` и `csv_handler.go`**
  - Если флаг `shuffle_rows` истинен, строки данных перед записью перемешиваются алгоритмом Фишера-Йетса с сидом от соли `anonymizer`.
  - Для `csv_handler`: буферизация записей и перемешивание.
  - Для `excel_handler`: применение перемешивания к строкам.

- [ ] **Step 4: Запустить тесты форматов**

Run: `go test -mod=vendor ./pkg/formats/...`
Expected: PASS.

- [ ] **Step 5: Зафиксировать изменения в git**

```bash
git add pkg/formats/ internal/domain/
git commit -m "feat(formats): add Order 140 compliant row shuffling method"
```

---

### Task 5: Чистый интерфейс Svelte 5 (Notion/Linear Style)

**Files:**
- Modify: `frontend/src/lib/types/domain.ts`
- Modify: `frontend/src/lib/api/wails.ts`
- Modify: `frontend/src/lib/stores/appState.svelte.ts`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte`
- Modify: `frontend/src/lib/components/common/SuccessModal.svelte`
- Modify: `frontend/src/lib/components/layout/Header.svelte`
- Create: `frontend/src/lib/components/common/AuditModal.svelte`

- [ ] **Step 1: Обновить TypeScript-типы и IPC вызовы**
  - Добавить `effectivePassword?: string; keyPasswordGenerated?: boolean;` в `ProcessResult`.
  - Добавить методы `GetAuditRecords(limit: number)` и `ExportAuditLog(destPath: string)`.

- [ ] **Step 2: Обновить `SaveDialogModal.svelte`**
  - При открытии модалки генерировать случайную соль через `crypto.getRandomValues`.
  - Добавить компактный переключатель: `Перемешать строки` (без громоздких текстов в скобках).
  - Поле пароля: плейсхолдер `Автогенерация при пустом поле`.

- [ ] **Step 3: Обновить `SuccessModal.svelte`**
  - Если `result.keyPasswordGenerated` истинно: лаконичный блок:
    ```svelte
    Пароль ключа: <code>{result.effectivePassword}</code> [📋 Скопировать]
    ```

- [ ] **Step 4: Добавить компактную кнопку «Журнал» в `Header.svelte` и компонент `AuditModal.svelte`**
  - Лаконичная модалка со списком последних операций (Дата, Файл, Записей, Статус) и кнопкой экспорта `.jsonl`.
  - Никакого визуального шума.

- [ ] **Step 5: Проверить типы и сборку фронтенда**

Run: `npm --prefix frontend run check && npm --prefix frontend run build`
Expected: PASS без ошибок TypeScript и Svelte.

- [ ] **Step 6: Зафиксировать изменения фронтенда в git**

```bash
git add frontend/
git commit -m "feat(ui): update clean Notion-style save dialog, success password display, and audit log viewer"
```

---

### Task 6: Актуализация документации, CHANGELOG и бамп версии до 1.6.0

**Files:**
- Modify: `internal/version/version.go` (`1.6.0`)
- Modify: `frontend/package.json` (`"version": "1.6.0"`)
- Modify: `Makefile` (`VERSION = 1.6.0`)
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Обновить версию в едином источнике правды и сопутствующих файлах**
  - `const Version = "1.6.0"` в `internal/version/version.go`.
  - `"version": "1.6.0"` в `frontend/package.json`.
  - `VERSION = 1.6.0` в `Makefile`.

- [ ] **Step 2: Актуализировать документацию**
  - Заменить все упоминания Приказа Роскомнадзора № 996 на **Приказ Роскомнадзора от 19.06.2025 № 140**.
  - Добавить описание подсистемы аудита `pkg/audit`, перемешивания строк, автогенерации надежных паролей и рекомендаций по раздельному хранению ключей деобезличивания.

- [ ] **Step 3: Заполнить `CHANGELOG.md` для версии 1.6.0**
  - Разделы `### Добавлено`, `### Изменено`, `### Исправлено`.

- [ ] **Step 4: Прогнать все тесты проекта**

Run: `go test -mod=vendor ./...`
Expected: ALL PASS.

- [ ] **Step 5: Зафиксировать документацию и версию в git**

```bash
git add internal/version/ frontend/package.json Makefile CHANGELOG.md docs/ README.md AGENTS.md
git commit -m "chore(release): bump version to 1.6.0 and update 152-FZ Order 140 documentation"
```

---

### Task 7: Сборка исполняемых бинарников (Build Artifacts)

**Files:**
- Output: `bin/depersonalizer` (macOS Universal)
- Output: `bin/depersonalizer-windows-amd64.exe` (Windows x86_64)

- [ ] **Step 1: Собрать бинарник macOS**

Run: `make build-macos`
Expected: Создание `bin/depersonalizer` (Universal binary).

- [ ] **Step 2: Собрать бинарник Windows**

Run: `make build-windows`
Expected: Создание `bin/depersonalizer-windows-amd64.exe`.

- [ ] **Step 3: Проверить собранные бинарники**

Run: `ls -lh bin/`
Expected: Наличие свежих бинарников версии 1.6.0.
