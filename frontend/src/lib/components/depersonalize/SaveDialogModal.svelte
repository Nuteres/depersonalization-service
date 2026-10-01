<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import { SaveFileDialog, SaveKeyFileDialog, ProcessFileWithSheets } from '../../api/wails';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
  }

  const { isOpen, onClose }: Props = $props();

  let salt = $state(appState.currentSalt || '152fz-secure-salt');
  let createKey = $state(true);
  let keyPassword = $state('');
  let outputPath = $state('');
  let keyPath = $state('');
  let selectedExportFormat = $state('xlsx');

  // Initialize selected export format when file changes
  $effect(() => {
    if (appState.currentFileInfo) {
      const ext = (appState.currentFileInfo.extension || 'xlsx').replace('.', '').toLowerCase();
      selectedExportFormat = ext === 'xls' || ext === 'xlsb' ? 'xlsx' : ext;
    }
  });

  const supportedFormats = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (!info) return ['xlsx'];
    if (info.supportedExportFormats && info.supportedExportFormats.length > 0) {
      return info.supportedExportFormats.map((f) => f.replace('.', '').toLowerCase());
    }
    const ext = (info.extension || 'xlsx').replace('.', '').toLowerCase();
    return ext === 'xls' || ext === 'xlsb' ? ['xlsx'] : [ext];
  });

  const formatHint = $derived.by(() => {
    const origExt = (appState.currentFileInfo?.extension || '').replace('.', '').toLowerCase();
    if (origExt === 'xls' || origExt === 'xlsb') {
      return 'Бинарный формат Excel конвертируется в современный открытый стандарт .xlsx без потери данных.';
    }
    if (selectedExportFormat === origExt) {
      return 'Формат файла сохраняется. Все стили, структура и шрифты остаются исходными.';
    }
    return `Данные будут сохранены и преобразованы в формат .${selectedExportFormat}.`;
  });

  function getFormatDisplayName(fmt: string): string {
    switch (fmt.toLowerCase()) {
      case 'xlsx':
        return 'Таблица Microsoft Excel (.xlsx)';
      case 'ods':
        return 'Таблица OpenDocument Spreadsheet (.ods)';
      case 'csv':
        return 'Текстовая таблица CSV (.csv)';
      case 'docx':
        return 'Документ Microsoft Word (.docx)';
      case 'odt':
        return 'Документ OpenDocument Text (.odt)';
      case 'pdf':
        return 'Документ Adobe PDF (.pdf)';
      default:
        return fmt.toUpperCase();
    }
  }

  function generateRandomSalt() {
    const arr = new Uint8Array(8);
    crypto.getRandomValues(arr);
    salt = Array.from(arr, (b) => b.toString(16).padStart(2, '0')).join('');
  }

  async function handleSelectOutputPath() {
    const ext = selectedExportFormat;
    const baseName = appState.currentFileInfo?.name.replace(/\.[^/.]+$/, '') || 'data';
    const defaultName = `${baseName}_anonymized.${ext}`;
    const chosen = await SaveFileDialog(defaultName);
    if (chosen) {
      outputPath = chosen;
    }
  }

  async function handleSelectKeyPath() {
    const baseName = appState.currentFileInfo?.name.replace(/\.[^/.]+$/, '') || 'data';
    const defaultName = `${baseName}_anonymized.depmap`;
    const chosen = await SaveKeyFileDialog(defaultName);
    if (chosen) {
      keyPath = chosen;
    }
  }

  async function handleStart() {
    if (!appState.currentFileInfo || !appState.currentFilePath) return;

    let destPath = outputPath;
    if (!destPath) {
      const ext = selectedExportFormat;
      const baseName = appState.currentFileInfo.name.replace(/\.[^/.]+$/, '');
      const defaultName = `${baseName}_anonymized.${ext}`;
      destPath = await SaveFileDialog(defaultName);
      if (!destPath) {
        // User cancelled file dialog
        return;
      }
    }

    // Save chosen salt to appState
    appState.setSalt(salt);

    // Calculate total rows across sheets
    let totalRows = appState.currentFileInfo.rowCount || 0;
    if (appState.currentFileInfo.sheets && appState.currentFileInfo.sheets.length > 1) {
      totalRows = appState.currentFileInfo.sheets.reduce(
        (acc, s) => acc + (s.rowCount || 0),
        0
      );
    }

    // Close this save options modal
    onClose();

    // Start progress modal
    appState.startProgress('Деперсонализация ПДн');
    appState.updateProgress(0, 0, totalRows);

    try {
      // Build disabled columns arrays
      const activeSheetName =
        appState.currentFileInfo.sheets?.[tableState.activeSheetIndex]?.name || '';
      const globalDisabled = tableState.disabledColumnsBySheet[activeSheetName] || [];
      const disabledBySheet = tableState.disabledColumnsBySheet;

      const result = await ProcessFileWithSheets(
        appState.currentFilePath,
        destPath,
        salt,
        globalDisabled,
        disabledBySheet,
        createKey,
        keyPassword,
        keyPath
      );

      if (result && result.success) {
        appState.showSuccess(result);
        appState.showToast('Деперсонализация успешно завершена!', 'success');
      } else {
        appState.finishProgress();
        appState.showToast(result?.error || 'Ошибка при деперсонализации', 'error');
      }
    } catch (err: any) {
      console.error('Error during depersonalization:', err);
      appState.finishProgress();
      const errStr = String(err);
      if (errStr.includes('context canceled') || errStr.includes('canceled') || errStr.includes('отменена')) {
        appState.showToast('Операция отменена', 'info');
      } else {
        appState.showToast(`Ошибка деперсонализации: ${err}`, 'error');
      }
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && isOpen) {
      onClose();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeydown);
    return () => {
      window.removeEventListener('keydown', handleKeydown);
    };
  });
</script>

{#if isOpen && appState.currentFileInfo}
  <div
    id="save-options-modal"
    class="fixed inset-0 bg-black/40 backdrop-blur-[2px] flex items-center justify-center p-4 z-50 overscroll-contain"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    aria-labelledby="save-modal-title"
    onclick={(e) => {
      if (e.target === e.currentTarget) onClose();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') onClose();
    }}
  >
    <div
      class="bg-white border border-[#E8E7E3] shadow-2xl max-w-lg w-full p-6 flex flex-col gap-4 max-h-[90vh] overflow-y-auto custom-scrollbar"
    >
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <h2 id="save-modal-title" class="text-sm font-bold text-[#1C1917]">
          Параметры сохранения и деперсонализации
        </h2>
        <button
          type="button"
          onclick={onClose}
          class="text-stone-400 hover:text-stone-700 text-sm font-bold cursor-pointer"
        >
          ✕
        </button>
      </div>

      <div class="space-y-4 text-xs">
        <!-- Input File Meta Box -->
        <div class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] flex items-center justify-between font-mono">
          <span class="font-bold text-[#1C1917] truncate max-w-[280px]" title={appState.currentFileInfo.name}>
            {appState.currentFileInfo.name}
          </span>
          <span class="text-[#78716C] shrink-0 ml-2">
            {#if appState.currentFileInfo.sheets && appState.currentFileInfo.sheets.length > 1}
              {appState.currentFileInfo.sheets.length} листов
            {:else if appState.currentFileInfo.fileType === 'text'}
              {(appState.currentFileInfo.paragraphCount || appState.currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} абз.
            {:else if appState.currentFileInfo.fileType === 'pdf'}
              {appState.currentFileInfo.pageCount} стр.
            {:else}
              {(appState.currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} строк
            {/if}
          </span>
        </div>

        <!-- Format Export Selection Block -->
        <div class="space-y-1.5 border border-[#E8E7E3] p-4 bg-white">
          <label
            for="export-format-select"
            class="block text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider"
          >
            Формат сохранения:
          </label>
          <select
            id="export-format-select"
            bind:value={selectedExportFormat}
            class="w-full px-3 py-2 border border-[#E8E7E3] bg-[#FAF9F5] text-xs font-mono text-[#1C1917] focus:outline-none focus:border-[#1C1917] cursor-pointer"
          >
            {#each supportedFormats as fmt}
              <option value={fmt}>{getFormatDisplayName(fmt)}</option>
            {/each}
          </select>
          <div class="text-[11px] text-[#78716C] mt-1 font-sans">
            {formatHint}
          </div>
        </div>

        <!-- Salt Field with Random Salt Generator -->
        <div class="space-y-2 border border-[#E8E7E3] p-4 bg-white">
          <div class="flex items-center justify-between">
            <label
              for="salt-input"
              class="block text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider"
            >
              Ключ целостности (соль хеширования):
            </label>
            <button
              type="button"
              onclick={generateRandomSalt}
              class="text-[11px] text-[#065F46] hover:text-[#047857] font-semibold underline cursor-pointer"
            >
              🎲 Случайная соль
            </button>
          </div>
          <input
            id="salt-input"
            type="text"
            bind:value={salt}
            placeholder="Введите или сгенерируйте соль..."
            class="w-full px-3 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5]"
          />
          <div class="text-[10px] text-[#78716C]">
            Используется для детерминированного псевдонимизирования. Одинаковая соль гарантирует воспроизводимость замен.
          </div>
        </div>

        <!-- Output Destination File Path -->
        <div class="space-y-1.5 border border-[#E8E7E3] p-4 bg-white">
          <label
            for="output-path-input"
            class="block text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider"
          >
            Путь для сохранения файла:
          </label>
          <div class="flex items-center gap-2">
            <input
              id="output-path-input"
              type="text"
              bind:value={outputPath}
              placeholder="По умолчанию рядом с исходным файлом..."
              class="flex-1 px-3 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
            />
            <button
              type="button"
              onclick={handleSelectOutputPath}
              class="px-3 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold whitespace-nowrap cursor-pointer shadow-2xs"
            >
              Обзор...
            </button>
          </div>
        </div>

        <!-- Key Generation Option (.depmap) -->
        <div class="space-y-3 border border-[#E8E7E3] p-4 bg-white">
          <label class="flex items-start space-x-2.5 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={createKey}
              class="mt-0.5 accent-[#1C1917] h-4 w-4 rounded-none cursor-pointer"
            />
            <div>
              <div class="font-bold text-[#1C1917] text-xs">
                Создать защищенный ключ восстановления (.depmap)
              </div>
              <div class="text-[11px] text-[#78716C] mt-0.5 leading-relaxed">
                Позволяет в будущем выполнить обратную персонализацию (деобезличивание) файла после разметки аналитиками или ML-моделями.
              </div>
            </div>
          </label>

          {#if createKey}
            <div class="pt-3 border-t border-[#F0EFEB] space-y-3">
              <!-- Key file path destination -->
              <div class="space-y-1.5">
                <label
                  for="key-path-input"
                  class="block text-[11px] font-mono text-[#57534E]"
                >
                  Путь для сохранения ключа (.depmap):
                </label>
                <div class="flex items-center gap-2">
                  <input
                    id="key-path-input"
                    type="text"
                    bind:value={keyPath}
                    placeholder="Рядом с обработанным файлом..."
                    class="flex-1 px-3 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
                  />
                  <button
                    type="button"
                    onclick={handleSelectKeyPath}
                    class="px-3 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold whitespace-nowrap cursor-pointer shadow-2xs"
                  >
                    Обзор...
                  </button>
                </div>
              </div>

              <!-- Password for Key -->
              <div class="space-y-1.5">
                <label
                  for="key-password-input"
                  class="block text-[11px] font-mono text-[#57534E]"
                >
                  Пароль шифрования ключа (необязательно):
                </label>
                <input
                  id="key-password-input"
                  type="password"
                  bind:value={keyPassword}
                  placeholder="Оставьте пустым для надежного авто-ключа..."
                  class="w-full px-3 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5]"
                />
                <div class="text-[10px] text-[#A8A29E]">
                  Контейнер ключа шифруется по стандарту AES-256-GCM + PBKDF2 (100 000 итераций).
                </div>
              </div>
            </div>
          {/if}
        </div>
      </div>

      <!-- Footer Buttons -->
      <div class="flex items-center justify-end space-x-2 pt-3 border-t border-[#F0EFEB]">
        <button
          type="button"
          onclick={onClose}
          class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold transition cursor-pointer"
        >
          Отмена
        </button>
        <button
          type="button"
          onclick={handleStart}
          class="px-5 py-2 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold transition cursor-pointer flex items-center space-x-1.5 shadow-sm active:scale-[0.99]"
        >
          <span>💾</span>
          <span>Начать обезличивание</span>
        </button>
      </div>
    </div>
  </div>
{/if}
