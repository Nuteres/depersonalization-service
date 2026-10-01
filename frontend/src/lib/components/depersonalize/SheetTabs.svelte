<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';

  function switchSheet(idx: number) {
    if (tableState.activeSheetIndex === idx) return;
    tableState.setActiveSheet(idx);
    const sheet = appState.currentFileInfo?.sheets?.[idx];
    if (sheet) {
      tableState.setPreviewRows(sheet.previewRows || []);
      tableState.setPage(1);
    }
  }
</script>

{#if appState.currentFileInfo?.sheets && appState.currentFileInfo.sheets.length > 1}
  <div
    id="excel-sheets-bar"
    class="flex flex-col sm:flex-row sm:items-center justify-between bg-[#F5F5F4] border border-[#E8E7E3] px-3.5 py-2.5 gap-2"
  >
    <div class="flex items-center space-x-2 overflow-x-auto custom-scrollbar py-0.5 max-w-full">
      <span class="text-xs font-mono font-bold text-[#78716C] uppercase tracking-wider mr-1.5 shrink-0 flex items-center space-x-1 select-none">
        <span>📑</span>
        <span>Листы книги:</span>
      </span>

      <div id="excel-sheets-list" class="flex items-center space-x-1.5 shrink-0">
        {#each appState.currentFileInfo.sheets as sheet, idx}
          {@const isActive = idx === tableState.activeSheetIndex}
          {@const disabledList = tableState.disabledColumnsBySheet[sheet.name] || []}
          {@const disabledCount = disabledList.length}
          {@const rowCountStr = (sheet.rowCount || 0).toLocaleString('ru-RU')}
          <button
            type="button"
            onclick={() => switchSheet(idx)}
            class={isActive
              ? "px-3 py-1.5 bg-white border border-[#1C1917] text-[#1C1917] font-semibold text-xs shadow-xs transition flex items-center space-x-1.5 cursor-pointer"
              : "px-3 py-1.5 bg-transparent border border-transparent hover:border-[#D6D3D1] hover:bg-white text-[#78716C] hover:text-[#1C1917] text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer"}
          >
            {#if isActive}
              <span class="w-2 h-2 rounded-full bg-emerald-600"></span>
            {/if}
            <span class="truncate max-w-[180px]" title={sheet.name}>{sheet.name}</span>
            <span
              class="text-[10px] font-mono {isActive ? 'text-[#78716C] bg-[#F0EFEB]' : 'text-[#A8A29E]'} px-1.5 py-0.2 rounded"
            >
              ({rowCountStr})
            </span>
            {#if disabledCount > 0}
              <span
                class="text-[10px] font-mono text-stone-500 bg-stone-200/80 px-1 py-0.2 rounded"
                title="Отключено колонок: {disabledCount}"
              >
                -{disabledCount}
              </span>
            {/if}
          </button>
        {/each}
      </div>
    </div>

    <div
      id="excel-sheets-summary"
      class="hidden sm:flex items-center text-[11px] font-mono text-[#78716C] shrink-0 select-none"
    >
      <span>Все листы книги будут деперсонализированы в один файл</span>
    </div>
  </div>
{/if}
