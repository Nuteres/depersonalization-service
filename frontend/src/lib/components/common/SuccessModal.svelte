<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { OpenContainingFolder } from '../../api/wails';

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
    const duration = res.durationMs || 0;
    if ('restoredCells' in res && typeof res.restoredCells === 'number') {
      return `Восстановлено ячеек с ПДн: ${res.restoredCells.toLocaleString('ru-RU')} ▪ Время: ${duration} мс`;
    }
    if ('rowsCount' in res && typeof res.rowsCount === 'number') {
      return `Обработано строк: ${res.rowsCount.toLocaleString('ru-RU')} ▪ Время: ${duration} мс`;
    }
    return `Время: ${duration} мс`;
  });

  async function handleRevealFolder() {
    const path = appState.successResult?.outputPath;
    if (!path) return;
    try {
      await OpenContainingFolder(path);
    } catch (err) {
      console.error('Failed to open containing folder:', err);
      appState.showToast('Не удалось открыть папку', 'error');
    }
  }

  async function copyOutputPath() {
    const path = appState.successResult?.outputPath;
    if (!path) return;
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(path);
        appState.showToast('Путь к файлу скопирован в буфер!', 'success');
      }
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
    }
  }

  async function copyKeyPath() {
    const kp = appState.successResult?.keyPath;
    if (!kp) return;
    try {
      if (typeof navigator !== 'undefined' && navigator.clipboard) {
        await navigator.clipboard.writeText(kp);
        appState.showToast('Путь к файлу-ключу скопирован в буфер!', 'success');
      }
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
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
          <span class="inline-flex items-center justify-center w-5 h-5 rounded-full bg-emerald-100 text-emerald-700 text-xs font-bold">
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

      <!-- Saved Output File Card -->
      {#if appState.successResult.outputPath}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider">
              Сохраненный файл:
            </span>
            <button
              type="button"
              onclick={copyOutputPath}
              class="text-[11px] text-[#1C1917] hover:underline font-mono font-semibold cursor-pointer shrink-0"
            >
              📋 Копировать путь
            </button>
          </div>
          <div
            class="text-xs font-mono text-[#1C1917] break-all select-all leading-relaxed bg-white p-2 border border-[#E8E7E3]"
            title={appState.successResult.outputPath}
          >
            {appState.successResult.outputPath}
          </div>
        </div>
      {/if}

      <!-- Recovery Key File Card (ONLY for depersonalization when key was generated) -->
      {#if !isRestore && appState.successResult.keyPath}
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-3 space-y-2">
          <div class="flex items-center justify-between gap-2">
            <span class="text-[11px] font-mono font-bold text-[#1C1917] uppercase tracking-wider flex items-center space-x-1">
              <span>🔑</span>
              <span>Ключ восстановления (.depmap):</span>
            </span>
            <button
              type="button"
              onclick={copyKeyPath}
              class="text-[11px] text-[#1C1917] hover:underline font-mono font-semibold cursor-pointer shrink-0"
            >
              📋 Копировать путь
            </button>
          </div>
          <div
            class="text-xs font-mono text-[#1C1917] break-all select-all leading-relaxed bg-white p-2 border border-[#E8E7E3]"
            title={appState.successResult.keyPath}
          >
            {appState.successResult.keyPath}
          </div>
          <div class="text-[11px] text-[#78716C] leading-normal font-sans">
            Сохраните этот файл в безопасном месте. Без него обратная персонализация будет невозможна.
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
        {#if appState.successResult.outputPath}
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
