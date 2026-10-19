<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import {
    SaveFileDialog,
    SaveKeyFileDialog,
    SaveArchiveFileDialog,
    GenerateArchivePassword,
    ValidateArchivePassword,
    ProcessFileWithSheets,
    ProcessFileWithOptions,
    ValidateKeyPassword
  } from '../../api/wails';
  import { copyToClipboard } from '../../services/clipboard';
  import type { PasswordValidationResult } from '../../types/domain';
  import { getDefaultOutputPath } from '../../utils/path';

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

  // Archive state
  let createArchive = $state(false);
  let archivePassword = $state('');
  let showArchivePassword = $state(false);
  let archivePath = $state('');

  function getArchivePasswordError(pwd: string): string {
    const trimmed = pwd.trim();
    if (!trimmed) return '';
    if (pwd.length < 4) return 'Пароль архива должен содержать не менее 4 символов';
    if (pwd !== trimmed) return 'Пароль архива не должен содержать пробелов';
    for (let i = 0; i < pwd.length; i++) {
      const code = pwd.charCodeAt(i);
      if (code < 33 || code > 126) {
        return 'Пароль архива может содержать только латиницу, цифры и спецсимволы без пробелов';
      }
    }
    return '';
  }

  const archivePasswordError = $derived(
    createArchive && archivePassword.trim() ? getArchivePasswordError(archivePassword) : ''
  );

  // Real-time password validation for key
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

  const isPasswordValid = $derived(
    appState.currentFileInfo?.fileType === 'pdf' ||
    !createKey ||
    !keyPassword.trim() ||
    passwordValidation.valid
  );

  const isFormValid = $derived(
    isPasswordValid &&
    (!createArchive || !archivePassword.trim() || archivePasswordError === '')
  );

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
      const formats = supportedFormats;
      if (formats && formats.length > 0 && formats[0]) {
        selectedExportFormat = formats[0];
      } else {
        const ext = (appState.currentFileInfo.extension || 'xlsx').replace('.', '').toLowerCase();
        selectedExportFormat = ext === 'xls' || ext === 'xlsb' ? 'xlsx' : ext;
      }
      if (appState.currentFileInfo.fileType !== 'table') {
        shuffleRows = false;
      }
      if (appState.currentFileInfo.fileType === 'pdf') {
        createKey = false;
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

  function handleSelectFormat(fmt: string) {
    selectedExportFormat = fmt;
    if (outputPath) {
      outputPath = outputPath.replace(/\.[^/.]+$/, `.${fmt}`);
    }
  }

  const formatHint = $derived.by(() => {
    const origExt = (appState.currentFileInfo?.extension || '').replace('.', '').toLowerCase();
    if (origExt === 'xls' || origExt === 'xlsb') {
      return 'Бинарный формат Excel конвертируется в открытый формат .xlsx.';
    }
    if (selectedExportFormat === origExt) {
      return '';
    }
    return `Данные будут сохранены в формат .${selectedExportFormat}.`;
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
    const defaultPath = getDefaultOutputPath(appState.currentFilePath || '', ext);
    const chosen = await SaveFileDialog(defaultPath);
    if (chosen) {
      outputPath = chosen;
    }
  }

  async function handleSelectKeyPath() {
    const baseSource = outputPath.trim() || appState.currentFilePath || '';
    const defaultPath = getDefaultOutputPath(baseSource, 'depmap');
    const chosen = await SaveKeyFileDialog(defaultPath);
    if (chosen) {
      keyPath = chosen;
    }
  }

  async function handleSelectArchivePath() {
    const baseSource = outputPath.trim() || appState.currentFilePath || '';
    const defaultPath = getDefaultOutputPath(baseSource, 'zip');
    const chosen = await SaveArchiveFileDialog(defaultPath);
    if (chosen) {
      archivePath = chosen;
    }
  }

  async function handleGenerateArchivePassword() {
    try {
      const pwd = await GenerateArchivePassword();
      if (pwd) {
        archivePassword = pwd;
      }
    } catch (err) {
      console.error('Failed to generate archive password:', err);
    }
  }

  async function handleCopyArchivePassword() {
    if (!archivePassword) return;
    const ok = await copyToClipboard(archivePassword);
    if (ok) {
      appState.showToast('Пароль архива скопирован в буфер обмена!', 'success');
    }
  }

  async function handleStart() {
    if (!appState.currentFileInfo || !appState.currentFilePath) return;
    if (!isFormValid) return;

    let destPath = outputPath.trim();
    if (destPath) {
      const curExt = destPath.split('.').pop()?.toLowerCase();
      if (curExt !== selectedExportFormat) {
        destPath = `${destPath.replace(/\.[^/.]+$/, '')}.${selectedExportFormat}`;
      }
    } else {
      destPath = getDefaultOutputPath(appState.currentFilePath, selectedExportFormat);
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

      const isPdf = appState.currentFileInfo.fileType === 'pdf';
      const effectiveCreateKey = isPdf ? false : createKey;
      const effectiveKeyPassword = effectiveCreateKey ? keyPassword : '';
      const effectiveKeyPath = effectiveCreateKey
        ? (keyPath.trim() || getDefaultOutputPath(destPath, 'depmap'))
        : '';
      const effectiveArchivePath = createArchive
        ? (archivePath.trim() || getDefaultOutputPath(destPath, 'zip'))
        : '';

      let result;
      if (createArchive) {
        result = await ProcessFileWithOptions({
          inputPath: appState.currentFilePath,
          outputPath: destPath,
          mode: appState.processingMode,
          strategy: effectiveMode,
          salt,
          disabledCols: globalDisabled,
          disabledBySheet,
          createKey: effectiveCreateKey,
          keyPassword: effectiveKeyPassword,
          keyPath: effectiveKeyPath,
          shuffleRows: effectiveShuffle,
          createArchive: true,
          archivePassword: archivePassword.trim(),
          archivePath: effectiveArchivePath,
          pdfRedactionBoxes: isPdf ? (appState.pdfRedactionBoxes || []) : undefined,
          pdfRedactTerms: isPdf ? (appState.pdfRedactTerms || []) : undefined,
        });
      } else if (isPdf && appState.pdfRedactionBoxes && appState.pdfRedactionBoxes.length > 0) {
        result = await ProcessFileWithOptions({
          inputPath: appState.currentFilePath,
          outputPath: destPath,
          mode: appState.processingMode,
          strategy: effectiveMode,
          salt,
          createKey: false,
          pdfRedactionBoxes: appState.pdfRedactionBoxes,
          pdfRedactTerms: appState.pdfRedactTerms || []
        });
      } else {
        result = await ProcessFileWithSheets(
          appState.currentFilePath,
          destPath,
          salt,
          globalDisabled,
          disabledBySheet,
          effectiveCreateKey,
          effectiveKeyPassword,
          effectiveKeyPath,
          effectiveShuffle,
          effectiveMode
        );
      }

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
    class="fixed inset-0 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4 z-50 overscroll-contain transition-opacity"
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
      class="bg-white border border-[#E8E7E3] rounded-lg shadow-2xl max-w-lg w-full p-6 flex flex-col gap-4 max-h-[calc(100%-2rem)] overflow-y-auto custom-scrollbar animate-surface-enter"
    >
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <h2 id="save-modal-title" class="text-sm font-bold text-[#1C1917]">
          Параметры сохранения
        </h2>
        <button
          type="button"
          onclick={onClose}
          class="text-stone-400 hover:text-stone-700 p-1 rounded-md hover:bg-stone-100 transition pressable cursor-pointer"
          title="Закрыть"
        >
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <div class="space-y-3.5 text-xs">
        <!-- Input File Meta Box -->
        <div class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg flex items-center justify-between font-mono">
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

        <!-- Format Export Selection Block (shown only when multiple formats are supported) -->
        {#if supportedFormats.length > 1}
          <div class="space-y-2 border border-[#E8E7E3] rounded-lg p-3.5 bg-white">
            <div class="text-xs font-semibold text-[#1C1917]">
              Формат сохранения
            </div>
            <div class="grid gap-2" style="grid-template-columns: repeat({supportedFormats.length}, minmax(0, 1fr));">
              {#each supportedFormats as fmt}
                <button
                  type="button"
                  onclick={() => handleSelectFormat(fmt)}
                  class="pressable py-1.5 rounded-md border text-center text-xs font-semibold cursor-pointer transition {selectedExportFormat === fmt
                    ? 'border-[#1C1917] bg-[#F5F5F4] text-[#1C1917] shadow-2xs'
                    : 'border-[#E8E7E3] hover:border-[#1C1917] bg-white text-[#78716C] hover:text-[#1C1917]'}"
                >
                  {fmt.toUpperCase()}
                </button>
              {/each}
            </div>
            {#if formatHint}
              <div class="text-[11px] text-[#78716C] mt-1 font-sans">
                {formatHint}
              </div>
            {/if}
          </div>
        {:else if formatHint}
          <div class="px-3 py-2 bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg text-[11px] text-[#78716C] font-sans">
            {formatHint}
          </div>
        {/if}

        <!-- Output Destination File Path -->
        <div class="space-y-2 border border-[#E8E7E3] rounded-lg p-3.5 bg-white">
          <label
            for="output-path-input"
            class="block text-xs font-semibold text-[#1C1917]"
          >
            Путь для сохранения файла
          </label>
          <div class="flex items-center gap-2">
            <input
              id="output-path-input"
              type="text"
              bind:value={outputPath}
              placeholder="По умолчанию рядом с исходным файлом..."
              class="flex-1 px-3 py-1.5 border border-[#E8E7E3] rounded-md focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
            />
            <button
              type="button"
              onclick={handleSelectOutputPath}
              class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
            >
              Обзор...
            </button>
          </div>
        </div>

        <!-- Shuffle Rows Toggle (Only for tabular files: Excel, CSV, ODS) -->
        {#if appState.currentFileInfo.fileType === 'table'}
          <div class="border border-[#E8E7E3] rounded-lg p-3.5 bg-white hover-sheen transition">
            <label class="flex items-start space-x-2.5 cursor-pointer select-none group">
              <div class="relative flex items-center justify-center mt-0.5 shrink-0">
                <input
                  id="shuffle-rows-checkbox"
                  type="checkbox"
                  bind:checked={shuffleRows}
                  class="sr-only"
                />
                <div
                  class="w-4 h-4 rounded border transition-colors flex items-center justify-center pressable {shuffleRows ? 'bg-[#1C1917] border-[#1C1917] checked' : 'bg-white border-[#A8A29E] group-hover:border-[#1C1917]'}"
                >
                  <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                    <path class="checkmark-path" d="M5 13l4 4L19 7" />
                  </svg>
                </div>
              </div>
              <div>
                <span class="font-semibold text-[#1C1917] text-xs">
                  Перемешать строки
                </span>
                <div class="text-[11px] text-[#78716C] mt-1 leading-relaxed font-sans">
                  Случайный порядок записей для защиты от деанонимизации по позиции в реестре. Данные внутри строк расшифровываются корректно.
                </div>
              </div>
            </label>
          </div>
        {/if}

        <!-- Key Generation Option (Only for non-PDF files: tables and text) -->
        {#if appState.currentFileInfo.fileType !== 'pdf'}
          <div class="space-y-3 border border-[#E8E7E3] rounded-lg p-3.5 bg-white hover-sheen transition">
            <label class="flex items-start space-x-2.5 cursor-pointer select-none group">
              <div class="relative flex items-center justify-center mt-0.5 shrink-0">
                <input
                  id="create-key-checkbox"
                  type="checkbox"
                  bind:checked={createKey}
                  class="sr-only"
                />
                <div
                  class="w-4 h-4 rounded border transition-colors flex items-center justify-center pressable {createKey ? 'bg-[#1C1917] border-[#1C1917] checked' : 'bg-white border-[#A8A29E] group-hover:border-[#1C1917]'}"
                >
                  <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                    <path class="checkmark-path" d="M5 13l4 4L19 7" />
                  </svg>
                </div>
              </div>
              <div>
                <span class="font-semibold text-[#1C1917] text-xs">
                  Создать ключ деобезличивания
                </span>
                <div class="text-[11px] text-[#78716C] mt-0.5 leading-relaxed font-sans">
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
                    class="block text-xs font-semibold text-[#1C1917]"
                  >
                    Путь для сохранения ключа
                  </label>
                  <div class="flex items-center gap-2">
                    <input
                      id="key-path-input"
                      type="text"
                      bind:value={keyPath}
                      placeholder="Рядом с обработанным файлом..."
                      class="flex-1 px-3 py-1.5 border border-[#E8E7E3] rounded-md focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
                    />
                    <button
                      type="button"
                      onclick={handleSelectKeyPath}
                      class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
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
                      class="block text-xs font-semibold text-[#1C1917]"
                    >
                      Пароль ключа
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
                      class="w-full pr-8 px-3 py-1.5 border rounded-md {keyPassword.trim() && !passwordValidation.valid ? 'border-[#DC2626] focus:border-[#DC2626]' : 'border-[#E8E7E3] focus:border-[#1C1917]'} focus:outline-none text-xs font-mono bg-[#FAF9F5]"
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
                      <div class="h-1 rounded-full transition-colors {passwordValidation.score >= 1 ? (passwordValidation.score === 1 ? 'bg-[#DC2626]' : passwordValidation.score === 2 ? 'bg-[#D97706]' : passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                      <div class="h-1 rounded-full transition-colors {passwordValidation.score >= 2 ? (passwordValidation.score === 2 ? 'bg-[#D97706]' : passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                      <div class="h-1 rounded-full transition-colors {passwordValidation.score >= 3 ? (passwordValidation.score === 3 ? 'bg-[#2563EB]' : 'bg-[#16A34A]') : 'bg-[#E7E5E4]'}"></div>
                      <div class="h-1 rounded-full transition-colors {passwordValidation.score >= 4 ? 'bg-[#16A34A]' : 'bg-[#E7E5E4]'}"></div>
                    </div>

                    <!-- Requirements checklist -->
                    <div class="mt-2 bg-[#FAF9F5] p-2.5 border border-[#E8E7E3] rounded-md text-[11px] font-sans">
                      <div class="font-medium text-[#44403C] mb-1.5 text-[10.5px]">Требования к паролю:</div>
                      <div class="grid grid-cols-2 gap-x-2 gap-y-1">
                        <div class="flex items-center space-x-1.5 {passwordValidation.minLength ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.minLength}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>12+ символов</span>
                        </div>
                        <div class="flex items-center space-x-1.5 {passwordValidation.onlyLatin ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.onlyLatin}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>Только латиница</span>
                        </div>
                        <div class="flex items-center space-x-1.5 {passwordValidation.hasUppercase ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.hasUppercase}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>Заглавные буквы</span>
                        </div>
                        <div class="flex items-center space-x-1.5 {passwordValidation.hasLowercase ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.hasLowercase}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>Строчные буквы</span>
                        </div>
                        <div class="flex items-center space-x-1.5 {passwordValidation.hasDigit ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.hasDigit}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>Цифры 0-9</span>
                        </div>
                        <div class="flex items-center space-x-1.5 {passwordValidation.hasSpecial ? 'text-[#16A34A] font-medium' : 'text-[#78716C]'}">
                          <span class="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                            {#if passwordValidation.hasSpecial}
                              <svg class="w-3 h-3 text-[#16A34A]" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                              </svg>
                            {:else}
                              <span class="w-1 h-1 rounded-full bg-stone-400"></span>
                            {/if}
                          </span>
                          <span>Спецсимволы</span>
                        </div>
                      </div>

                      {#if keyPassword.trim() && !passwordValidation.onlyLatin}
                        <div class="mt-2 pt-1.5 border-t border-[#E8E7E3] text-[10.5px] text-[#DC2626] font-medium flex items-center space-x-1.5">
                          <svg class="w-3.5 h-3.5 text-[#DC2626] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                          </svg>
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
        {/if}

        <!-- Archive Generation Option (For all files) -->
        <div class="space-y-3 border border-[#E8E7E3] rounded-lg p-3.5 bg-white hover-sheen transition">
          <label class="flex items-start space-x-2.5 cursor-pointer select-none group">
            <div class="relative flex items-center justify-center mt-0.5 shrink-0">
              <input
                id="create-archive-checkbox"
                type="checkbox"
                bind:checked={createArchive}
                class="sr-only"
              />
              <div
                class="w-4 h-4 rounded border transition-colors flex items-center justify-center pressable {createArchive ? 'bg-[#1C1917] border-[#1C1917] checked' : 'bg-white border-[#A8A29E] group-hover:border-[#1C1917]'}"
              >
                <svg class="w-3 h-3 text-white" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                  <path class="checkmark-path" d="M5 13l4 4L19 7" />
                </svg>
              </div>
            </div>
            <div>
              <span class="font-semibold text-[#1C1917] text-xs">
                Упаковать в ZIP-архив с паролем
              </span>
              <div class="text-[11px] text-[#78716C] mt-0.5 leading-relaxed font-sans">
                Создает защищенный паролем архив с обработанным файлом для отправки по почте.
              </div>
            </div>
          </label>

          {#if createArchive}
            <div class="pt-3 border-t border-[#F0EFEB] space-y-3">
              <!-- Archive file path destination -->
              <div class="space-y-1.5">
                <label
                  for="archive-path-input"
                  class="block text-xs font-semibold text-[#1C1917]"
                >
                  Путь для сохранения архива
                </label>
                <div class="flex items-center gap-2">
                  <input
                    id="archive-path-input"
                    type="text"
                    bind:value={archivePath}
                    placeholder="По умолчанию рядом с файлом (*.zip)..."
                    class="flex-1 px-3 py-1.5 border border-[#E8E7E3] rounded-md focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] truncate"
                  />
                  <button
                    type="button"
                    onclick={handleSelectArchivePath}
                    class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
                  >
                    Обзор...
                  </button>
                </div>
              </div>

              <!-- Password for Archive -->
              <div class="space-y-1.5">
                <div class="flex items-center justify-between">
                  <label
                    for="archive-password-input"
                    class="block text-xs font-semibold text-[#1C1917]"
                  >
                    Пароль архива
                  </label>
                  {#if archivePassword.trim()}
                    <span class="text-[10px] font-mono {archivePasswordError ? 'text-[#DC2626]' : 'text-[#16A34A]'}">
                      {archivePasswordError ? 'Некорректный пароль' : 'Пароль подходит'}
                    </span>
                  {/if}
                </div>

                <div class="relative flex items-center gap-1.5">
                  <div class="relative flex-1 flex items-center">
                    <input
                      id="archive-password-input"
                      type={showArchivePassword ? 'text' : 'password'}
                      bind:value={archivePassword}
                      placeholder="Оставьте пустым для автогенерации (16 симв.)"
                      class="w-full pr-8 px-3 py-1.5 border rounded-md {archivePassword.trim() && archivePasswordError ? 'border-[#DC2626] focus:border-[#DC2626]' : 'border-[#E8E7E3] focus:border-[#1C1917]'} focus:outline-none text-xs font-mono bg-[#FAF9F5]"
                    />
                    {#if archivePassword}
                      <button
                        type="button"
                        onclick={() => showArchivePassword = !showArchivePassword}
                        class="absolute right-2 text-[#78716C] hover:text-[#1C1917] p-1 text-xs select-none cursor-pointer"
                        title={showArchivePassword ? 'Скрыть пароль' : 'Показать пароль'}
                        aria-label={showArchivePassword ? 'Скрыть пароль' : 'Показать пароль'}
                      >
                        {#if showArchivePassword}
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
                  <button
                    type="button"
                    onclick={handleGenerateArchivePassword}
                    class="px-2.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs flex items-center gap-1.5 pressable"
                    title="Сгенерировать случайный пароль"
                  >
                    <svg class="w-3.5 h-3.5 text-stone-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                    </svg>
                    <span>Сгенерировать</span>
                  </button>
                  {#if archivePassword}
                    <button
                      type="button"
                      onclick={handleCopyArchivePassword}
                      class="px-2.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md whitespace-nowrap cursor-pointer shadow-2xs pressable"
                      title="Скопировать пароль архива"
                    >
                      <svg class="w-3.5 h-3.5 text-stone-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                      </svg>
                    </button>
                  {/if}
                </div>

                {#if archivePasswordError}
                  <div class="text-[10.5px] text-[#DC2626] font-medium flex items-center space-x-1.5 mt-1">
                    <svg class="w-3.5 h-3.5 text-[#DC2626] shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                    <span>{archivePasswordError}</span>
                  </div>
                {:else}
                  <p class="text-[11px] text-[#78716C] leading-relaxed font-sans">
                    Минимум 4 символа (только латиница, цифры и знаки). Если оставить пустым — при сохранении сгенерируется надёжный пароль на 16 символов.
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
          class="h-8 px-4 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md pressable cursor-pointer flex items-center justify-center"
        >
          Отмена
        </button>
        <button
          type="button"
          onclick={handleStart}
          disabled={!isFormValid}
          class="h-8 px-5 text-xs font-semibold rounded-md transition flex items-center justify-center gap-1.5 shadow-sm pressable {isFormValid ? 'bg-[#1C1917] hover:bg-[#292524] text-white cursor-pointer' : 'bg-[#E7E5E4] text-[#A8A29E] cursor-not-allowed border border-[#D6D3D1]'}"
        >
          <svg class="w-3.5 h-3.5 text-stone-300 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4" />
          </svg>
          <span>{appState.processingMode === 'delete' ? 'Начать удаление' : 'Начать обезличивание'}</span>
        </button>
      </div>
    </div>
  </div>
{/if}
