<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { CancelCurrentOperation } from '../../api/wails';

  let isCanceling = $state(false);

  const modalTitle = $derived.by(() => {
    if (appState.progress.title) return appState.progress.title;
    return appState.currentAppMode === 'restore'
      ? 'Восстановление исходных данных'
      : 'Деперсонализация ПДн';
  });

  const statusText = $derived.by(() => {
    return appState.currentAppMode === 'restore'
      ? 'Расшифровка ключа и восстановление данных...'
      : 'Потоковая обработка строк...';
  });

  const countText = $derived.by(() => {
    if (appState.progress.total > 0) {
      return `${appState.progress.processed.toLocaleString('ru-RU')} / ${appState.progress.total.toLocaleString('ru-RU')} строк`;
    }
    if (appState.progress.processed > 0) {
      return `${appState.progress.processed.toLocaleString('ru-RU')} строк`;
    }
    return 'Инициализация...';
  });

  async function handleCancel() {
    if (isCanceling) return;
    isCanceling = true;
    try {
      await CancelCurrentOperation();
      appState.finishProgress();
      appState.showToast('Операция отменена пользователем', 'warning');
    } catch (err) {
      console.error('Failed to cancel operation:', err);
      appState.showToast('Ошибка при отмене операции', 'error');
    } finally {
      isCanceling = false;
    }
  }
</script>

{#if appState.showProgressModal}
  <div
    id="progress-modal"
    class="fixed inset-0 bg-black/40 backdrop-blur-[2px] flex items-center justify-center p-4 z-50"
    role="dialog"
    aria-modal="true"
    aria-labelledby="progress-modal-title"
  >
    <div class="bg-white border border-[#E8E7E3] shadow-2xl max-w-md w-full p-6 flex flex-col gap-4">
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <div class="flex items-center space-x-2">
          <span id="progress-indicator" class="w-2.5 h-2.5 rounded-none bg-emerald-600 animate-pulse"></span>
          <h2 id="progress-modal-title" class="text-sm font-bold text-[#1C1917]">{modalTitle}</h2>
        </div>
        <span id="progress-pct-badge" class="font-mono text-xs font-bold text-[#1C1917] bg-[#F0EFEB] px-2 py-0.5">
          {appState.progress.percent}%
        </span>
      </div>

      <div class="space-y-2">
        <div class="w-full bg-[#F0EFEB] h-3 rounded-none overflow-hidden">
          <div
            id="progress-bar-fill"
            class="bg-[#1C1917] h-full transition-all duration-150"
            style="width: {appState.progress.percent}%"
          ></div>
        </div>
        <div class="flex items-center justify-between text-xs font-mono text-[#78716C]">
          <span id="progress-status-text">{statusText}</span>
          <span id="progress-count-text">{countText}</span>
        </div>
      </div>

      <div class="flex items-center justify-end space-x-2 pt-2 border-t border-[#F0EFEB]">
        <button
          id="btn-cancel-progress"
          type="button"
          onclick={handleCancel}
          disabled={isCanceling}
          class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold transition cursor-pointer disabled:opacity-50"
        >
          {isCanceling ? 'Отмена...' : 'Отмена'}
        </button>
      </div>
    </div>
  </div>
{/if}
