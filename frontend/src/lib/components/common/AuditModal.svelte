<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { GetAuditRecords, ExportAuditLog, SaveAuditFileDialog } from '../../api/wails';
  import type { AuditRecord } from '../../types/domain';

  let records = $state<AuditRecord[]>([]);
  let isLoading = $state(false);
  let isExporting = $state(false);

  async function loadRecords() {
    isLoading = true;
    try {
      records = await GetAuditRecords(50);
    } catch (err) {
      console.error('Failed to load audit records:', err);
    } finally {
      isLoading = false;
    }
  }

  $effect(() => {
    if (appState.showAuditModal) {
      loadRecords();
    }
  });

  async function handleExport() {
    isExporting = true;
    try {
      const today = new Date().toISOString().slice(0, 10);
      const defaultName = `audit_${today}.jsonl`;
      const destPath = await SaveAuditFileDialog(defaultName);
      if (destPath) {
        await ExportAuditLog(destPath);
        appState.showToast('Журнал успешно экспортирован', 'success');
      }
    } catch (err) {
      console.error('Failed to export audit log:', err);
      appState.showToast('Ошибка при экспорте журнала', 'error');
    } finally {
      isExporting = false;
    }
  }

  function formatTime(isoStr: string): string {
    if (!isoStr) return '';
    try {
      const d = new Date(isoStr);
      if (isNaN(d.getTime())) return isoStr;
      return d.toLocaleString('ru-RU', {
        day: '2-digit',
        month: '2-digit',
        year: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
      });
    } catch {
      return isoStr;
    }
  }

  function getFileName(path: string): string {
    if (!path) return '—';
    return path.split('/').pop()?.split('\\').pop() || path;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && appState.showAuditModal) {
      appState.closeAuditModal();
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeydown);
    return () => {
      window.removeEventListener('keydown', handleKeydown);
    };
  });
</script>

{#if appState.showAuditModal}
  <div
    id="audit-modal"
    class="fixed inset-0 bg-black/40 backdrop-blur-[2px] flex items-center justify-center p-4 z-50 overscroll-contain"
    role="dialog"
    aria-modal="true"
    aria-labelledby="audit-modal-title"
    tabindex="-1"
    onclick={(e) => {
      if (e.target === e.currentTarget) appState.closeAuditModal();
    }}
    onkeydown={(e) => {
      if (e.key === 'Escape') appState.closeAuditModal();
    }}
  >
    <div
      class="bg-white border border-[#E8E7E3] shadow-2xl max-w-4xl w-full p-6 flex flex-col gap-4 max-h-[calc(100%-2rem)]"
    >
      <!-- Modal Header -->
      <div class="flex items-center justify-between border-b border-[#F0EFEB] pb-3">
        <div class="flex items-center space-x-2.5">
          <span class="text-sm">📜</span>
          <h2 id="audit-modal-title" class="text-sm font-bold text-[#1C1917]">
            Журнал операций
          </h2>
          <span class="text-[11px] font-mono text-[#78716C] bg-[#FAF9F5] px-2 py-0.5 border border-[#E8E7E3]">
            {records.length}
          </span>
        </div>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={loadRecords}
            disabled={isLoading}
            class="text-[#78716C] hover:text-[#1C1917] text-xs font-mono px-2 py-1 border border-[#E8E7E3] hover:border-[#1C1917] bg-[#FAF9F5] transition cursor-pointer disabled:opacity-50"
            title="Обновить"
          >
            ↻
          </button>
          <button
            type="button"
            onclick={() => appState.closeAuditModal()}
            class="text-stone-400 hover:text-stone-700 text-sm font-bold cursor-pointer ml-1"
            title="Закрыть"
          >
            ✕
          </button>
        </div>
      </div>

      <!-- Table Content Area -->
      <div class="flex-1 overflow-y-auto border border-[#E8E7E3] bg-white custom-scrollbar min-h-[220px]">
        {#if isLoading}
          <div class="flex items-center justify-center h-48 text-xs font-mono text-[#78716C]">
            Загрузка записей...
          </div>
        {:else if records.length === 0}
          <div class="flex flex-col items-center justify-center h-48 text-xs text-[#78716C] gap-1.5 font-mono">
            <span>Журнал пуст</span>
          </div>
        {:else}
          <table class="w-full text-left text-xs font-mono border-collapse">
            <thead class="sticky top-0 bg-[#FAF9F5] border-b border-[#E8E7E3] text-[11px] text-[#57534E]">
              <tr>
                <th class="py-2.5 px-3 font-semibold">Дата</th>
                <th class="py-2.5 px-3 font-semibold">Операция</th>
                <th class="py-2.5 px-3 font-semibold">Файл</th>
                <th class="py-2.5 px-3 font-semibold text-right">Записей</th>
                <th class="py-2.5 px-3 font-semibold text-center">Статус</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-[#F0EFEB]">
              {#each records as r}
                <tr class="hover:bg-[#FAF9F5] transition-colors">
                  <td class="py-2 px-3 whitespace-nowrap text-[#78716C] text-[11px]">
                    {formatTime(r.timestamp)}
                  </td>
                  <td class="py-2 px-3 whitespace-nowrap">
                    {#if r.operation === 'depersonalize'}
                      <span class="inline-block px-1.5 py-0.5 bg-[#FAF9F5] border border-[#E8E7E3] text-[#1C1917] text-[10px]">
                        Обезличивание
                      </span>
                    {:else if r.operation === 'restore'}
                      <span class="inline-block px-1.5 py-0.5 bg-sky-50 border border-sky-200 text-sky-800 text-[10px]">
                        Восстановление
                      </span>
                    {:else}
                      <span class="inline-block px-1.5 py-0.5 bg-[#FAF9F5] border border-[#E8E7E3] text-[#78716C] text-[10px]">
                        {r.operation}
                      </span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 max-w-[200px] truncate text-[#1C1917]" title={r.source_file}>
                    {getFileName(r.source_file)}
                  </td>
                  <td class="py-2 px-3 whitespace-nowrap text-right text-[#57534E]">
                    {(r.records_count || 0).toLocaleString('ru-RU')}
                  </td>
                  <td class="py-2 px-3 whitespace-nowrap text-center">
                    {#if r.status === 'success'}
                      <span class="inline-block px-1.5 py-0.5 bg-emerald-50 border border-emerald-200 text-emerald-700 text-[10px] font-semibold">
                        Успех
                      </span>
                    {:else if r.status === 'canceled'}
                      <span class="inline-block px-1.5 py-0.5 bg-stone-100 border border-stone-300 text-stone-600 text-[10px]">
                        Отмена
                      </span>
                    {:else}
                      <span class="inline-block px-1.5 py-0.5 bg-rose-50 border border-rose-200 text-rose-700 text-[10px] font-semibold" title={r.error || ''}>
                        Ошибка
                      </span>
                    {/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </div>

      <!-- Modal Footer -->
      <div class="flex items-center justify-between pt-2 border-t border-[#F0EFEB]">
        <button
          type="button"
          onclick={handleExport}
          disabled={isExporting || records.length === 0}
          class="px-3.5 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold transition cursor-pointer flex items-center gap-1.5 shadow-2xs disabled:opacity-50"
        >
          <span class="text-xs">💾</span>
          <span>Экспорт .jsonl</span>
        </button>

        <button
          type="button"
          onclick={() => appState.closeAuditModal()}
          class="px-4 py-2 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold transition cursor-pointer shadow-sm active:scale-[0.99]"
        >
          Закрыть
        </button>
      </div>
    </div>
  </div>
{/if}
