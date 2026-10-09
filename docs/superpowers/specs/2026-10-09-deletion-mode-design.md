# Архитектурная спецификация: Режим удаления персональных данных (ModeDelete) и устойчивое восстановление (Header-Aware)

**Дата:** 2026-10-09  
**Статус:** Согласовано  
**Версия компонента:** Minor bump (добавление нового режима работы и расширение формата .depmap)

---

## 1. Обзор и цели

### 1.1. Назначение
В дополнение к существующему режиму деперсонализации (псевдонимизации со случайной солью и генерацией синтетических данных) в приложение внедряется полноценный **режим удаления персональных данных (`ModeDelete`)**. В этом режиме обнаруженные персональные данные заменяются на стандартизированный маркер `[УДАЛЕНО]`.

### 1.2. Ключевые требования и сценарии
1. **Единый файловый режим**: Пользователь в интерфейсе переключает режим обработки файла: `[ Замена | Удаление ]`.
2. **Маркер удаления**: Для всех поддерживаемых форматов используется единая константа маркера `[УДАЛЕНО]`.
3. **Обратимость через координатный `.depmap`**: Возможность генерации защищенного паролем файла ключей `.depmap`, позволяющего выполнить обратное восстановление исходных значений.
4. **Устойчивость к обогащению нейросетями (Header-Aware Matching)**:
   - При последующей обработке файла внешними моделями/скриптами (LLM, нейросети, аналитические пайплайны) в таблицу могут быть добавлены новые столбцы (в том числе посередине таблицы).
   - Алгоритм восстановления должен осуществлять поиск колонок по **именам заголовков (`HeaderName`)**, а не по жестким физическим индексам столбцов, как в режиме удаления, так и в режиме замены (псевдонимизации).
   - Добавленные нейросетью столбцы ни при каких обстоятельствах не должны повреждаться или перезаписываться.
5. **Поддержка всех 8 форматов**:
   - Таблицы: Excel (`.xlsx`, `.xlsm`, `.xlsb`, `.xls`), OpenDocument Spreadsheet (`.ods`), CSV/TSV (`.csv`, `.tsv`).
   - Документы: Microsoft Word (`.docx`), OpenDocument Text (`.odt`), Adobe PDF (`.pdf`).
6. **100% Air-Gapped и 152-ФЗ**: Полное соответствие требованиям 152-ФЗ и Приказа РКН № 140 с фиксацией в журнале аудита `audit.jsonl`.

---

## 2. Модели данных и спецификация `.depmap`

### 2.1. Доменные модели (`internal/domain/options.go`)
В пакет `domain` вводится перечисление `ProcessingMode`:

```go
type ProcessingMode string

const (
    ModeReplace ProcessingMode = "replace" // По умолчанию: псевдонимизация синтетическими данными
    ModeDelete  ProcessingMode = "delete"  // Удаление: подстановка маркера [УДАЛЕНО]
)

type ProcessOptions struct {
    InputPath       string           `json:"inputPath"`
    OutputPath      string           `json:"outputPath"`
    Mode            ProcessingMode   `json:"mode"`            // "replace" | "delete"
    Salt            string           `json:"salt"`
    DisabledCols    []int            `json:"disabledCols"`
    DisabledBySheet map[string][]int `json:"disabledBySheet"`
    CreateKey       bool             `json:"createKey"`
    KeyPassword     string           `json:"keyPassword"`
    KeyPath         string           `json:"keyPath"`
    ShuffleRows     bool             `json:"shuffleRows"`
}
```

### 2.2. Расширение криптоконтейнера `pkg/depmap/depmap.go`
Для обеспечения обратимости при удалении и защиты от сдвига колонок структура `DepMap` расширяется новыми полями с обратной совместимостью (тэги `omitempty`):

```go
type DepMap struct {
    Version          int               `json:"version"`                     // 1 для value-only, 2 при наличии координатных/заголовочных маппингов
    CreatedAt        time.Time         `json:"created_at"`
    OriginalFileName string            `json:"original_file_name"`
    RecordsCount     int               `json:"records_count"`
    Mode             string            `json:"mode,omitempty"`             // "replace" | "delete"
    Mapping          map[string]string `json:"mapping,omitempty"`          // Для обратной совместимости и режима замены
    Entities         map[string]string `json:"entities,omitempty"`
    CellMappings     map[string]string `json:"cell_mappings,omitempty"`     // "Sheet!HeaderName:RowIdx" -> originalValue
    TargetHeaders    []string          `json:"target_headers,omitempty"`    // Список защищаемых заголовков (для обоих режимов)
    OrdinalMappings  []string          `json:"ordinal_mappings,omitempty"`  // Для последовательного восстановления документов DOCX/ODT
}
```

- **Спецификация ключа `CellMappings`**:
  Ключ строится в формате `"<Лист>!<Заголовок>:<Номер_строки>"`, например:
  - Excel/ODS: `"Лист1!ФИО:5" -> "Иванов Иван Иванович"`
  - CSV (без листов): `"ФИО:5" -> "Иванов Иван Иванович"`
- **Шифрование**: Без изменений (PBKDF2-HMAC-SHA256, 100 000 итераций, AES-256-GCM + ZLIB).

---

## 3. Ядро обработки и форматы (`pkg/formats`)

### 3.1. Константа маркера удаления
В `pkg/formats/columns.go`:
```go
const DeletionMarker = "[УДАЛЕНО]"
```

### 3.2. Унифицированная функция обработки ячеек
```go
func ProcessCell(anon *anonymizer.Anonymizer, colType ColumnType, val string, mode domain.ProcessingMode, dateRange int) string {
    if strings.TrimSpace(val) == "" || colType == ColUnknown {
        return val
    }
    if mode == domain.ModeDelete {
        return DeletionMarker
    }
    return AnonymizeCell(anon, colType, val, dateRange)
}
```

### 3.3. Обработка таблиц (Excel, CSV, ODS, XLSB, XLS)
1. **При формировании обезличенного файла**:
   - Ячейки с распознанными PII заменяются на `[УДАЛЕНО]`.
   - Если включен `CreateKey = true`:
     Для каждой модифицированной ячейки в `depMap.CellMappings` добавляется запись:
     `depMap.CellMappings[fmt.Sprintf("%s!%s:%d", sheetName, headerName, rowNum)] = originalVal`.
     В `depMap.TargetHeaders` фиксируются уникальные имена заголовков затронутых колонок.

### 3.4. Обработка текстовых документов и PDF
1. **DOCX и ODT**:
   - Чувствительные фрагменты текста (в параграфах, таблицах, колонтитулах) заменяются на `[УДАЛЕНО]`.
   - При создании ключа исходные фрагменты сохраняются в `depMap.OrdinalMappings` в порядке их следования в документе.
2. **PDF**:
   - Физическое удаление текста и наложение плашки вымарывания `[УДАЛЕНО]`.
   - PDF традиционно остается форматом необратимого обезличивания (вымарывания).

---

## 4. Алгоритм восстановления (Header-Aware Resilience)

### 4.1. Восстановление таблиц (`RestoreExcel`, `RestoreCSV`, `RestoreODS`)
Алгоритм восстановления устойчив к добавлению нейросетью новых колонок:

1. **Считывание заголовков**:
   - На этапе инициализации каждого листа считывается строка заголовков (`FindHeaderRow`).
   - Формируется динамическая карта актуальных индексов колонок:
     `currentColMap = map[headerName]currentColIndex`.
2. **Режим удаления (`ModeDelete` с `CellMappings`)**:
   - Для каждой строки `rowNum`:
     Для каждого заголовка из `TargetHeaders` (или ключей `CellMappings` для текущей строки):
       Определяется текущий индекс колонки `colIdx = currentColMap[headerName]`.
       Если ячейка в текущем файле содержит маркер `[УДАЛЕНО]`, она замещается исходным значением из `CellMappings["Sheet!headerName:rowNum"]`.
   - Все добавленные нейросетью столбцы (которых нет в `TargetHeaders`) **вообще не затрагиваются**.
3. **Режим замены (`ModeReplace` с `TargetHeaders`)**:
   - Программа производит словарную замену **строго внутри колонок**, чьи заголовки присутствуют в `TargetHeaders`.
   - Столбцы с выводами, аннотациями и классификацией нейросети защищены от случайных подмен.
4. **Fallback для старых ключей**:
   - Если в `.depmap` отсутствуют `CellMappings` и `TargetHeaders` (ключ v1), используется прежний полнотекстовый поиск по словарю `Mapping`.

### 4.2. Восстановление документов (`RestoreDOCX`, `RestoreODT`)
- Если `len(depMap.OrdinalMappings) > 0`:
  Документ обходится в детерминированном порядке, и каждый маркер `[УДАЛЕНО]` замещается очередным элементом из `OrdinalMappings`.

---

## 5. Пользовательский интерфейс (Svelte 5)

### 5.1. Управление состоянием (`appState.svelte.ts`)
- Добавляется реактивное поле `processingMode = $state<ProcessingMode>('replace')` и метод `setProcessingMode(mode: ProcessingMode)`.

### 5.2. Визуальные элементы UI
1. **Переключатель режима**:
   - Располагается над областью предпросмотра:
     `Режим обработки: [ Замена ] [ Удаление ]`.
   - Строгий лаконичный дизайн без лишних скобок и пояснений.
2. **Динамический предпросмотр (`TableView.svelte`, `TextPreview.svelte`)**:
   - При выборе «Удаление» в предпросмотре колонка «После» моментально отображает `[УДАЛЕНО]`.
3. **Модальное окно сохранения (`SaveDialogModal.svelte`)**:
   - В режиме `Удаление` блок соли скрывается как нерелевантный.
   - Опция создания ключа `.depmap` с шифрованием паролем остается полностью рабочей.

---

## 6. Журнал аудита (152-ФЗ / Приказ РКН № 140)
В `pkg/audit` в структуру записи `AuditRecord` добавляется поле:
- `Mode: "replace" | "delete"`
- Операция деперсонализации сохраняет хэши SHA-256 входного и выходного файлов, дату, статус и перечень сущностей.

---

## 7. Стратегия тестирования (TDD)
1. **Unit-тесты маппинга `.depmap`**:
   - Тест упаковки/распаковки с `CellMappings` и `TargetHeaders`.
   - Тест обратной совместимости с ключами v1.
2. **Интеграционные тесты восстановления со сдвигом колонок**:
   - Создание таблицы с колонками `["ID", "ФИО", "Телефон"]`.
   - Деперсонализация в режиме `delete` с созданием `.depmap`.
   - Имитация работы нейросети: добавление колонок `["Скоринг_ИИ", "Категория"]` между `"ID"` и `"ФИО"`.
   - Запуск `RestoreExcel` / `RestoreCSV`.
   - Проверка: исходные ФИО и Телефон восстановлены точно, колонки `"Скоринг_ИИ"` и `"Категория"` сохранены без изменений.
3. **UI / Frontend проверка**:
   - `npm --prefix frontend run check`
   - `npm --prefix frontend run build`
4. **Регрессионные тесты**:
   - `go test ./...`
