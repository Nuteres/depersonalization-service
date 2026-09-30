# Обратная персонализация (деобезличивание) через .depmap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать обратную персонализацию (деобезличивание / ре-идентификацию) данных 152-ФЗ с помощью защищённых файлов-ключей соответствий (`.depmap`), зашифрованных алгоритмом AES-256-GCM с KDF PBKDF2-HMAC-SHA256, с возможностью полного восстановления исходных данных в Excel (XLSX) и CSV.

**Architecture:**
- Ядро `pkg/depmap`: бинарный контейнер с аутентифицированным шифрованием AES-256-GCM, PBKDF2 KDF (100 000 итераций) и zlib-сжатием словаря соответствий `synthetic -> original`.
- Обработчики `pkg/formats`: потоковые алгоритмы восстановления `RestoreCSV` и `RestoreExcel`, заменяющие синтетические значения реальными и сохраняющие любые добавленные внешними аналитиками колонки.
- Сервисный слой `cmd/app`: методы Wails IPC для инспекции ключей, создания ключей при деперсонализации и пакетного обратного восстановления файлов.
- Веб-интерфейс `frontend/index.html`: вкладка режима восстановления, опция генерации ключа при сохранении, валидация ключей и индикатор прогресса.

**Tech Stack:** Go 1.25, `crypto/aes`, `crypto/cipher`, `crypto/sha256`, `crypto/hmac`, `compress/zlib`, `github.com/xuri/excelize/v2`, Wails v2.

**Spec:** [docs/superpowers/specs/2026-09-30-reverse-depersonalization-design.md](file:///Users/nuter/Go/dep_service/docs/superpowers/specs/2026-09-30-reverse-depersonalization-design.md)

## Global Constraints
- On-premise / Air-gapped: 0 внешних сетевых запросов.
- Безопасность: AES-256-GCM аутентифицированное шифрование; отказ при неверном пароле или нарушении целостности.
- Совместимость: кроссплатформенная сборка macOS (Universal ARM64+Intel), Windows x86_64, Alt Linux 10.4.

---

### Task 1: Пакет `pkg/depmap` — криптографический контейнер .depmap

**Files:**
- Create: `pkg/depmap/depmap.go`
- Test: `pkg/depmap/depmap_test.go`

**Interfaces:**
- Produces:
  ```go
  type DepMap struct {
      Version          int               `json:"version"`
      CreatedAt        time.Time         `json:"created_at"`
      OriginalFileName string            `json:"original_file_name"`
      RecordsCount     int               `json:"records_count"`
      Mapping          map[string]string `json:"mapping"` // synthetic -> original
  }
  func New(originalFileName string) *DepMap
  func (dm *DepMap) Put(synthetic, original string)
  func (dm *DepMap) SaveFile(path string, password string) error
  func LoadFile(path string, password string) (*DepMap, error)
  func InspectKeyFile(path string) (bool, error)
  ```

- [ ] **Step 1: Написать падающий unit-тест `pkg/depmap/depmap_test.go`**
  - Тест 1: `TestDepMap_SaveAndLoad`: создание карты, сохранение с паролем, чтение, сверка значений.
  - Тест 2: `TestDepMap_WrongPassword`: ошибка при неверном пароле.
  - Тест 3: `TestDepMap_CorruptedData`: ошибка при повреждении байтов файла.
  - Тест 4: `TestDepMap_DefaultMasterKey`: сохранение и открытие без пароля (с мастер-секретом приложения).

- [ ] **Step 2: Запустить тест `go test -v ./pkg/depmap` и убедиться в ошибке компиляции/падении**

- [ ] **Step 3: Реализовать `pkg/depmap/depmap.go`**
  - Сигнатура `DEPMAP01`, генерация 16-байтной KDF соли, PBKDF2-HMAC-SHA256 (100 000 итераций).
  - Сжатие JSON через `zlib`, шифрование через `cipher.NewGCM(block)`.
  - Чтение, дешифрация, проверка аутентификации GCM, распаковка zlib.

- [ ] **Step 4: Запустить тест `go test -v ./pkg/depmap` и убедиться в 100% PASS**

- [ ] **Step 5: Закоммитить изменения:**
  `git add pkg/depmap/ && git commit -m "feat(depmap): implement encrypted .depmap key container with AES-256-GCM and PBKDF2"`

---

### Task 2: Потоковые восстановители форматов в `pkg/formats`

**Files:**
- Create: `pkg/formats/restore_csv.go`
- Create: `pkg/formats/restore_excel.go`
- Test: `pkg/formats/restore_test.go`

**Interfaces:**
- Consumes: `pkg/depmap.DepMap`
- Produces:
  ```go
  func RestoreCSV(ctx context.Context, r io.Reader, w io.Writer, dm *depmap.DepMap) (int, error)
  func RestoreExcel(ctx context.Context, inputPath, outputPath string, dm *depmap.DepMap, onProgress func(pct int, proc, total int)) (int, error)
  ```

- [ ] **Step 1: Написать падающий unit-тест `pkg/formats/restore_test.go`**
  - Тест 1: `TestRestoreCSV`: генерация CSV с фейковыми ФИО/паспортами, восстановление, сверка с исходным CSV.
  - Тест 2: `TestRestoreExcel`: генерация XLSX с фейковыми данными и добавленной аналитиками колонкой `Score`, восстановление, проверка сохранения колонки `Score` и возврата реальных ФИО.

- [ ] **Step 2: Запустить тесты `go test -v ./pkg/formats -run TestRestore` и убедиться в падении**

- [ ] **Step 3: Реализовать `pkg/formats/restore_csv.go` и `pkg/formats/restore_excel.go`**
  - `RestoreCSV`: авто-определение разделителя, построчная замена через `dm.Mapping[cell]`.
  - `RestoreExcel`: открытие книги через `excelize`, обход листов, замена ячеек, сохранение в `outputPath` с вызовом `onProgress`.

- [ ] **Step 4: Запустить тесты `go test -v ./pkg/formats` и убедиться в успешном прохождении**

- [ ] **Step 5: Закоммитить изменения:**
  `git add pkg/formats/ && git commit -m "feat(formats): implement streaming CSV and Excel XLSX data restorers"`

---

### Task 3: Интеграция генерации ключа при деперсонализации в `cmd/app`

**Files:**
- Modify: `cmd/app/wails_app.go`
- Modify: `cmd/app/wails_app_test.go`

**Interfaces:**
- Updates `ProcessFileTo`:
  ```go
  func (a *App) ProcessFileTo(inputPath string, destPath string, salt string, disabledCols []int, createKey bool, keyPassword string, keyPath string) (*ProcessResult, error)
  ```
- `ProcessResult` расширяется полями:
  ```go
  KeyPath string `json:"keyPath,omitempty"`
  ```

- [ ] **Step 1: Написать тест в `cmd/app/wails_app_test.go`**
  - Проверка, что при `createKey: true` создаётся файл `.depmap`, который успешно открывается через `depmap.LoadFile` и содержит верные соответствия.

- [ ] **Step 2: Обновить `ProcessFileTo` в `cmd/app/wails_app.go`**
  - В процессе деперсонализации собирать `dm.Put(syntheticVal, originalVal)`.
  - Если `createKey` включен, сохранять ключ в `keyPath` (или `destPath + ".depmap"`).

- [ ] **Step 3: Прогнать тесты `go test -v ./cmd/app`**

- [ ] **Step 4: Закоммитить изменения:**
  `git add cmd/app/ && git commit -m "feat(app): support automatic .depmap key generation during depersonalization"`

---

### Task 4: Wails IPC методы восстановления данных

**Files:**
- Modify: `cmd/app/wails_app.go`
- Test: `cmd/app/wails_app_test.go`

**Interfaces:**
- Produces:
  ```go
  type KeyFileInfo struct {
      Path             string `json:"path"`
      OriginalFileName string `json:"originalFileName"`
      CreatedAt        string `json:"createdAt"`
      RecordsCount     int    `json:"recordsCount"`
      Valid            bool   `json:"valid"`
  }
  type RestoreResult struct {
      Success       bool   `json:"success"`
      InputPath     string `json:"inputPath"`
      KeyPath       string `json:"keyPath"`
      OutputPath    string `json:"outputPath"`
      RestoredCells int    `json:"restoredCells"`
      DurationMs    int64  `json:"durationMs"`
      Error         string `json:"error,omitempty"`
  }
  func (a *App) SelectKeyFile() (string, error)
  func (a *App) InspectKeyFile(keyPath string, password string) (*KeyFileInfo, error)
  func (a *App) RestoreFileTo(inputPath string, keyPath string, password string, destPath string) (*RestoreResult, error)
  ```

- [ ] **Step 1: Написать тест `TestApp_RestoreWorkflow` в `cmd/app/wails_app_test.go`**
  - Полный цикл: исходный файл -> деперсонализация с генерацией ключа -> восстановление через `RestoreFileTo` -> сверка данных.

- [ ] **Step 2: Реализовать методы в `cmd/app/wails_app.go`**
  - `SelectKeyFile` с фильтром `*.depmap`.
  - `InspectKeyFile` с верификацией пароля.
  - `RestoreFileTo` с отправкой прогресса `restore-progress`.

- [ ] **Step 3: Запустить `go test -v ./cmd/app` и убедиться в успешном прохождении**

- [ ] **Step 4: Закоммитить изменения:**
  `git add cmd/app/ && git commit -m "feat(app): implement RestoreFileTo and KeyFile Wails IPC handlers"`

---

### Task 5: Пользовательский интерфейс (UI) режима восстановления и опции ключа

**Files:**
- Modify: `frontend/index.html`

- [ ] **Step 1: Добавить вкладки верхнего уровня в навигацию:**
  - `[ ● Деперсонализация ПД | ○ Восстановление данных ]`
  - Переключение режимов интерфейса без перезагрузки страницы.

- [ ] **Step 2: Добавить опцию создания ключа в диалог сохранения файла:**
  - Чекбокс `[✔] Создать защищенный ключ восстановления (.depmap)`.
  - Поле пароля (опциональное).
  - При успехе: вывод ссылки/пути как к очищенному файлу, так и к сохранённому файлу `.depmap`.

- [ ] **Step 3: Реализовать экран «Восстановление данных»:**
  - Блок 1: Выбор обработанного файла данных (`.xlsx`, `.csv`) + отображение пути и строк.
  - Блок 2: Выбор файла-ключа `.depmap` + отображение метаданных (дата создания, количество записей).
  - Блок 3: Поле пароля (при необходимости).
  - Кнопка «Восстановить исходные данные...» -> выбор места сохранения.
  - Модальное окно прогресса и окно завершения с кнопкой «📁 Показать в папке».

- [ ] **Step 4: Проверить синтаксис JS через Node.js и запустить сборку:**
  - `node -e "..."`
  - `make build-macos && make build-windows`

- [ ] **Step 5: Закоммитить изменения:**
  `git add frontend/index.html && git commit -m "feat(ui): implement dual-mode restoration UI and .depmap key options"`
