# Проектная спецификация: Функция создания защищённого паролем ZIP-архива

**Дата:** 2026-10-16  
**Версия приложения:** 1.12.0 (MINOR update с 1.11.1)  
**Статус:** Согласовано (Ready for planning)

---

## 1. Контекст и цель (Intent & The Why)

### Пользовательские сценарии:
1. **Передача данных в LLM / нейросеть**: Пользователь обрабатывает файл и загружает очищенный документ в веб-интерфейс или локальную модель. Упаковка в архив не требуется.
2. **Передача деперсонализированных данных внешним контрагентам по e-mail**: Почтовые DLP-системы, антивирусные сканеры и корпоративные почтовые шлюзы безопасности («служба безопасности») при получении файлов от внешних отправителей часто блокируют или отфильтровывают офисные документы, если они передаются в открытом виде. Стандартный защищённый паролем ZIP-архив решает эту задачу — он пропускается алгоритмами безопасности и защищает данные в канале передачи.

### Цель фичи:
Дать пользователю возможность в окне «Параметры сохранения» переключателем включить упаковку деперсонализированного документа в защищённый паролем универсальный ZIP-архив с гибким заданием или автогенерацией пароля, выводом пароля и пути в финальном окне успешного сохранения для быстрого копирования в буфер обмена.

---

## 2. Требования и границы функционала

### Цели (Goals):
- **Опциональность**: Возможность включать/выключать архивацию в окне «Параметры сохранения» перед началом деперсонализации (`createArchive: boolean`, по умолчанию выключено).
- **Универсальность архива**: Использование формата `.zip` со стандартным шифрованием PKWARE (`zip.StandardEncryption` / ZipCrypto), гарантирующим открываемость на любой операционной системе (Windows Проводник «Извлечь всё», macOS Archive Utility, Linux `unzip -P`, Android, iOS, а также 7-Zip/WinRAR) без необходимости установки стороннего ПО.
- **Изоляция содержимого**: В архив упаковывается **исключительно деперсонализированный файл**. Ключ восстановления `.depmap` в архив **НЕ помещается** и остаётся у отправителя.
- **Сохранение обоих файлов**: На диске сохраняются оба файла — распакованный деперсонализированный файл и защищённый паролем `.zip` архив рядом с ним.
- **Гибкая политика пароля архива**:
  - При ручном вводе: гибкие требования — от 4 символов, только ASCII-символы (латиница, цифры, знаки препинания, без кириллицы и пробелов во избежание проблем с кодировками между UTF-8 в Go и CP1251/CP866 в русском Windows Explorer).
  - При автогенерации (кнопка 🎲 или если поле оставлено пустым): генерация надёжного 16-значного криптостойкого пароля (`depmap.GenerateRandomPassword(16)`).
  - Удобный UI: кнопка просмотра пароля (глазик), кнопка автогенерации (🎲), кнопка копирования в буфер (📋).
- **Экран завершения (`SuccessModal`)**:
  - Карточка созданного архива с путём и кнопкой «Открыть папку»;
  - Отображение пароля архива с кнопкой «Скопировать пароль» и возможностью скрыть/показать значение.
- **Выбор пути**: Возможность выбрать путь для архива через системный диалог или использовать путь по умолчанию (`<имя_файла>_anonymized.zip`).
- **Поддержка всех типов файлов**: Архивация доступна для таблиц (.xlsx, .xls, .xlsb, .ods, .csv), текстов (.docx, .odt, .txt) и PDF.

### Не-цели (Non-Goals):
- Создание архивов других форматов (.7z, .rar, .tar.gz).
- Добавление файла `.depmap` внутрь архива.
- Архивация при обратном восстановлении (в режиме Restore архив не создаётся).

### Анти-цели (Anti-Goals):
- Замена деперсонализированного файла архивом (сохраняются оба).
- Использование WinZip AES-256 в ZIP, так как стандартный Проводник Windows и macOS не могут открыть его штатно.

---

## 3. Архитектурные решения и компоненты

### 3.1. Go Backend

1. **Новый пакет `pkg/ziputil`**:
   - `CreatePasswordZip(srcFilePath, destZipPath, password string) error`.
   - Зависимость: `github.com/yeka/zip` (завендорена в `vendor/`), метод `w.Encrypt(filename, password, zip.StandardEncryption)`.
   - Файл упаковывается с базовым именем `filepath.Base(srcFilePath)`.
   - Безопасное создание через временный файл `.tmp` с атомарным переименованием и очисткой при ошибке.
   - Функция валидации пароля архива: `ValidateArchivePassword(password string) error` (проверка: ASCII, длина >= 4, без пробелов).

2. **Модели домена (`internal/domain`)**:
   - `ProcessOptions`:
     ```go
     CreateArchive   bool   `json:"createArchive"`
     ArchivePassword string `json:"archivePassword"`
     ArchivePath     string `json:"archivePath,omitempty"`
     ```
   - `ProcessResult`:
     ```go
     ArchivePath              string `json:"archivePath,omitempty"`
     ArchivePassword          string `json:"archivePassword,omitempty"`
     ArchivePasswordGenerated bool   `json:"archivePasswordGenerated,omitempty"`
     ```

3. **Интеграция в `Processor.Process` (`internal/service/processor/processor.go`)**:
   - После сохранения деперсонализированного файла и ключа:
   - Если `opts.CreateArchive == true`:
     - Определяется путь: если `opts.ArchivePath` пуст, берется `strings.TrimSuffix(outPath, filepath.Ext(outPath)) + ".zip"`.
     - Определяется пароль: если `opts.ArchivePassword` пуст, генерируется `depmap.GenerateRandomPassword(16)` и `ArchivePasswordGenerated = true`. Если передан — валидируется `ValidateArchivePassword`.
     - Создается архив через `ziputil.CreatePasswordZip(outPath, archivePath, effectivePassword)`.
     - В `ProcessResult` записываются `archivePath`, `archivePassword`, `archivePasswordGenerated`.

4. **Wails API (`internal/desktop/app.go`)**:
   - Метод `SaveArchiveFileDialog(defaultFilename string) (string, error)` для вызова нативного диалога выбора пути `.zip`.
   - Метод `GenerateArchivePassword() (string, error)` для генерации пароля в UI.
   - Обновление параметров и вызовов `ProcessFileWithSheets` и `ProcessFileWithOptions`.

### 3.2. Svelte 5 Frontend

1. **`SaveDialogModal.svelte`**:
   - Состояния:
     - `createArchive = $state(false)`
     - `archivePassword = $state('')`
     - `showArchivePassword = $state(false)`
     - `archivePath = $state('')`
     - `archivePasswordError = $state('')`
   - Блок интерфейса:
     - Чекбокс «📦 Упаковать в ZIP-архив с паролем» с бейджем «Для отправки по почте / СБ»;
     - При включении:
       - Поле ввода пароля с кнопками показа (глаз), генерации (🎲) и копирования (📋);
       - Поле выбора пути архива с кнопкой «Обзор...»;
       - Валидация пароля (только ASCII, мин. 4 символа) с информативной подсказкой.
   - Передача `createArchive`, `archivePassword`, `archivePath` в бэкенд.

2. **`SuccessModal.svelte`**:
   - Если в результате есть `archivePath`:
     - Секция «Защищённый архив для отправки»:
       - Путь к архиву с кнопкой «Открыть папку»;
       - Пароль архива с кнопкой «Скопировать пароль» и иконкой скрытия/показа.

3. **TypeScript типы (`frontend/src/lib/types/domain.ts`)**:
   - Синхронизация типов с `domain.ProcessOptions` и `domain.ProcessResult`.

---

## 4. Ограничения и правила репозитория (Constraints)

- **100% Air-Gapped**: Приложение не выполняет внешних сетевых запросов.
- **Go Vendor**: Библиотека `github.com/yeka/zip` добавляется в `go.mod` и вендорится в `vendor/`.
- **Семантическое версионирование**:
  - `internal/version/version.go`: бамп до `1.12.0` (MINOR).
  - `frontend/package.json`: `"version": "1.12.0"`.
  - `Makefile`: `VERSION = 1.12.0`.
- **CHANGELOG.md**: Запись в формате Keep a Changelog.
- **Документация**:
  - `docs/USER_GUIDE.md`: описание настройки архивации с паролем и сценария отправки по почте.
  - `docs/ARCHITECTURE.md`: описание модуля `pkg/ziputil` и обновлённых структур данных.
  - `README.md`: отражение новой возможности.
- **Проверка качества**:
  - `npm --prefix frontend run check`
  - `npm --prefix frontend run build`
  - `go test ./...`
  - `make build-macos` и `make build-windows` (свежие бинарники в `bin/`).
