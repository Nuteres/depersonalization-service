<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import { SaveFileDialog, SaveKeyFileDialog, ProcessFileWithSheets, ValidateKeyPassword } from '../../api/wails';
  import type { PasswordValidationResult } from '../../types/domain';

  interface Props {
    isOpen: boolean;
    onClose: () => void;
  }

  const { isOpen, onClose }: Props = $props();

  let salt = $state(appState.currentSalt || '');
  let shuffleRows = $state(false);
  let createKey = $state(true);
  let keyPassword = $state('');
  let showPassword = $state(false);
  let passwordValidation = $state<PasswordValidationResult>({
    valid: true,
    score: 0,
    errors: [],
    minLength: false,
    hasUppercase: false,
    hasLowercase: false,
    hasDigit: false,
    hasSpecial: false,
    onlyLatin: false,
  });
  let outputPath = $state('');
  let keyPath = $state('');
  let selectedExportFormat = $state('xlsx');

  // Real-time password validation
  $effect(() => {
    const trimmed = keyPassword.trim();
    if (!trimmed) {
      passwordValidation = {
        valid: true,
        score: 0,
        errors: [],
        minLength: false,
        hasUppercase: false,
        hasLowercase: false,
        hasDigit: false,
        hasSpecial: false,
        onlyLatin: false,
      };
    } else {
      ValidateKeyPassword(keyPassword).then((res) => {
        passwordValidation = res;
      });
    }
  });

  const isPasswordValid = $derived(!createKey || !keyPassword.trim() || passwordValidation.valid);

  // Generate random salt on open if not already set
  $effect(() => {
    if (isOpen) {
      if (!salt || salt === '152fz-secure-salt') {
        generateRandomSalt();
      }
    }
  });

  // Initialize selected export format when file changes
  $effect(() => {
    if (appState.currentFileInfo) {
      const ext = (appState.currentFileInfo.extension || 'xlsx').replace('.', '').toLowerCase();
      selectedExportFormat = ext === 'xls' || ext === 'xlsb' ? 'xlsx' : ext;
      if (appState.currentFileInfo.fileType !== 'table') {
        shuffleRows = false;
      }
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
    if (!isPasswordValid) return;

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
    appState.startProgress(
      appState.processingMode === 'delete' ? 'Удаление ПДн' : 'Деперсонализация ПДн'
    );
    appState.updateProgress(0, 0, totalRows);

    try {
      // Build disabled columns arrays
      const activeSheetName =
        appState.currentFileInfo.sheets?.[tableState.activeSheetIndex]?.name || '';
      const globalDisabled = tableState.disabledColumnsBySheet[activeSheetName] || [];
      const disabledBySheet = tableState.disabledColumnsBySheet;

      const effectiveMode =
        appState.processingMode === 'delete' ? 'delete' : (appState.strategy || 'tokens');
      const effectiveShuffle =
        appState.currentFileInfo.fileType === 'table' ? shuffleRows : false;

      const result = await ProcessFileWithSheets(
        appState.currentFilePath,
        destPath,
        salt,
        globalDisabled,
        disabledBySheet,
        createKey,
        keyPassword,
        keyPath,
        effectiveShuffle,
        effectiveMode
      );

      if (result && result.success) {
        appState.showSuccess(result);
        appState.showToast(
          appState.processingMode === 'delete'
            ? 'Удаление ПДн успешно завершено!'
            : 'Деперсонализация успешно завершена!',
          'success'
        );
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
          Параметры сохранения
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
          <span class="font-bold text-[#1C1917] truncate flex-1 min-w-0" title={appState.currentFileInfo.name}>
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

        <!-- Shuffle Rows Toggle (Only for tabular files: Excel, CSV, ODS) -->
        {#if appState.currentFileInfo.fileType === 'table'}
          <div class="border border-[#E8E7E3] p-4 bg-white">
            <label class="flex items-start space-x-2.5 cursor-pointer select-none">
              <input
                id="shuffle-rows-checkbox"
                type="checkbox"
                bind:checked={shuffleRows}
                class="mt-0.5 accent-[#1C1917] h-4 w-4 rounded-none cursor-pointer"
              />
              <div>
                <div class="flex items-center gap-2">
                  <span class="font-bold text-[#1C1917] text-xs">
                    Перемешать строки
                  </span>
                  <span class="inline-flex items-center px-1.5 py-0.5 text-[10px] font-mono font-medium bg-stone-100 text-stone-600 border border-stone-200">
                    РКН № 140
                  </span>
                </div>
                <div class="text-[11px] text-[#78716C] mt-1 leading-relaxed font-sans">
                  Случайный порядок записей (метод перемешивания по Приказу Роскомнадзора № 140) для защиты от деанонимизации по позиции в реестре. Данные внутри строк расшифровываются корректно, но порядок строк остаётся перемешанным.
                </div>
              </div>
            </label>
          </div>
        {/if}

        <!-- Key Generation Option -->
        <div class="space-y-3 border border-[#E8E7E3] p-4 bg-white">
          <label class="flex items-start space-x-2.5 cursor-pointer select-none">
            <input
              id="create-key-checkbox"
              type="checkbox"
              bind:checked={createKey}
              class="mt-0.5 accent-[#1C1917] h-4 w-4 rounded-none cursor-pointer"
            />
            <div>
              <div class="font-bold text-[#1C1917] text-xs">
                Создать ключ восстановления
              </div>
              <div class="text-[11px] text-[#78716C] mt-0.5 leading-relaxed">
                Позволяет в будущем восстановить исходные данные.
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
                  Путь для сохранения ключа:
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
                <div class="flex items-center justify-between">
                  <label
                    for="key-password-input"
                    class="block text-[11px] font-mono text-[#57534E]"
                  >
                    Пароль ключа:
                  </label>
                  {#if keyPassword.trim()}
                    <span class="text-[10px] font-mono font-medium">
                      {#if passwordValidation.score <= 1}
                        <span class="text-[#DC2626]">Слабый</span>
                      {:else if passwordValidation.score === 2}
                        <span class="text-[#D97706]">Средний</span>
                      {:else if passwordValidation.score === 3}
                        <span class="text-[#2563EB]">Хороший</span>
                      {:else}
                        <span class="text-[#16A34A]">Надежный</span>
                      {/if}
                    </span>
                  {/if}
                </div>

                <div class="relative flex items-center">
                  <input
                    id="key-password-input"
                    type={showPassword ? 'text' : 'password'}
                    bind:value={keyPassword}
                    placeholder="Оставьте пустым для автогенерации"
                    class="w-full pr-8 px-3 py-2 border {keyPassword.trim() && !passwordValidation.valid ? 'border-[#DC2626] focus:border-[#DC2626]' : 'border-[#E8E7E3] focus:border-[#1C1917]'} focus:outline-none text-xs font-mono bg-[#FAF9F5]"
                  />
                  {#if keyPassword}
                    <button
                      type="button"
                      onclick={() => showPassword = !showPassword}
                      class="absolute right-2 text-[#78716C] hover:text-[#1C1917] p-1 text-xs select-none cursor-pointer"
                      title={showPassword ? 'Скрыть пароль' : 'Показать пароль'}
                      aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}
                    >
                      {#if showPassword}
                        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l18 18" />
                        </svg>
                      {:else}
                        <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                        </svg>
                      {/if}
                    </button>
                  {/if}
                </div>

                <!-- Password strength indicator bars -->
                {#if keyPassword.trim()}
                  <div class="grid grid-cols-4 gap-1 pt-0.5">
                    <div class="h-1 rounded-xs transition-colors {passwordValidation.score >= 1 ? (passwordValidation.score === 1 ? 'bg-[#DC2626]' : passwordValidation.score === 2 ? 'bg-[#D97706]' : passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                    <div class="h-1 rounded-xs transition-colors {passwordValidation.score >= 2 ? (passwordValidation.score === 2 ? 'bg-[#D97706]' : passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                    <div class="h-1 rounded-xs transition-colors {passwordValidation.score >= 3 ? (passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                    <div class="h-1 rounded-xs transition-colors {passwordValidation.score >= 4 ? 'bg-[#16A34A]' : 'bg-[#E7E5E4]'}"></div>
                  </div>

                  <!-- Requirements checklist -->
                  <div class="mt-2 bg-[#FAF9F5] p-2.5 border border-[#E8E7E3] text-[11px] font-sans">
                    <div class="font-medium text-[#44403C] mb-1.5 text-[10.5px]">Требования к паролю:</div>
                    <div class="grid grid-cols-2 gap-x-2 gap-y-1">
                      <div class="flex items-center space-x-1.5 {passwordValidation.minLength ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.minLength ? '✓' : '▪'}</span>
                        <span>12+ символов</span>
                      </div>
                      <div class="flex items-center space-x-1.5 {passwordValidation.onlyLatin ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.onlyLatin ? '✓' : '▪'}</span>
                        <span>Только латиница</span>
                      </div>
                      <div class="flex items-center space-x-1.5 {passwordValidation.hasUppercase ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.hasUppercase ? '✓' : '▪'}</span>
                        <span>Заглавные (A-Z)</span>
                      </div>
                      <div class="flex items-center space-x-1.5 {passwordValidation.hasLowercase ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.hasLowercase ? '✓' : '▪'}</span>
                        <span>Строчные (a-z)</span>
                      </div>
                      <div class="flex items-center space-x-1.5 {passwordValidation.hasDigit ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.hasDigit ? '✓' : '▪'}</span>
                        <span>Цифры (0-9)</span>
                      </div>
                      <div class="flex items-center space-x-1.5 {passwordValidation.hasSpecial ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                        <span>{passwordValidation.hasSpecial ? '✓' : '▪'}</span>
                        <span>Спецсимволы (!@#$%...)</span>
                      </div>
                    </div>

                    {#if keyPassword.trim() && !passwordValidation.onlyLatin}
                      <div class="mt-2 pt-1.5 border-t border-[#E8E7E3] text-[10.5px] text-[#DC2626] font-medium flex items-center space-x-1">
                        <span>⚠</span>
                        <span>Кириллица и пробелы запрещены — используйте только латиницу</span>
                      </div>
                    {/if}
                  </div>
                {:else}
                  <p class="text-[11px] text-[#78716C] leading-relaxed">
                    Минимум 12 символов: только латиница (A-Z, a-z), цифры и спецсимволы. Если оставить пустым — пароль будет сгенерирован автоматически.
                  </p>
                {/if}
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
          disabled={!isPasswordValid}
          class="px-5 py-2 text-xs font-semibold transition flex items-center gap-1.5 shadow-sm {isPasswordValid ? 'bg-[#1C1917] hover:bg-[#292524] text-white cursor-pointer active:scale-[0.99]' : 'bg-[#E7E5E4] text-[#A8A29E] cursor-not-allowed border border-[#D6D3D1]'}"
        >
          <span class="text-xs leading-none shrink-0 inline-flex items-center justify-center select-none">💾</span>
          <span>{appState.processingMode === 'delete' ? 'Начать удаление' : 'Начать обезличивание'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
