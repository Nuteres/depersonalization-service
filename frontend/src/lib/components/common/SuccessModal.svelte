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

  const bannerTitle = $derived(
    isRestore
      ? 'Исходные персональные данные успешно восстановлены'
      : 'Файл успешно сохранен без персональных данных'
  );

  const statsText = $derived.by(() => {
    const res = appState.successResult;
    if (!res) return '';
    const formattedDuration = formatDuration(res.durationMs || 0);
    if ('restoredCells' in res && typeof res.restoredCells === 'number') {
      return `Восстановлено ячеек с ПДн: ${res.restoredCells.toLocaleString('ru-RU')} ▪ Время: ${formattedDuration}`;
    }
    if ('rowsCount' in res && typeof res.rowsCount === 'number') {
      return `Обработано строк: ${res.rowsCount.toLocaleString('ru-RU')} ▪ Время: ${formattedDuration}`;
    }
    return `Время: ${formattedDuration}`;
  });

  async function handleRevealFolder() {
    const path = appState.successResult?.outputPath || appState.successResult?.archivePath;
    if (!path) return;
    try {
      await OpenContainingFolder(path);
    } catch (err) {
      console.error('Failed to open containing folder:', err);
      appState.showToast('Не удалось открыть папку', 'error');
    }
  }
</script>

{#if appState.showSuccessModal && appState.successResult}
  <div
    id="success-modal"
    class="fixed inset-0 bg-black/40 backdrop-blur-[2px] flex items-center justify-center p-4 z-50 overscroll-contain"
    role="dialog"
    aria-modal="true"
    aria-labelledby="success-modal-title"
    tabindex="-1"
    onkeydown={(e) => {
      if (e.key === 'Escape') appState.closeSuccessModal();
    }}
  >
    <div class="bg-white border border-[#E8E7E3] shadow-2xl max-w-lg w-full p-6 flex flex-col gap-4">
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <div class="flex items-center space-x-2.5">
          <span class="inline-flex items-center justify-center w-5 h-5 rounded-none bg-emerald-100 text-emerald-700 text-xs font-bold">
            ✔
          </span>
          <h2 id="success-modal-title" class="text-sm font-bold text-[#1C1917]">{modalTitle}</h2>
        </div>
        <button
          type="button"
          onclick={() => appState.closeSuccessModal()}
          class="text-stone-400 hover:text-stone-700 text-sm font-bold cursor-pointer"
          title="Закрыть"
        >
          ✕
        </button>
      </div>

      <!-- Success Banner -->
      <div class="p-3 bg-emerald-50/70 border border-emerald-200/80 text-xs text-emerald-950 flex items-center space-x-2 font-medium">
        <span>✔</span>
        <span>{bannerTitle}</span>
      </div>

      <!-- Compact Files & Location Card -->
      <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3.5 space-y-2">
        <div class="flex items-center gap-1.5 min-w-0 text-xs font-mono text-[#57534E]">
          <span class="text-stone-500 shrink-0">📁</span>
          <span class="text-[#78716C] shrink-0">Папка:</span>
          <span class="font-semibold text-[#1C1917] truncate select-all" title={savedDir}>
            {savedDir}
          </span>
        </div>

        <div class="space-y-1 pt-2 border-t border-[#E8E7E3]">
          {#if appState.successResult.outputPath}
            <div class="flex items-center gap-2 text-xs font-mono text-[#1C1917] py-0.5 px-1">
              <span class="shrink-0 text-stone-500">📄</span>
              <span class="truncate font-medium select-all" title={appState.successResult.outputPath}>
                {outputFileName}
              </span>
            </div>
          {/if}

          {#if !isRestore && appState.successResult.archivePath}
            <div class="flex items-center gap-2 text-xs font-mono text-[#1C1917] py-0.5 px-1">
              <span class="shrink-0 text-stone-500">📦</span>
              <span class="truncate font-medium select-all" title={appState.successResult.archivePath}>
                {archiveFileName}
              </span>
            </div>
          {/if}

          {#if !isRestore && appState.successResult.keyPath}
            <div class="flex items-center gap-2 text-xs font-mono text-[#1C1917] py-0.5 px-1">
              <span class="shrink-0 text-stone-500">🔑</span>
              <span class="truncate font-medium select-all" title={appState.successResult.keyPath}>
                {keyFileName}
              </span>
            </div>
          {/if}
        </div>
      </div>

      <!-- Password-Protected Archive Card -->
      {#if !isRestore && appState.successResult.archivePassword}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider flex items-center space-x-1.5">
              <span>📦</span>
              <span>Пароль архива:</span>
            </span>
            <button
              type="button"
              onclick={copyArchivePassword}
              class="text-[11px] text-[#1C1917] hover:underline font-mono font-semibold cursor-pointer shrink-0"
            >
              {copiedArchivePassword ? '✔ Скопировано' : '📋 Скопировать'}
            </button>
          </div>
          <div class="text-xs font-mono font-bold text-[#1C1917] select-all leading-relaxed bg-white p-2 border border-[#E8E7E3] tracking-wider">
            <code>{appState.successResult.archivePassword}</code>
          </div>
        </div>
      {/if}

      <!-- Recovery Key Password Card -->
      {#if !isRestore && appState.successResult && 'keyPasswordGenerated' in appState.successResult && appState.successResult.keyPasswordGenerated && appState.successResult.effectivePassword}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider flex items-center space-x-1">
              <span>🔑</span>
              <span>Пароль ключа:</span>
            </span>
            <button
              id="copy-key-password"
              type="button"
              onclick={copyPassword}
              class="text-[11px] text-[#1C1917] hover:underline font-mono font-semibold cursor-pointer shrink-0"
            >
              {copiedPassword ? '✔ Скопировано' : '📋 Скопировать'}
            </button>
          </div>
          <div
            class="text-xs font-mono font-bold text-[#1C1917] select-all leading-relaxed bg-white p-2 border border-[#E8E7E3] tracking-wider"
          >
            <code>{appState.successResult.effectivePassword}</code>
          </div>
          <div class="text-[11px] text-[#78716C] leading-normal font-sans">
            Сохраните пароль и файл-ключ в безопасном месте для восстановления данных.
          </div>
        </div>
      {/if}

      <!-- Stats Summary -->
      {#if statsText}
        <div class="text-xs text-[#78716C] font-mono pt-1">
          {statsText}
        </div>
      {/if}

      <!-- Footer Actions -->
      <div class="flex items-center justify-end space-x-2 pt-2 border-t border-[#F0EFEB]">
        {#if appState.successResult.outputPath || appState.successResult.archivePath}
          <button
            id="btn-reveal-folder"
            type="button"
            onclick={handleRevealFolder}
            class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold transition cursor-pointer flex items-center gap-1.5"
          >
            <span class="text-xs leading-none shrink-0 inline-flex items-center justify-center select-none">📁</span>
            <span>Показать в папке</span>
          </button>
        {/if}
        <button
          id="btn-close-success"
          type="button"
          onclick={() => appState.closeSuccessModal()}
          class="px-4 py-2 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold transition cursor-pointer"
        >
          Закрыть
        </button>
      </div>
    </div>
  </div>
{/if}
