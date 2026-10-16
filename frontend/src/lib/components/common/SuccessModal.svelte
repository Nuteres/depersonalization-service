<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { OpenContainingFolder } from '../../api/wails';
  import { copyToClipboard } from '../../services/clipboard';

  let copiedPassword = $state(false);
  let copiedArchivePassword = $state(false);

  function getDirectory(filePath: string): string {
    if (!filePath) return '';
    const lastSlash = Math.max(filePath.lastIndexOf('/'), filePath.lastIndexOf('\\'));
    return lastSlash >= 0 ? filePath.slice(0, lastSlash) : filePath;
  }

  function getFileName(filePath: string): string {
    if (!filePath) return '';
    const lastSlash = Math.max(filePath.lastIndexOf('/'), filePath.lastIndexOf('\\'));
    return lastSlash >= 0 ? filePath.slice(lastSlash + 1) : filePath;
  }

  function formatDuration(ms: number): string {
    if (ms < 1000) {
      return `${ms} мс`;
    }
    const totalSeconds = Math.round(ms / 1000);
    if (totalSeconds < 60) {
      return `${totalSeconds} сек`;
    }
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    if (seconds === 0) {
      return `${minutes} мин`;
    }
    return `${minutes} мин ${seconds} сек`;
  }

  const savedDir = $derived.by(() => {
    return getDirectory(appState.successResult?.outputPath || appState.successResult?.archivePath || '');
  });

  const outputFileName = $derived.by(() => {
    return getFileName(appState.successResult?.outputPath || '');
  });

  const archiveFileName = $derived.by(() => {
    return getFileName(appState.successResult?.archivePath || '');
  });

  const keyFileName = $derived.by(() => {
    return getFileName(appState.successResult?.keyPath || '');
  });

  async function copyArchivePassword() {
    const pwd = appState.successResult?.archivePassword;
    if (!pwd) return;
    const ok = await copyToClipboard(pwd);
    if (ok) {
      copiedArchivePassword = true;
      setTimeout(() => { copiedArchivePassword = false; }, 2000);
      appState.showToast('Пароль архива скопирован в буфер обмена!', 'success');
    } else {
      appState.showToast('Не удалось скопировать пароль архива', 'error');
    }
  }

  async function copyPassword() {
    const pwd =
      appState.successResult && 'effectivePassword' in appState.successResult
        ? appState.successResult.effectivePassword
        : undefined;
    if (!pwd) return;
    const ok = await copyToClipboard(pwd);
    if (ok) {
      copiedPassword = true;
      setTimeout(() => { copiedPassword = false; }, 2000);
      appState.showToast('Пароль ключа скопирован в буфер обмена!', 'success');
    } else {
      appState.showToast('Не удалось скопировать пароль ключа', 'error');
    }
  }

  const isRestore = $derived.by(() => {
    return (
      appState.currentAppMode === 'restore' ||
      (appState.successResult !== null && 'restoredCells' in appState.successResult)
    );
  });

  const modalTitle = $derived(
    isRestore ? 'Восстановление успешно завершено' : 'Деперсонализация успешно завершена'
  );


  const formattedDuration = $derived.by(() => {
    return formatDuration(appState.successResult?.durationMs || 0);
  });

  const statsCount = $derived.by(() => {
    const res = appState.successResult;
    if (!res) return null;
    if ('restoredCells' in res && typeof res.restoredCells === 'number') {
      return {
        label: 'Восстановлено ячеек с ПДн',
        value: res.restoredCells.toLocaleString('ru-RU')
      };
    }
    if ('rowsCount' in res && typeof res.rowsCount === 'number') {
      return {
        label: 'Обработано строк',
        value: res.rowsCount.toLocaleString('ru-RU')
      };
    }
    return null;
  });

  async function handleRevealFile(targetPath?: string) {
    if (!targetPath) return;
    try {
      await OpenContainingFolder(targetPath);
    } catch (err) {
      console.error('Failed to open containing folder:', err);
      appState.showToast('Не удалось открыть папку', 'error');
    }
  }

  async function handleRevealFolder() {
    const path = appState.successResult?.outputPath || appState.successResult?.archivePath || appState.successResult?.keyPath;
    await handleRevealFile(path);
  }
</script>

{#if appState.showSuccessModal && appState.successResult}
  <div
    id="success-modal"
    class="fixed inset-0 bg-black/40 backdrop-blur-xs flex items-center justify-center p-4 z-50 overscroll-contain transition-opacity"
    role="dialog"
    aria-modal="true"
    aria-labelledby="success-modal-title"
    tabindex="-1"
    onkeydown={(e) => {
      if (e.key === 'Escape') appState.closeSuccessModal();
    }}
  >
    <div class="bg-white border border-[#E8E7E3] rounded-lg shadow-2xl max-w-lg w-full p-6 flex flex-col gap-4 animate-surface-enter">
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <div class="flex items-center space-x-2.5">
          <div class="w-6 h-6 rounded-full bg-emerald-50 border border-emerald-200/80 flex items-center justify-center text-emerald-600">
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="20 6 9 17 4 12" />
            </svg>
          </div>
          <h2 id="success-modal-title" class="text-sm font-bold text-[#1C1917]">{modalTitle}</h2>
        </div>
        <button
          type="button"
          onclick={() => appState.closeSuccessModal()}
          class="text-stone-400 hover:text-stone-700 p-1 rounded-md hover:bg-stone-100 transition pressable cursor-pointer"
          title="Закрыть"
        >
          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Saved Files Card -->
      <div class="border border-[#E8E7E3] bg-[#FAF9F5] rounded-lg p-3 space-y-2">
        <div class="text-[11px] font-semibold text-[#78716C] uppercase tracking-wider">
          Сохраненные файлы
        </div>

        <div class="space-y-1.5">
          {#if appState.successResult.outputPath}
            <div class="flex items-center justify-between gap-2 bg-white border border-[#E8E7E3] rounded-md p-2">
              <div class="min-w-0 flex items-center gap-2">
                <svg class="w-3.5 h-3.5 text-emerald-700 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                <div class="min-w-0">
                  <div class="text-xs font-mono font-medium text-[#1C1917] truncate select-all" title={appState.successResult.outputPath}>
                    {outputFileName}
                  </div>
                  <div class="text-[10px] text-[#78716C] font-mono truncate select-all" title={appState.successResult.outputPath}>
                    {getDirectory(appState.successResult.outputPath)}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onclick={() => handleRevealFile(appState.successResult?.outputPath)}
                class="px-2 py-1 bg-[#FAF9F5] hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-[11px] font-medium rounded shadow-2xs pressable shrink-0 cursor-pointer"
                title="Показать файл в папке"
              >
                Показать
              </button>
            </div>
          {/if}

          {#if !isRestore && appState.successResult.archivePath}
            <div class="flex items-center justify-between gap-2 bg-white border border-[#E8E7E3] rounded-md p-2">
              <div class="min-w-0 flex items-center gap-2">
                <svg class="w-3.5 h-3.5 text-amber-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
                </svg>
                <div class="min-w-0">
                  <div class="text-xs font-mono font-medium text-[#1C1917] truncate select-all" title={appState.successResult.archivePath}>
                    {archiveFileName}
                  </div>
                  <div class="text-[10px] text-[#78716C] font-mono truncate select-all" title={appState.successResult.archivePath}>
                    {getDirectory(appState.successResult.archivePath)}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onclick={() => handleRevealFile(appState.successResult?.archivePath)}
                class="px-2 py-1 bg-[#FAF9F5] hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-[11px] font-medium rounded shadow-2xs pressable shrink-0 cursor-pointer"
                title="Показать архив в папке"
              >
                Показать
              </button>
            </div>
          {/if}

          {#if !isRestore && appState.successResult.keyPath}
            <div class="flex items-center justify-between gap-2 bg-white border border-[#E8E7E3] rounded-md p-2">
              <div class="min-w-0 flex items-center gap-2">
                <svg class="w-3.5 h-3.5 text-stone-500 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                </svg>
                <div class="min-w-0">
                  <div class="text-xs font-mono font-medium text-[#1C1917] truncate select-all" title={appState.successResult.keyPath}>
                    {keyFileName}
                  </div>
                  <div class="text-[10px] text-[#78716C] font-mono truncate select-all" title={appState.successResult.keyPath}>
                    {getDirectory(appState.successResult.keyPath)}
                  </div>
                </div>
              </div>
              <button
                type="button"
                onclick={() => handleRevealFile(appState.successResult?.keyPath)}
                class="px-2 py-1 bg-[#FAF9F5] hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-[11px] font-medium rounded shadow-2xs pressable shrink-0 cursor-pointer"
                title="Показать ключ в папке"
              >
                Показать
              </button>
            </div>
          {/if}
        </div>
      </div>

      <!-- Password-Protected Archive Card -->
      {#if !isRestore && appState.successResult.archivePassword}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3.5 rounded-lg space-y-2">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold text-[#1C1917]">
              Пароль архива
            </span>
            <button
              type="button"
              onclick={copyArchivePassword}
              class="px-2.5 py-1 bg-white hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-medium rounded-md shadow-2xs pressable flex items-center gap-1.5 cursor-pointer"
            >
              {#if copiedArchivePassword}
                <svg class="w-3 h-3 text-emerald-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                </svg>
                <span class="text-emerald-700 font-semibold">Скопировано</span>
              {:else}
                <svg class="w-3 h-3 text-stone-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
                <span>Скопировать</span>
              {/if}
            </button>
          </div>
          <div class="text-xs font-mono font-semibold text-[#1C1917] select-all bg-white px-3 py-2 border border-[#E8E7E3] rounded-md tracking-wider">
            <code>{appState.successResult.archivePassword}</code>
          </div>
        </div>
      {/if}

      <!-- Recovery Key Password Card -->
      {#if !isRestore && appState.successResult && 'keyPasswordGenerated' in appState.successResult && appState.successResult.keyPasswordGenerated && appState.successResult.effectivePassword}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3.5 rounded-lg space-y-2">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs font-semibold text-[#1C1917]">
              Пароль ключа
            </span>
            <button
              id="copy-key-password"
              type="button"
              onclick={copyPassword}
              class="px-2.5 py-1 bg-white hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-medium rounded-md shadow-2xs pressable flex items-center gap-1.5 cursor-pointer"
            >
              {#if copiedPassword}
                <svg class="w-3 h-3 text-emerald-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
                </svg>
                <span class="text-emerald-700 font-semibold">Скопировано</span>
              {:else}
                <svg class="w-3 h-3 text-stone-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
                </svg>
                <span>Скопировать</span>
              {/if}
            </button>
          </div>
          <div class="text-xs font-mono font-semibold text-[#1C1917] select-all bg-white px-3 py-2 border border-[#E8E7E3] rounded-md tracking-wider">
            <code>{appState.successResult.effectivePassword}</code>
          </div>
          <div class="text-[11px] text-[#78716C] leading-normal font-sans">
            Сохраните пароль и файл-ключ в безопасном месте для восстановления данных.
          </div>
        </div>
      {/if}

      <!-- Stats Summary -->
      {#if statsCount || formattedDuration}
        <div class="flex items-center justify-between text-xs text-[#78716C] pt-0.5 font-sans">
          <div>
            {#if statsCount}
              <span>{statsCount.label}: </span>
              <span class="font-semibold text-[#1C1917] font-mono">{statsCount.value}</span>
            {/if}
          </div>
          <div class="shrink-0">
            <span>Время: </span>
            <span class="font-semibold text-[#1C1917] font-mono">{formattedDuration}</span>
          </div>
        </div>
      {/if}

      <!-- Footer Buttons -->
      <div class="flex items-center justify-end space-x-2 pt-3 border-t border-[#F0EFEB]">
        {#if appState.successResult.outputPath || appState.successResult.archivePath}
          <button
            id="btn-reveal-folder"
            type="button"
            onclick={handleRevealFolder}
            class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md transition cursor-pointer flex items-center gap-1.5 shadow-2xs pressable"
          >
            <svg class="w-3.5 h-3.5 text-stone-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
            </svg>
            <span>Показать в папке</span>
          </button>
        {/if}
        <button
          id="btn-close-success"
          type="button"
          onclick={() => appState.closeSuccessModal()}
          class="px-4 py-1.5 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold rounded-md transition cursor-pointer shadow-sm pressable"
        >
          Закрыть
        </button>
      </div>
    </div>
  </div>
{/if}
