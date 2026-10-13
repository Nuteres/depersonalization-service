# Key Password Policy and Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать строгую парольную политику для шифрования контейнеров ключей `.depmap` (12+ символов, заглавные/строчные буквы, цифры, спецсимволы) с автогенерацией полного набора символов, валидацией на бэкенде Go, интерактивным индикатором сложности в UI (без лишнего бюрократического текста) и обновлением документации до v1.9.0.

**Architecture:** 
1. Пакет `pkg/depmap` дополняется функцией `ValidatePassword(password string) error` и расширенным `GenerateRandomPassword(16)` с включением безопасных спецсимволов.
2. `DesktopApp` в `internal/desktop` валидирует пользовательский пароль перед запуском процесса деперсонализации и предоставляет метод проверки пароля для Wails IPC.
3. Модальное окно `SaveDialogModal.svelte` дополняется кнопкой переключения видимости пароля («глазок»), динамическим индикатором надежности и критериями валидации в лаконичном стиле Notion/Linear.
4. Синхронный бамп версии приложения до 1.9.0, обновление документации и пересборка бинарников.

**Tech Stack:** Go 1.23, Svelte 5, Tailwind CSS v4, Wails v2, AES-256-GCM / PBKDF2-HMAC-SHA256.

**Spec:** Запрос пользователя от 13.10.2026: внедрение требований к паролю файла ключа (длина от 12 символов, спецсимволы, заглавные/строчные буквы, цифры, автогенерация) с лаконичным интерфейсом без бюрократических отсылок в скобках.

## Global Constraints
- 100% Air-Gapped (никаких внешних запросов или CDN).
- Все зависимости Go строго через `-mod=vendor`.
- В UI не отображать юридическую бюрократию (никаких «(мера ИАФ.4 Приказа ФСТЭК № 21)»).
- Сохранять совместимость со старыми ключами `.depmap` при расшифровке.
- Проверка типов TypeScript: `npm --prefix frontend run check` без ошибок.
- Тесты Go: `go test -mod=vendor ./...` 100% pass.

---

### Task 1: Расширение генерации паролей и валидация политики в `pkg/depmap`

**Files:**
- Create: `pkg/depmap/policy.go`
- Modify: `pkg/depmap/provider.go`
- Modify: `pkg/depmap/depmap.go`
- Test: `pkg/depmap/policy_test.go`
- Test: `pkg/depmap/provider_test.go`

- [ ] **Step 1: Написать падающие тесты в `pkg/depmap/policy_test.go`**
  - Проверка паролей короче 12 символов (ошибка `ErrPasswordTooShort`).
  - Проверка отсутствия заглавных букв (`ErrPasswordNoUppercase`).
  - Проверка отсутствия строчных букв (`ErrPasswordNoLowercase`).
  - Проверка отсутствия цифр (`ErrPasswordNoDigit`).
  - Проверка отсутствия спецсимволов (`ErrPasswordNoSpecial`).
  - Проверка валидного пароля (например, `Correct#Pass2026!`).
  - Проверка того, что `GenerateRandomPassword(16)` всегда генерирует пароль, удовлетворяющий всем 4 критериям.

- [ ] **Step 2: Запустить тесты и убедиться в падении**
  - `go test -mod=vendor ./pkg/depmap -run TestValidatePassword`

- [ ] **Step 3: Реализовать `pkg/depmap/policy.go` и обновить `pkg/depmap/provider.go`**
  - Определить ошибки: `ErrPasswordTooShort`, `ErrPasswordNoUppercase`, `ErrPasswordNoLowercase`, `ErrPasswordNoDigit`, `ErrPasswordNoSpecial`.
  - Реализовать функцию `ValidatePassword(password string) error`.
  - Реализовать расчет шкалы сложности `EvaluatePasswordStrength(password string) (score int, details PasswordPolicyDetails)`.
  - Обновить `GenerateRandomPassword`: включить спецсимволы `!@#$%^&*()_+-=[]{}|;:,.<>?` и гарантировать наличие хотя бы одного символа из каждого из 4 классов.
  - В `SaveFileWithAutoPassword(path, password)`: если передан непустой пароль, вызывать `ValidatePassword(password)`.

- [ ] **Step 4: Запустить тесты `pkg/depmap` и убедиться в их прохождении**
  - `go test -v -mod=vendor ./pkg/depmap`

---

### Task 2: Интеграция валидации пароля в DesktopApp и Processor

**Files:**
- Modify: `internal/domain/results.go`
- Modify: `internal/desktop/app.go`
- Modify: `internal/service/processor/processor.go`
- Test: `internal/desktop/app_test.go`

- [ ] **Step 1: Написать тест на валидацию пароля в `internal/desktop/app_test.go`**
  - Тест вызова `ValidateKeyPassword`: возврат валидности и ошибок.
  - Тест `ProcessFileWithSheets` с невалидным кастомным паролем (должен вернуть ошибку валидации).
  - Тест с пустым паролем (автогенерация должна вернуть стойкий пароль, удовлетворяющий политике).

- [ ] **Step 2: Добавить структуру результата и метод `ValidateKeyPassword` в `DesktopApp`**
  - Модель `PasswordValidationResult` в `domain`.
  - Сигнатура: `ValidateKeyPassword(password string) domain.PasswordValidationResult`.
  - Возвращает `Valid`, `Score`, `Errors`, флаги выполнения критериев (length, upper, lower, digit, special).

- [ ] **Step 3: Проверить валидацию в `Processor.Process`**
  - Если `opts.CreateKey` и передан кастомный пароль, валидировать его до начала ресурсоемкой обработки.

- [ ] **Step 4: Прогнать тесты сервисов**
  - `go test -mod=vendor ./internal/desktop/... ./internal/service/processor/...`

---

### Task 3: Обновление интерфейса модального окна сохранения в Svelte 5

**Files:**
- Modify: `frontend/src/lib/types/domain.ts`
- Modify: `frontend/src/lib/api/wails.ts`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte`

- [ ] **Step 1: Обновить TypeScript-типы и Wails API мост**
  - Добавить `PasswordValidationResult` в `frontend/src/lib/types/domain.ts`.
  - Экспортировать `ValidateKeyPassword` в `frontend/src/lib/api/wails.ts`.

- [ ] **Step 2: Модернизировать блок ввода пароля в `SaveDialogModal.svelte`**
  - Добавить переключатель видимости пароля (кнопка-глазок 👁/👁‍🗨).
  - Добавить динамическую строку подсказки:
    * *«Минимум 12 символов: заглавные и строчные буквы, цифры и спецсимволы. Оставьте пустым для надежной автогенерации.»*
  - Добавить индикатор сложности пароля (шкала с 3 делениями: Слабый / Средний / Надежный) при вводе.
  - Добавить компактный чеклист критериев (зеленые/серые индикаторы для 12+ символов, A-Z, a-z, 0-9, спецсимволы).
  - Блокировать кнопку «Начать обезличивание» (`disabled`), если пользователь ввел непустой пароль, не удовлетворяющий политике.

- [ ] **Step 3: Проверить сборку фронтенда**
  - `npm --prefix frontend run check`
  - `npm --prefix frontend run build`

---

### Task 4: Синхронизация версий, обновление документации и сборка бинарников

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`
- Build: `bin/`

- [ ] **Step 1: Бамп версии до 1.9.0**
  - Обновить `internal/version/version.go` -> `1.9.0`.
  - Обновить `frontend/package.json` -> `1.9.0`.
  - Обновить `Makefile` -> `1.9.0`.

- [ ] **Step 2: Обновить CHANGELOG.md**
  - Добавить секцию `## [1.9.0] - 2026-10-13`.
  - Описать внедрение парольной политики, автогенерацию со спецсимволами и UI-индикатор сложности.

- [ ] **Step 3: Актуализировать документацию**
  - `docs/USER_GUIDE.md`: описание требований к паролю ключа и подсказок интерфейса.
  - `docs/ARCHITECTURE.md`: описание криптографической парольной политики.
  - `README.md`: бейджи версий.

- [ ] **Step 4: Прогнать все тесты проекта**
  - `npm --prefix frontend run check`
  - `npm --prefix frontend run build`
  - `go test -mod=vendor ./...`

- [ ] **Step 5: Собрать свежие бинарные файлы**
  - `make build-macos`
  - `make build-windows`
