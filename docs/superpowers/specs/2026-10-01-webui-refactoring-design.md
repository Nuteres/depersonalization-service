# Дизайн-документ: Рефакторинг Go WebUI приложения (Clean Architecture + Svelte 5 + TypeScript)

- **Дата**: 2026-10-01
- **Статус**: Согласовано
- **Стек**: Go 1.25, Wails v2, Svelte 5 (Runes), Vite, TypeScript, Tailwind CSS v4

---

## 1. Введение и цели

Приложение «Деперсонализация ПД (152-ФЗ)» представляет собой кроссплатформенный десктопный инструмент для локального обезличивания и обратного восстановления структурированных таблиц (XLSX, XLSB, XLS, ODS, CSV) и текстовых документов (DOCX, ODT, PDF) с криптографическим хранилищем соответствий `.depmap` (AES-256-GCM).

### Текущие проблемы:
1. **Монолитный контроллер Go (`cmd/app/wails_app.go`)**: свыше 2100 строк в одном файле. Бизнес-логика (парсинг 8 форматов, эвристики шапок, стриминг, шифрование) переплетена с Wails IPC и диалогами ОС. Заготовка `pkg/engine/engine.go` осталась неиспользованным стабом.
2. **Отсутствие изоляции ядра**: Невозможно тестировать сервисы чистыми unit-тестами без привязки к `wails.runtime`. Невозможно запустить приложение как CLI-утилиту.
3. **Хрупкий фронтенд**: Vanilla JS, разрозненное глобальное состояние `window`, прямые манипуляции DOM (`innerHTML`, `getElementById`), отсутствие статической типизации вызовов Wails IPC.
4. **Тяжелый рантайм стилей**: Использование `tailwind.min.js`, компилирующего CSS в браузере при каждом изменении DOM.

### Цели рефакторинга:
1. **Чистая архитектура (Clean Architecture)** на Go: полная изоляция доменной логики и сервисов от слоя представления Wails.
2. **Современный фронтенд**: Перевод интерфейса на **Svelte 5** (с новой реактивностью **Runes**), **Vite**, **TypeScript** и Ahead-of-Time компиляцию **Tailwind CSS v4**.
3. **Строгая типизация контрактов**: Автогенерация интерфейсов TypeScript из Go-структур (`wailsjs`), исключающая ошибки рассинхронизации API.
4. **100% сохранение функциональности и производительности**:
   - Полное сохранение всех возможностей для 8 форматов, мульти-листов Excel, исключения колонок, бесконечного скролла и криптоконтейнеров.
   - Повышение отзывчивости интерфейса (AOT CSS, нативный DOM без Virtual DOM в Svelte 5, `$state.raw` для больших массивов строк).
   - Поддержка отмены тяжелых операций через `context.Context`.

---

## 2. Архитектура бэкенда (Go)

### 2.1. Структура каталогов
```
dep_service/
├── cmd/
│   └── app/
│       └── main.go                  # Точка входа: сборка DI-контейнера и запуск Wails
├── internal/
│   ├── domain/                      # Чистые сущности и контракты (без внешних зависимостей)
│   │   ├── models.go                # FileInfo, SheetInfo, EntityInfo, PreviewRow
│   │   ├── options.go               # ProcessOptions, RestoreOptions
│   │   ├── results.go               # ProcessResult, RestoreResult, KeyFileInfo
│   │   ├── progress.go              # ProgressReporter callback type
│   │   └── errors.go                # Специфичные доменные ошибки
│   ├── service/                     # Чистая бизнес-логика (чистые unit-тесты)
│   │   ├── inspector/               # Инспекция форматов и распознавание ПДн
│   │   │   ├── inspector.go
│   │   │   └── inspector_test.go
│   │   ├── processor/               # Потоковая деперсонализация с context.Context
│   │   │   ├── processor.go
│   │   │   └── processor_test.go
│   │   ├── restorer/                # Потоковое восстановление по .depmap
│   │   │   ├── restorer.go
│   │   │   └── restorer_test.go
│   │   ├── preview/                 # Генерация пагинированных срезов данных (offset/limit)
│   │   │   ├── preview.go
│   │   │   └── preview_test.go
│   │   └── keymanager/              # Извлечение маппингов, валидация и создание .depmap
│   │       ├── keymanager.go
│   │       └── keymanager_test.go
│   └── desktop/                     # Wails IPC адаптер (Presentation Layer)
│       ├── app.go                   # Методы, экспортируемые в UI
│       ├── dialogs.go               # Нативные диалоги выбора/сохранения файлов ОС
│       └── events.go                # Мост между ProgressReporter и Wails runtime.EventsEmit
├── pkg/                             # Низкоуровневые доменные пакеты
│   ├── anonymizer/                  # Детерминированный генератор синтетических данных
│   ├── depmap/                      # Формат файла-контейнера AES-256-GCM / PBKDF2
│   ├── detector/                    # Контрольные суммы (СНИЛС, ИНН) и поиск шапок
│   └── formats/                     # Форматные ридеры/райтеры (Excelize, ODF, CSV, PDF)
└── frontend/                        # Современный UI на Svelte 5 + Vite + TypeScript
```

### 2.2. Сервисный слой

1. **`InspectorService`**:
   - Принимает `path string`.
   - Распознаёт формат (XLSX, XLSB, XLS, ODS, CSV, DOCX, ODT, PDF).
   - Читает структуру, находит шапку через `detector.FindHeaderRow`, сэмплирует строки.
   - Запускает детекцию типов колонок через `anonymizer.Anonymizer` и `detector`.
   - Возвращает агрегированный `domain.FileInfo`.
2. **`ProcessorService`**:
   - Сигнатура: `Process(ctx context.Context, opts domain.ProcessOptions, reporter domain.ProgressReporter) (*domain.ProcessResult, error)`.
   - Выполняет потоковую обработку без загрузки всего файла в память.
   - Каждые $N$ строк или 100 мс вызывает `reporter(processed, total, percent)`.
   - В каждой итерации проверяет `ctx.Err()`: при отмене прерывает запись, закрывает потоки и удаляет недописанный файл.
   - При `opts.CreateKey == true` координирует генерацию соответствий через `KeyManager`.
3. **`RestorerService`**:
   - Сигнатура: `Restore(ctx context.Context, opts domain.RestoreOptions, reporter domain.ProgressReporter) (*domain.RestoreResult, error)`.
   - Открывает зашифрованный контейнер `.depmap`, валидирует пароль.
   - Потоково применяет обратные замены к целевому файлу.
4. **`PreviewService`**:
   - Сигнатура: `GetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error)`.
   - Отвечает за быструю подгрузку порций строк для бесконечного скролла таблицы.
   - Вычисляет значения «До» и «После» на лету для переданного диапазона.
5. **`KeyManager`**:
   - `ValidateKey(keyPath, password string) (*domain.KeyFileInfo, error)` — валидация контейнера без распаковки всех данных.
   - `CreateContainer(destPath string, mappings map[string]string, password string, origFile string) error`.

---

## 3. Слой адаптера Wails (`internal/desktop`) и IPC контракт

### 3.1. Типизированные параметры (DTO)
Вместо длинных цепочек аргументов используются структуры:
```go
type ProcessOptions struct {
    InputPath        string           `json:"inputPath"`
    OutputPath       string           `json:"outputPath"`
    Salt             string           `json:"salt"`
    DisabledCols     []int            `json:"disabledCols"`
    DisabledBySheet  map[string][]int `json:"disabledBySheet"`
    CreateKey        bool             `json:"createKey"`
    KeyPassword      string           `json:"keyPassword"`
    KeyPath          string           `json:"keyPath"`
}

type RestoreOptions struct {
    InputPath  string `json:"inputPath"`
    KeyPath    string `json:"keyPath"`
    Password   string `json:"password"`
    OutputPath string `json:"outputPath"`
}
```

### 3.2. Методы `DesktopApp`
- `GetAppInfo() map[string]string`
- `SelectFile() (string, error)`
- `SaveFileDialog(defaultName string) (string, error)`
- `SelectKeyFile() (string, error)`
- `SaveKeyFileDialog(defaultName string) (string, error)`
- `OpenContainingFolder(path string) error`
- `InspectFile(path string) (*domain.FileInfo, error)`
- `InspectKeyFile(keyPath, password string) (*domain.KeyFileInfo, error)`
- `GetTableRows(path string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error)`
- `GetSheetRows(path, sheetName string, offset, limit int, salt string, disabledCols []int) ([]domain.PreviewRow, error)`
- `ProcessFile(opts domain.ProcessOptions) (*domain.ProcessResult, error)`
- `RestoreFile(opts domain.RestoreOptions) (*domain.RestoreResult, error)`
- `CancelCurrentOperation()` — вызывает отмену активного контекста.

### 3.3. События (Wails Events)
- `app:file-dropped`: путь перетащенного файла.
- `process-progress`: `{ "processed": int, "total": int, "percent": int }`.
- `restore-progress`: `{ "processed": int, "total": int, "percent": int }`.

---

## 4. Архитектура фронтенда (Svelte 5 + Vite + TypeScript)

### 4.1. Структура фронтенда
```
frontend/
├── package.json
├── vite.config.ts
├── tsconfig.json
├── svelte.config.js
├── src/
│   ├── main.ts                      # Точка входа Svelte 5 (mount(App, ...))
│   ├── app.css                      # Tailwind v4 директивы (@import "tailwindcss";)
│   ├── App.svelte                   # Главный контейнер, навигация табов, DnD listener
│   ├── lib/
│   │   ├── api/
│   │   │   └── wails.ts             # Типизированный фасад для вызовов Wails и подписок
│   │   ├── stores/
│   │   │   ├── appState.svelte.ts   # Глобальное состояние: режим, активный файл, соль ($state)
│   │   │   └── tableState.svelte.ts # Состояние таблицы: строки ($state.raw), оффсеты, лист
│   │   └── components/
│   │       ├── layout/
│   │       │   └── Header.svelte
│   │       ├── depersonalize/
│   │       │   ├── HeroBanner.svelte
│   │       │   ├── SheetTabs.svelte
│   │       │   ├── EntityGrid.svelte
│   │       │   ├── TableView.svelte
│   │       │   ├── TextPreview.svelte
│   │       │   ├── PdfPreview.svelte
│   │       │   └── SaveDialogModal.svelte
│   │       ├── restore/
│   │       │   ├── RestoreView.svelte
│   │       │   └── KeyInfoCard.svelte
│   │       └── common/
│   │           ├── ProgressModal.svelte
│   │           ├── SuccessModal.svelte
│   │           └── Toast.svelte
│   └── wailsjs/                     # Автогенерируемые биндинги Wails
└── dist/                            # Каталог сборки Vite, встраиваемый в Go (embed.FS)
```

### 4.2. Использование возможностей Svelte 5
- **Runes**:
  - `$state` для реактивных примитивов и флагов (текущий режим, статус загрузки, выбранный лист).
  - `$state.raw` для тяжелых массивов строк (`PreviewRow[]`). Исключает накладные расходы на глубокую проксификацию, гарантируя мгновенный рендеринг сотен строк таблицы.
  - `$derived` для вычисляемых свойств (процент исключённых колонок, отображаемый формат экспорта).
  - `$props()` для типизированных пропсов компонентов.
- **Tailwind CSS v4**:
  - Подключение через `@tailwindcss/vite`.
  - Все CSS-классы компилируются Ahead-of-Time при `npm run build`. Никакого runtime JS для стилей.

---

## 5. Обработка ошибок и сценарии отмены

1. **Отмена тяжелых задач**:
   - При старте `ProcessFile` или `RestoreFile` создается контекст `context.WithCancel(desktopApp.ctx)`.
   - Экземпляр `cancelFunc` сохраняется в потокобезопасном поле `DesktopApp`.
   - Клик по кнопке «Отмена» в модалке шлет `CancelCurrentOperation()`.
   - Воркер прерывает цикл, закрывает I/O потоки и удаляет временный выходной файл.
2. **Пользовательские ошибки**:
   - Ошибки типизируются кодами и возвращаются с понятным сообщением на русском языке.
   - Ошибки пароля `.depmap` не приводят к падению, а отображаются в карточке ввода пароля.

---

## 6. Стратегия тестирования и верификации

1. **Go Unit-тесты (`internal/service/...`)**:
   - `inspector_test.go`: проверка детекции на всех поддерживаемых фикстурах (XLSX, XLSB, XLS, ODS, CSV, DOCX, ODT, PDF).
   - `processor_test.go`: потоковое обезличивание таблиц и текстов, проверка эвристики пола, корректности контрольных сумм СНИЛС/ИНН, проверка вызова отмены через контекст.
   - `restorer_test.go`: сквозной round-trip тест (`Оригинал -> Анонимизация с ключом -> Восстановление -> Проверка равенства значений`).
2. **Типизация и линтинг фронтенда**:
   - `npm run check`: строгая проверка `svelte-check` и TypeScript компилятора.
   - `npm run build`: верификация успешного бандлинга в `dist/`.
3. **Регрессионная сборка**:
   - `go test -v ./...`
   - Сборка бинарника через `go build -mod=vendor -tags desktop,production ./cmd/app`.
