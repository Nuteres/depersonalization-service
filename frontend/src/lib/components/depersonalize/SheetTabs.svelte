<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';

  function switchSheet(idx: number) {
    if (tableState.activeSheetIndex === idx) return;
    tableState.setActiveSheet(idx);
    tableState.setColumnWidths([]);
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
    class="flex items-center gap-1.5 overflow-x-auto custom-scrollbar py-1"
  >
    <div id="excel-sheets-list" class="flex items-center gap-1.5 shrink-0">
      {#each appState.currentFileInfo.sheets as sheet, idx}
        {@const isActive = idx === tableState.activeSheetIndex}
        {@const disabledList = tableState.disabledColumnsBySheet[sheet.name] || []}
        {@const disabledCount = disabledList.length}
        <button
          type="button"
          onclick={() => switchSheet(idx)}
          class={isActive
            ? "h-7 px-3.5 bg-white border border-[#1C1917] text-[#1C1917] font-semibold text-xs rounded-md shadow-2xs pressable flex items-center gap-2 cursor-pointer"
            : "h-7 px-3.5 bg-transparent border border-transparent hover:border-[#D6D3D1] hover:bg-white text-[#78716C] hover:text-[#1C1917] text-xs font-medium rounded-md pressable flex items-center gap-2 cursor-pointer"}
        >
          {#if isActive}
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-500 shrink-0"></span>
          {/if}
          <span class="truncate max-w-[200px]" title={sheet.name}>{sheet.name}</span>
          {#if disabledCount > 0}
            <span
              class="text-[10px] font-mono text-stone-500 bg-stone-200 px-1.5 py-0.5 rounded leading-none shrink-0"
              title="Отключено колонок: {disabledCount}"
            >
              -{disabledCount}
            </span>
          {/if}
        </button>
      {/each}
    </div>
  </div>
{/if}
