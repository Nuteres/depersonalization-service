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
      return `Восстановлено ячеек с ПДн: ${res.restoredCells.toLocaleString('ru-RU')} • Время: ${duration} мс`;
    }
    if ('rowsCount' in res && typeof res.rowsCount === 'number') {
      return `Обработано строк: ${res.rowsCount.toLocaleString('ru-RU')} • Время: ${duration} мс`;
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
    class="fixed inset-0 bg-black/40 backdrop-blur-[2px] flex items-center justify-center p-4 z-50"
    role="dialog"
    aria-modal="true"
    aria-labelledby="success-modal-title"
  >
    <div class="bg-white border border-[#E8E7E3] shadow-2xl max-w-lg w-full p-6 flex flex-col gap-4">
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <div class="flex items-center space-x-2">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-600"></span>
          <h2 id="success-modal-title" class="text-sm font-bold text-[#1C1917]">{modalTitle}</h2>
        </div>
      </div>

      <!-- Success Details Box -->
      <div class="p-3.5 bg-[#ECFDF5] border border-[#A7F3D0] text-xs text-[#065F46] flex flex-col gap-2 font-mono">
        <div class="font-bold flex items-center space-x-1.5 text-sm">
          <span>✔</span>
          <span>{bannerTitle}</span>
        </div>

        {#if appState.successResult.outputPath}
          <div>
            <div class="text-[10px] text-[#065F46] font-sans font-semibold">Сохраненный файл:</div>
            <div
              class="text-[11px] text-[#047857] truncate mt-0.5 select-all"
              title={appState.successResult.outputPath}
            >
              {appState.successResult.outputPath}
            </div>
          </div>
        {/if}

        {#if appState.successResult.keyPath}
          <div class="text-[11px] text-[#065F46] bg-emerald-100/70 p-2.5 border border-emerald-300 rounded-none break-all space-y-1">
            <div class="flex items-center justify-between gap-2">
              <span class="font-bold flex items-center space-x-1">
                <span>🔑</span><span>Ключ для обратного восстановления:</span>
              </span>
              <button
                type="button"
                onclick={copyKeyPath}
                class="text-[10px] text-emerald-800 underline font-sans hover:text-emerald-950 cursor-pointer"
              >
                📋 Копировать путь
              </button>
            </div>
            <div
              class="text-[10px] font-mono opacity-90 truncate select-all"
              title={appState.successResult.keyPath}
            >
              {appState.successResult.keyPath}
            </div>
            <div class="text-[10px] text-emerald-700 font-sans">
              Сохраните этот файл в безопасном месте. Без него обратная персонализация невозможна.
            </div>
          </div>
        {/if}

        {#if statsText}
          <div class="text-[11px] text-[#065F46] mt-0.5 font-mono">
            {statsText}
          </div>
        {/if}
      </div>

      <div class="flex items-center justify-end space-x-2 pt-2 border-t border-[#F0EFEB]">
        {#if appState.successResult.outputPath}
          <button
            id="btn-reveal-folder"
            type="button"
            onclick={handleRevealFolder}
            class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold transition cursor-pointer flex items-center space-x-1"
          >
            <span>📁</span>
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
