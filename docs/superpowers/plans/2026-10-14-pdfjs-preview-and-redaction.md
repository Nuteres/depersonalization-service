# Интеграция Mozilla PDF.js для предпросмотра и физического обезличивания PDF: План реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать универсальный, пиксель-в-пиксель предпросмотр любых PDF-документов через автономный движок Mozilla `pdfjs-dist` со сплошным скроллом страниц, интерактивными плашками ПДн в стиле Word (`TextPreview.svelte`) с поточечным (per-instance) отключением и 100% гарантией физического наложения плашек цензурирования в итоговом файле.

**Architecture:** Frontend (Svelte 5 + Vite 6) загружает бинарные данные PDF через Wails API `GetFileBase64`, рендерит страницы единой вертикальной лентой на Canvas + TextLayer через локальный Web Worker Mozilla `pdfjs-dist`. Текст декодируется без искажений CMap/Identity-H, сопоставляется с детектором ПДн и покрывается интерактивными чипами. При сохранении точные координаты активных плашек `[{page, x, y, w, h}]` передаются в Go `PDFHandler.Process`, который рисует физические векторные прямоугольники (`re f`), гарантируя сокрытие по 152-ФЗ.

**Tech Stack:** Go 1.23, Wails v2, Svelte 5 (runes `$state`, `$derived`), Tailwind v4, Vite 6, `pdfjs-dist`.

**Spec:** `docs/superpowers/specs/2026-10-14-pdfjs-preview-design.md`

## Global Constraints
- **Air-Gapped:** 100% изоляция от интернета. `pdfjs-dist` и его Web Worker бандлятся строго локально, запрещены внешние CDN и запросы к сети.
- **Go Vendor:** Сборка только с флагом `-mod=vendor`.
- **Версионирование (SemVer):** Единый источник правды `internal/version/version.go` -> инкремент до `1.9.19` (MINOR/FEATURE). Синхронизация с `frontend/package.json` и `Makefile`.
- **Документация:** Обязательное обновление `CHANGELOG.md`, `README.md`, `docs/USER_GUIDE.md`, `docs/ARCHITECTURE.md`.
- **Качество и сборка бинарников:** Обязательный прогон `npm run check`, `npm run build`, `go test ./...` и пересборка исполняемых файлов в `bin/` (`make build-macos`, `make build-windows`, `make build-altlinux`).

## Review Focus
1. **Инверсия координат Y (PDF vs Canvas/DOM):** В PDF начало координат `(0,0)` находится в левом нижнем углу страницы, а в браузере/DOM — в левом верхнем. Расчет координат для `re f` на бэкенде обязан учитывать высоту страницы: `Y_pdf = PageHeight - Y_dom - Height_dom`.
2. **Масштабирование и плотность пикселей (DPR):** Экранные координаты плашек не должны зависеть от коэффициента масштабирования Canvas (Retina / 2x / 1.5x zoom) — координаты для бэкенда должны нормализоваться в стандартные PDF-пункты (72 DPI, 1 pt = 1/72 дюйма).
3. **Поточечное (per-instance) отключение:** Отключение одной фамилии на странице 1 (например, директора) не должно случайно снимать цензуру с той же фамилии на странице 2 или с других лиц.
4. **Разряженный текст (авиабилеты с letter-spacing):** В билетах буквы разделены кернингом (`П о д д е р ж к а`). Алгоритм извлечения текста в PDF.js должен объединять близко стоящие глифы одного блока строки, чтобы регулярные выражения корректно распознавали телефоны и ФИО.
5. **Многостраничные документы (сканы 10+ страниц):** Рендеринг 10 тяжелых сканированных страниц не должен блокировать интерфейс или вызывать утечку памяти WebView.

---

### Task 1: Backend Wails API: Доступ к содержимому PDF и наложение плашек по координатам

**Files:**
- Modify: `internal/domain/options.go`
- Modify: `internal/domain/models.go`
- Modify: `internal/desktop/app.go`
- Modify: `internal/service/processor/processor.go`
- Modify: `pkg/formats/pdf_handler.go`
- Test: `pkg/formats/pdf_test.go`

**Interfaces:**
- Produces in `internal/domain/options.go`:
  ```go
  type PDFRedactionBox struct {
      Page   int     `json:"page"`
      X      float64 `json:"x"`
      Y      float64 `json:"y"`
      Width  float64 `json:"width"`
      Height float64 `json:"height"`
  }
  ```
  Поле `PDFRedactionBoxes []PDFRedactionBox` в `domain.ProcessOptions`.
- Produces in `internal/desktop/app.go`:
  `func (a *DesktopApp) GetFileBase64(path string) (string, error)`
- Consumes in `pkg/formats/pdf_handler.go`:
  Опция `options["redaction_boxes"] = []domain.PDFRedactionBox` в `PDFHandler.Process`.

- [x] **Step 1: Написать падающий тест для наложения плашек по точным координатам**
  Добавить в `pkg/formats/pdf_test.go` тест `TestPDFHandler_ExactRedactionBoxes`, проверяющий, что при передаче `redaction_boxes` в итоговый PDF-поток записываются команды `re f` с переданными координатами.

- [x] **Step 2: Запустить тест и убедиться в падении**
  Run: `go test -v -run TestPDFHandler_ExactRedactionBoxes ./pkg/formats`
  Expected: FAIL (redaction_boxes option unhandled).

- [x] **Step 3: Реализовать поддержку `PDFRedactionBox` и `GetFileBase64`**
  1. Добавить структуру `PDFRedactionBox` и поле `PDFRedactionBoxes` в `internal/domain/options.go`.
  2. Добавить метод `GetFileBase64(path string) (string, error)` в `internal/desktop/app.go` (чтение `os.ReadFile` с кодированием в `base64.StdEncoding.EncodeToString`).
  3. В `internal/service/processor/processor.go` пробросить `opts.PDFRedactionBoxes` в `options["redaction_boxes"]` для PDF-обработчика.
  4. В `pkg/formats/pdf_handler.go` при наличии `redaction_boxes` внедрять векторные прямоугольники (`0 0 0 rg`, `x y w h re f`) на соответствующих страницах и вырезать текст под ними.

- [x] **Step 4: Запустить тест и убедиться в успешном прохождении**
  Run: `go test -v -run TestPDFHandler_ExactRedactionBoxes ./pkg/formats`
  Expected: PASS.

- [x] **Step 5: Коммит изменений бэкенда**
  ```bash
  git add internal/domain/options.go internal/desktop/app.go internal/service/processor/processor.go pkg/formats/pdf_handler.go pkg/formats/pdf_test.go
  git commit -m "feat(pdf): add GetFileBase64 API and coordinate-based PDF redaction"
  ```

---

### Task 2: Установка Mozilla `pdfjs-dist` и настройка локального Web Worker

**Files:**
- Modify: `frontend/package.json`
- Create: `frontend/src/lib/utils/pdfWorker.ts`
- Modify: `frontend/vite.config.ts` (при необходимости для worker-ассетов)

**Interfaces:**
- Produces: `import { getPdfDocument } from '$lib/utils/pdfWorker'`
  `getPdfDocument(data: Uint8Array | string): Promise<PDFDocumentProxy>`

- [x] **Step 1: Установить `pdfjs-dist` локально в `frontend/package.json`**
  Выполнить команду `npm --prefix frontend install pdfjs-dist@^4.10.38`.

- [x] **Step 2: Создать модуль инициализации `frontend/src/lib/utils/pdfWorker.ts`**
  Настроить бандлинг локального воркера без внешних CDN:
  ```ts
  import * as pdfjsLib from 'pdfjs-dist';
  import pdfWorker from 'pdfjs-dist/build/pdf.worker.min.mjs?url';

  pdfjsLib.GlobalWorkerOptions.workerSrc = pdfWorker;

  export async function loadPdf(data: Uint8Array | ArrayBuffer) {
    return pdfjsLib.getDocument({ data }).promise;
  }
  ```

- [x] **Step 3: Проверить проверку типов и сборку бандла**
  Run: `npm --prefix frontend run check && npm --prefix frontend run build`
  Expected: PASS (бандл собирается без сетевых зависимостей).

- [x] **Step 4: Коммит настройки PDF.js**
  ```bash
  git add frontend/package.json frontend/package-lock.json frontend/src/lib/utils/pdfWorker.ts
  git commit -m "feat(frontend): integrate offline Mozilla pdfjs-dist and worker"
  ```

---

### Task 3: Компонент `PdfPreview.svelte`: Непрерывный вертикальный скролл страниц

**Files:**
- Modify: `frontend/src/lib/components/depersonalize/PdfPreview.svelte`
- Modify: `frontend/src/lib/types/domain.ts`

**Interfaces:**
- Consumes: `loadPdf` из `$lib/utils/pdfWorker`, `GetFileBase64` из Wails backend.
- Produces: Отрисовка всех страниц документа в контейнере `#pdf-sheet-viewport` со сплошным скроллом страниц, HiDPI `<canvas>` и контейнером `div.textLayer`.

- [x] **Step 1: Реализовать загрузку байт документа через `GetFileBase64`**
  При изменении `appState.currentFileInfo` загружать base64 через бэкенд, декодировать в `Uint8Array` и инициализировать `pdfDoc`.

- [x] **Step 2: Реализовать постраничный рендеринг ленты страниц**
  Для каждой страницы `pageIndex = 1..numPages`:
  1. Создавать обертку листа `.pdf-page-sheet` (белый фон, тень, границы `#D5D3CC`).
  2. Рендерить страницу на `<canvas>` с учетом `window.devicePixelRatio` (четкость на Retina).
  3. Извлекать `page.getTextContent()` и вычислять геометрию текстовых блоков (PDF-координаты `x, y, w, h` в pt и экранные CSS-координаты).

- [x] **Step 3: Проверить типы и сборку**
  Run: `npm --prefix frontend run check && npm --prefix frontend run build`
  Expected: PASS.

- [x] **Step 4: Коммит компонента ленты страниц**
  ```bash
  git add frontend/src/lib/components/depersonalize/PdfPreview.svelte
  git commit -m "feat(frontend): continuous scroll PDF preview with pdfjs-dist"
  ```

---

### Task 4: Детекция ПДн, интерактивные плашки (Word-style) и поточечное отключение (Per-Instance)

**Files:**
- Modify: `frontend/src/lib/components/depersonalize/PdfPreview.svelte`
- Modify: `frontend/src/lib/stores/appState.svelte.ts`
- Modify: `frontend/src/lib/components/depersonalize/SaveDialogModal.svelte`

**Interfaces:**
- Produces in `PdfPreview.svelte`:
  - `disabledInstances = $state<Set<string>>(new Set())`
  - Функция клика `toggleInstance(instanceId: string)`
  - Всплывающий тултип (`До`, `После`, статус, подсказка)
  - Экспорт активных рамок цензурирования `activeBoxes: PDFRedactionBox[]` в `appState` для последующего сохранения.

- [x] **Step 1: Реализовать сопоставление текстовых блоков с детектором ПДн**
  На декодированном тексте страниц запускать детекцию сущностей (ФИО, телефоны, паспорта, адреса, даты), привязывая каждый найденный диапазон символов к точным координатам `BoundingBox` соответствующих глифов страницы.

- [x] **Step 2: Отрисовать интерактивные плашки и переключатель видов**
  1. Липкий тулбар: `[ Обезличенный вид ]` / `[ Исходный вид ]`, счетчик страниц и фрагментов ПДн.
  2. Плашки поверх Canvas:
     - В режиме «Обезличенный вид»: зеленые плашки с синтетическим/маскированным текстом (или `[УДАЛЕНО]`).
     - В режиме «Исходный вид»: янтарные плашки с оригинальным текстом.
     - При отключении конкретного экземпляра: серая перечеркнутая плашка.
  3. Тултип при наведении мыши (стиль 1-в-1 с `TextPreview.svelte`).

- [x] **Step 3: Реализовать поточечное переключение (per-instance toggle)**
  По клику на плашку переключать ее `instanceId` в `disabledInstances`.

- [x] **Step 4: Связать активные координаты цензурирования с диалогом сохранения**
  При нажатии кнопки «Обезличить» передавать активные координаты `PDFRedactionBoxes` в `ProcessFileWithOptions`.

- [x] **Step 5: Проверить типы и сборку**
  Run: `npm --prefix frontend run check && npm --prefix frontend run build`
  Expected: PASS.

- [x] **Step 6: Коммит интерактивных плашек**
  ```bash
  git add frontend/src/lib/components/depersonalize/PdfPreview.svelte frontend/src/lib/stores/appState.svelte.ts frontend/src/lib/components/depersonalize/SaveDialogModal.svelte
  git commit -m "feat(frontend): word-style interactive chips with per-instance toggle for PDF"
  ```

---

### Task 5: Сквозное тестирование на реальных файлах, бамп версии (1.9.19), документация и сборка бинарников

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `README.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`

- [x] **Step 1: Сквозное тестирование на 4 проблемных PDF-файлах**
  Проверить в приложении:
  1. `forma_tipovogo_dogovora_vyezdnoe_obsluzhivanie.pdf`: идеальная 2-колоночная верстка, корректные плашки, при сохранении черные плашки наложены строго на ПДн.
  2. `ETICKET_PLETNEV...`: распознавание телефонов/билетов, плашки на разряженном тексте.
  3. `Плетнёв_ЯА_Оригинал_заявления...`: идеальное чтение без спецсимволов Identity-H.
  4. `Положение о управлении.pdf`: чистый скролл 10 страниц без зависания.

- [x] **Step 2: Прогон всех тестов и линтеров**
  Run: `npm --prefix frontend run check`
  Run: `npm --prefix frontend run build`
  Run: `go test ./...`
  Expected: Все тесты проходят, 0 ошибок.

- [x] **Step 3: Бамп версии до 1.9.19 и обновление документации**
  1. `internal/version/version.go`: `Version = "1.9.19"`.
  2. `frontend/package.json`: `"version": "1.9.19"`.
  3. `Makefile`: `VERSION := 1.9.19`.
  4. `CHANGELOG.md`: добавить раздел `## [1.9.19] - 2026-10-14` с разделами `### Добавлено`, `### Изменено`, `### Исправлено`.
  5. `docs/USER_GUIDE.md`, `docs/ARCHITECTURE.md`, `README.md`: отразить работу с PDF через PDF.js и пообъектное отключение плашек.

- [x] **Step 4: Сборка исполняемых бинарных файлов для всех платформ**
  Run: `make build-macos`
  Run: `make build-windows`
  Run: `make build-altlinux`
  Проверить наличие свежих файлов в `bin/`.

- [x] **Step 5: Финальный коммит версии 1.9.19**
  ```bash
  git add -A
  git commit -m "release: v1.9.19 - universal PDF.js preview with continuous scroll and per-instance redaction"
  ```
