# Масштабирование интерфейса приложения (UI Zoom & Scaling) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Реализовать централизованное кроссплатформенное масштабирование интерфейса (70%–200%) с поддержкой Ctrl/Cmd+колёсико мыши, горячих клавиш, компактным блоком управления в шапке и сохранением настроек в `localStorage`.

**Architecture:** Выделенное реактивное хранилище Svelte 5 `zoomState.svelte.ts` управляет числовым коэффициентом масштабирования и сохранением в `localStorage`, применяя аппаратный CSS `zoom` к `document.documentElement`. Глобальные слушатели событий `wheel` и `keydown` перехватывают жесты и горячие клавиши, а `Header.svelte` отображает кнопки дискретного изменения и сброса.

**Tech Stack:** Svelte 5 (runes: `$state`, `$derived`), Tailwind CSS v4, TypeScript, Go 1.23, Wails v2.

**Spec:** `docs/superpowers/specs/2026-10-15-ui-zoom-and-scaling-design.md`

## Global Constraints

- **Версионирование:** обновление с 1.9.22 до 1.10.0 (MINOR) синхронно в `internal/version/version.go`, `frontend/package.json` и `Makefile`.
- **100% Air-Gapped:** отсутствие сетевых запросов и сторонних CDN.
- **Go Vendor:** флаг `-mod=vendor` при сборках Go.
- **Диапазон масштабирования:** от 0.70 (70%) до 2.00 (200%). Шаг кнопок: 0.10 (10%), шаг колёсика мыши: 0.05 (5%).
- **Кроссплатформенность:** обработка `e.ctrlKey || e.metaKey` для поддержки macOS, Windows и Alt Linux.

## Review Focus

1. **Защита от нежелательного вертикального скролла при зумировании:** при `Ctrl/Cmd + Wheel` обязательно вызывается `e.preventDefault()`, чтобы документ не дергался по вертикали во время прокрутки.
2. **Защита от поврежденных значений в `localStorage`:** значение из хранилища парсится и валидируется на `NaN` и границы `[0.70, 2.00]`; при выходе за пределы восстанавливается `1.0`.
3. **Совместимость с native Drag & Drop:** пересчет координат или вызов `document.elementFromPoint` в `App.svelte` не должен ломать зоны сброса файлов `.depmap` и табличных данных при нестандартном зуме.
4. **Очистка слушателей и таймеров:** таймер скрытия оверлей-индикатора масштаба корректно очищается (`clearTimeout`) при быстром повторном изменении или демонтировании компонента.
5. **Предотвращение размытия шрифтов:** использование аппаратного свойства `zoom` вместо `transform: scale`, сохраняющее векторную растеризацию текстовых глифов на ретина-экранах и WebView2/WebKit.

---

### Task 1: Создание хранилища `zoomState.svelte.ts` и модульные тесты

**Files:**
- Create: `frontend/src/lib/stores/zoomState.svelte.ts`
- Create: `frontend/src/lib/stores/zoomState.test.mjs`

**Interfaces:**
- Consumes: `localStorage`
- Produces:
  ```typescript
  export class ZoomState {
    currentZoom: number;
    showIndicator: boolean;
    init(): void;
    zoomIn(step?: number): void;
    zoomOut(step?: number): void;
    resetZoom(): void;
    setZoom(val: number): void;
    triggerIndicator(): void;
  }
  export const zoomState: ZoomState;
  ```

- [ ] **Step 1: Написать падающий модульный тест `zoomState.test.mjs`**
  Создать тест логики расчётов масштабирования (проверка границ min 0.70 / max 2.00, шагов 0.10 и 0.05, валидации сохраненных значений).

- [ ] **Step 2: Запустить тест и убедиться, что он падает (модуль ещё не реализован)**
  Run: `node --test frontend/src/lib/stores/zoomState.test.mjs`
  Expected: FAIL (Cannot find module)

- [ ] **Step 3: Реализовать `ZoomState` в `frontend/src/lib/stores/zoomState.svelte.ts`**
  - Использовать руны Svelte 5 (`$state`).
  - Реализовать `clampZoom(val)`, `init()`, `zoomIn()`, `zoomOut()`, `resetZoom()`, `setZoom()`, `triggerIndicator()`.
  - Применять `document.documentElement.style.zoom = String(this.currentZoom)` в браузерном окружении.
  - Сохранять значение в `localStorage.setItem('dep_app_zoom', String(this.currentZoom))`.

- [ ] **Step 4: Запустить тесты и убедиться в успешном прохождении**
  Run: `node --test frontend/src/lib/stores/zoomState.test.mjs`
  Expected: PASS

- [ ] **Step 5: Запустить проверку типов TypeScript**
  Run: `npm --prefix frontend run check`
  Expected: 0 errors

- [ ] **Step 6: Зафиксировать изменения в git**
  Run: `git add frontend/src/lib/stores/zoomState.svelte.ts frontend/src/lib/stores/zoomState.test.mjs && git commit -m "feat(ui): add reactive zoomState store with bounds clamping and persistence"`

---

### Task 2: Интеграция блока масштабирования в `Header.svelte` и горячих клавиш в `App.svelte`

**Files:**
- Modify: `frontend/src/lib/components/layout/Header.svelte`
- Modify: `frontend/src/App.svelte`

**Interfaces:**
- Consumes: `zoomState` из `frontend/src/lib/stores/zoomState.svelte.ts`
- Produces:
  - Интерактивный блок кнопок `–`, `{percent}%`, `+` в шапке рядом с журналом.
  - Глобальный оверлей с текущим значением зума при изменении через клавиатуру/мышь.
  - Глобальные слушатели `wheel` (с зажатым Ctrl/Cmd) и `keydown` (`Ctrl/Cmd + / - / 0`).

- [ ] **Step 1: Добавить блок масштабирования в `Header.svelte`**
  - Подключить `import { zoomState } from '../../stores/zoomState.svelte';`.
  - Рядом с кнопкой `btn-open-audit` разместить контейнер с кнопками `–`, текущим процентом `zoomState.currentZoom` (сброс при клике) и `+`.
  - Задать тултипы с горячими клавишами.

- [ ] **Step 2: Добавить обработку колёсика мыши, горячих клавиш и оверлей в `App.svelte`**
  - Вызвать `zoomState.init()` в `onMount()`.
  - Зарегистрировать слушатель `wheel` с `{ passive: false }`: при `e.ctrlKey || e.metaKey` вызывать `e.preventDefault()`, менять зум на 0.05 и показывать индикатор.
  - Зарегистрировать слушатель `keydown`: при `e.ctrlKey || e.metaKey` перехватывать `+`, `=`, `-`, `0`, вызывать `e.preventDefault()` и менять масштаб.
  - Добавить модальный оверлей-бейдж `{Math.round(zoomState.currentZoom * 100)}%` с плавной CSS-анимацией появления и исчезновения.

- [ ] **Step 3: Проверить типы и собрать фронтенд**
  Run: `npm --prefix frontend run check && npm --prefix frontend run build`
  Expected: 0 errors, build successful

- [ ] **Step 4: Зафиксировать изменения в git**
  Run: `git add frontend/src/lib/components/layout/Header.svelte frontend/src/App.svelte && git commit -m "feat(ui): integrate header zoom controls, hotkeys and mouse wheel scaling"`

---

### Task 3: Инкремент версии, актуализация документации и прогон тестов

**Files:**
- Modify: `internal/version/version.go`
- Modify: `frontend/package.json`
- Modify: `Makefile`
- Modify: `CHANGELOG.md`
- Modify: `docs/USER_GUIDE.md`
- Modify: `docs/ARCHITECTURE.md`
- Modify: `README.md`

- [ ] **Step 1: Обновить версию до `1.10.0`**
  - В `internal/version/version.go`: `const Version = "1.10.0"`
  - В `frontend/package.json`: `"version": "1.10.0"`
  - В `Makefile`: `VERSION = 1.10.0`

- [ ] **Step 2: Добавить секцию в `CHANGELOG.md`**
  - Создать заголовок `## [1.10.0] - 2026-10-15` с секциями `### Добавлено` и `### Изменено`.
  - Подробно описать возможности масштабирования (горячие клавиши, колёсико мыши, шаги, индикатор, сохранение).

- [ ] **Step 3: Актуализировать пользовательскую документацию и архитектуру**
  - В `docs/USER_GUIDE.md`: добавить раздел «Масштабирование интерфейса» с таблицей горячих клавиш и описанием блока в шапке.
  - В `docs/ARCHITECTURE.md`: описать реактивное состояние `zoomState` и механизм аппаратного CSS-масштабирования.
  - В `README.md`: обновить бейджи и номера версий.

- [ ] **Step 4: Прогнать тесты Go**
  Run: `go test -mod=vendor ./...`
  Expected: All packages PASS

- [ ] **Step 5: Зафиксировать изменения в git**
  Run: `git add internal/version/version.go frontend/package.json Makefile CHANGELOG.md docs/USER_GUIDE.md docs/ARCHITECTURE.md README.md && git commit -m "chore(release): bump version to 1.10.0 and update documentation"`

---

### Task 4: Сборка исполняемых бинарных файлов (Build Artifacts)

**Files:**
- Produce: `bin/depersonalizer_macos` (или universal bundle)
- Produce: `bin/depersonalizer_windows_x86_64.exe`
- Produce: `bin/depersonalizer_altlinux_p10_x86_64`

- [ ] **Step 1: Собрать бинарник под macOS**
  Run: `make build-macos`
  Expected: Успешная компиляция и создание бинарника в `bin/`

- [ ] **Step 2: Собрать бинарник под Windows**
  Run: `make build-windows`
  Expected: Генерация ресурсов `.syso` и сборка `.exe` в `bin/`

- [ ] **Step 3: Собрать бинарник под Alt Linux через Docker**
  Run: `make build-altlinux`
  Expected: Docker-сборка под платформу `linux/amd64` и размещение в `bin/`

- [ ] **Step 4: Проверить наличие и размеры всех собранных бинарников в `bin/`**
  Run: `ls -la bin/`
  Expected: Присутствуют актуальные бинарники для всех 3 целевых платформ.
