<script lang="ts">
  import { onMount } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import { GetSheetRows } from '../../api/wails';
  import type { EntityInfo } from '../../types/domain';

  const sheetName = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (info?.sheets && info.sheets.length > 0) {
      return info.sheets[tableState.activeSheetIndex]?.name || '';
    }
    return '';
  });

  const headers = $derived.by<string[]>(() => {
    const info = appState.currentFileInfo;
    if (!info) return [];
    if (info.sheets && info.sheets.length > 0) {
      const activeSheet = info.sheets[tableState.activeSheetIndex];
      return activeSheet?.headers || [];
    }
    return info.headers || [];
  });

  const entities = $derived.by<EntityInfo[]>(() => {
    const info = appState.currentFileInfo;
    if (!info) return [];
    if (info.sheets && info.sheets.length > 0) {
      const activeSheet = info.sheets[tableState.activeSheetIndex];
      return activeSheet?.entities || [];
    }
    return info.entities || [];
  });

  const disabledCols = $derived<number[]>(
    tableState.disabledColumnsBySheet[sheetName] || []
  );

  const totalRowCount = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (!info) return 0;
    if (info.sheets && info.sheets.length > 0) {
      const activeSheet = info.sheets[tableState.activeSheetIndex];
      return activeSheet?.rowCount || 0;
    }
    return info.rowCount || 0;
  });

  const mainRows = $derived(tableState.previewRows.slice(0, 10));

  async function loadMoreRows() {
    if (tableState.isLoadingRows) return;
    if (tableState.previewRows.length >= totalRowCount) return;
    if (!appState.currentFilePath) return;

    tableState.setIsLoadingRows(true);
    try {
      const offset = tableState.previewRows.length;
      const limit = 50;
      const newRows = await GetSheetRows(
        appState.currentFilePath,
        sheetName,
        offset,
        limit,
        appState.currentSalt,
        disabledCols
      );
      if (newRows && newRows.length > 0) {
        tableState.appendPreviewRows(newRows);
      }
    } catch (err) {
      console.error('Failed to load table rows batch:', err);
    } finally {
      tableState.setIsLoadingRows(false);
    }
  }

  function handleModalScroll(e: Event) {
    const target = e.currentTarget as HTMLElement;
    if (!target) return;
    if (target.scrollTop + target.clientHeight >= target.scrollHeight - 200) {
      loadMoreRows();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape' && tableState.isFullscreen) {
      tableState.setIsFullscreen(false);
    }
  }

  onMount(() => {
    window.addEventListener('keydown', handleKeydown);
    return () => {
      window.removeEventListener('keydown', handleKeydown);
    };
  });
</script>

{#if appState.currentFileInfo && appState.currentFileInfo.fileType === 'table'}
  <!-- Main Data Inspection Box -->
  <div id="table-inspection-box" class="bg-white border border-[#E8E7E3] shadow-sm flex flex-col">
    <!-- Table Header Toolbar -->
    <div
      class="p-4 border-b border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-[#FAF9F5]"
    >
      <div class="flex items-center space-x-2">
        <span class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono">
          Предпросмотр таблицы
        </span>
        <span id="preview-count-label" class="text-xs text-[#78716C] font-mono">
          {sheetName ? `[Лист: ${sheetName}] ` : ''}(первые {Math.min(10, tableState.previewRows.length)} строк)
        </span>
      </div>

      <div class="flex items-center space-x-3">
        <!-- Expand Button -->
        <button
          id="btn-expand-preview"
          type="button"
          onclick={() => tableState.setIsFullscreen(true)}
          class="px-3 py-1 bg-white hover:bg-[#F5F5F4] border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold flex items-center space-x-1.5 transition cursor-pointer shadow-xs active:scale-[0.99]"
          title="Раскрыть окно предпросмотра таблицы"
        >
          <span>⤢</span>
          <span>Раскрыть предпросмотр</span>
        </button>

        <!-- Toggle: Before vs After -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
          <button
            type="button"
            onclick={() => tableState.setIsBefore(false)}
            class="px-3 py-1 {!tableState.isBefore
              ? 'bg-[#1C1917] text-white font-semibold'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            Очищенные данные
          </button>
          <button
            type="button"
            onclick={() => tableState.setIsBefore(true)}
            class="px-3 py-1 {tableState.isBefore
              ? 'bg-[#1C1917] text-white font-semibold'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            Исходные с ПДн
          </button>
        </div>
      </div>
    </div>

    <!-- Table Scroll Container (10 rows preview) -->
    <div id="table-scroll-container" class="overflow-x-auto max-h-[460px] overflow-y-auto custom-scrollbar">
      <table id="preview-table" class="w-full text-left text-xs border-collapse">
        <thead class="sticky top-0 z-10 bg-white shadow-[0_1px_0_0_#E8E7E3]">
          <tr id="table-header-row" class="bg-white text-[#78716C] font-mono text-[11px] whitespace-nowrap">
            <th
              style="width: 54px; min-width: 54px; max-width: 54px;"
              class="py-3 px-3 font-normal text-center text-[#A8A29E] bg-[#FAF9F5] border-r border-[#F0EFEB] select-none"
            >
              #
            </th>
            {#each headers as h, idx}
              {@const col1Based = idx + 1}
              {@const ent = entities.find((e) => e.columnIndex === col1Based)}
              {@const isPii = !!ent}
              {@const isDisabled = disabledCols.includes(col1Based)}
              <th
                class="py-3 px-3 font-semibold text-[#1C1917] select-none border-r border-[#F0EFEB] min-w-[160px] max-w-[340px]"
              >
                <div class="flex items-center justify-between gap-1.5 w-full min-w-0 overflow-hidden pr-1">
                  <span
                    class="truncate min-w-0 flex-1 whitespace-nowrap {isDisabled ? 'line-through text-stone-400' : ''}"
                    title={h}
                  >
                    {h}
                  </span>

                  {#if isPii}
                    <button
                      type="button"
                      onclick={() => tableState.toggleColumn(sheetName, col1Based)}
                      title={isDisabled ? 'Включить деперсонализацию' : 'Исключить колонку'}
                      class="shrink-0 cursor-pointer"
                    >
                      {#if isDisabled}
                        <span
                          class="text-[10px] text-stone-500 bg-stone-100 border border-stone-200 px-1 py-0.5 rounded font-mono line-through"
                        >
                          ПДн
                        </span>
                      {:else}
                        <span
                          class="text-[10px] text-emerald-700 bg-emerald-50 border border-emerald-200 px-1 py-0.5 rounded font-mono font-medium"
                        >
                          ПДн: Замена
                        </span>
                      {/if}
                    </button>
                  {/if}
                </div>
              </th>
            {/each}
          </tr>
        </thead>

        <tbody id="table-rows" class="divide-y divide-[#F0EFEB] text-[13px]">
          {#if mainRows.length === 0}
            <tr>
              <td colspan={headers.length + 1} class="py-8 text-center text-xs text-[#78716C] font-mono">
                В таблице нет строк данных для отображения
              </td>
            </tr>
          {:else}
            {#each mainRows as row, i}
              <tr class="hover:bg-[#FAF9F5] transition-colors whitespace-nowrap">
                <td
                  class="py-3 px-3 font-mono text-center text-[#A8A29E] text-xs bg-[#FAF9F5] border-r border-[#F0EFEB] select-none"
                >
                  {String(row.index || i + 1).padStart(2, '0')}
                </td>
                {#each headers as _, cIdx}
                  {@const col1Based = cIdx + 1}
                  {@const ent = entities.find((e) => e.columnIndex === col1Based)}
                  {@const isPii = !!ent}
                  {@const isDisabled = disabledCols.includes(col1Based)}
                  {@const origVal = row.before?.[cIdx] ?? ''}
                  {@const anonVal = row.after?.[cIdx] ?? origVal}

                  {#if tableState.isBefore}
                    <!-- Before view -->
                    <td
                      class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate max-w-[340px] {isPii
                        ? isDisabled
                          ? 'text-[#78716C] bg-stone-50/50'
                          : 'font-medium text-rose-700 bg-rose-50/50'
                        : 'text-[#57534E]'}"
                      title={origVal}
                    >
                      {origVal}
                    </td>
                  {:else}
                    <!-- After view (Cleaned) -->
                    <td
                      class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate max-w-[340px] {isDisabled
                        ? 'text-[#78716C] bg-stone-50/70 italic'
                        : isPii
                          ? 'font-medium text-[#1C1917] bg-emerald-50/40'
                          : 'text-[#57534E]'}"
                      title={isDisabled ? origVal : anonVal}
                    >
                      {isDisabled ? origVal : anonVal}
                    </td>
                  {/if}
                {/each}
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>

    <!-- Table Footer / Trust note -->
    <div
      class="px-4 py-3 bg-[#FAF9F5] border-t border-[#E8E7E3] text-xs text-[#78716C] flex flex-col sm:flex-row sm:items-center justify-between gap-2 font-mono"
    >
      <div class="flex items-center space-x-2">
        <span>Ключ целостности: <strong class="text-[#1C1917]">{appState.currentSalt}</strong></span>
        <span>•</span>
        <span>
          {sheetName ? `[Лист: ${sheetName}] ` : ''}Показано {Math.min(10, tableState.previewRows.length)} из {totalRowCount.toLocaleString('ru-RU')} строк
        </span>
        <button
          type="button"
          onclick={() => tableState.setIsFullscreen(true)}
          class="ml-1 text-[#1C1917] hover:underline font-semibold cursor-pointer"
        >
          (раскрыть все строки ⤢)
        </button>
      </div>
      <span class="text-[#065F46] font-semibold">✔ Деперсонализация необратима без секретного ключа</span>
    </div>
  </div>

  <!-- EXPANDED FULLSCREEN MODAL -->
  {#if tableState.isFullscreen}
    <div
      id="expanded-preview-modal"
      class="fixed inset-0 bg-stone-900/60 backdrop-blur-xs flex items-center justify-center p-3 sm:p-5 z-50 overscroll-contain"
      onclick={(e) => {
        if (e.target === e.currentTarget) tableState.setIsFullscreen(false);
      }}
      onkeydown={(e) => {
        if (e.key === 'Escape') tableState.setIsFullscreen(false);
      }}
      role="dialog"
      aria-modal="true"
      tabindex="-1"
      aria-label="Полный предпросмотр таблицы"
    >
      <div
        class="bg-white border border-[#E8E7E3] shadow-2xl w-full h-[92vh] max-h-[92vh] flex flex-col max-w-[96vw] overflow-hidden overscroll-contain"
      >
        <!-- Modal Header -->
        <div
          class="px-5 py-3.5 border-b border-[#E8E7E3] flex items-center justify-between bg-[#FAF9F5] shrink-0 gap-3"
        >
          <div class="flex items-center space-x-3 truncate">
            <span class="font-bold text-sm tracking-tight text-[#1C1917] whitespace-nowrap">
              Полный предпросмотр таблицы
            </span>
            <span class="text-xs font-mono font-semibold text-[#1C1917] bg-[#E8E7E3] px-2 py-0.5 truncate">
              {appState.currentFileInfo.name}
            </span>
            <span class="text-xs text-[#78716C] font-mono whitespace-nowrap">
              {sheetName ? `[Лист: ${sheetName}] ` : ''}(показано {tableState.previewRows.length.toLocaleString('ru-RU')} из {totalRowCount.toLocaleString('ru-RU')} строк)
            </span>
          </div>

          <div class="flex items-center space-x-3 shrink-0">
            <!-- Toggle Before vs After inside Modal -->
            <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
              <button
                type="button"
                onclick={() => tableState.setIsBefore(false)}
                class="px-3 py-1 {!tableState.isBefore
                  ? 'bg-[#1C1917] text-white font-semibold'
                  : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
              >
                Очищенные данные
              </button>
              <button
                type="button"
                onclick={() => tableState.setIsBefore(true)}
                class="px-3 py-1 {tableState.isBefore
                  ? 'bg-[#1C1917] text-white font-semibold'
                  : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
              >
                Исходные с ПДн
              </button>
            </div>

            <!-- Close Modal Button -->
            <button
              type="button"
              onclick={() => tableState.setIsFullscreen(false)}
              class="px-3.5 py-1.5 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold shadow-xs transition flex items-center space-x-1 cursor-pointer"
            >
              <span>✕ Свернуть</span>
              <span class="text-[10px] text-[#A8A29E] font-mono ml-1">[Esc]</span>
            </button>
          </div>
        </div>

        <!-- Resizable Table in Modal with Dynamic Infinite Scroll -->
        <div
          id="modal-table-scroll-container"
          class="flex-1 min-h-0 w-full overflow-x-auto overflow-y-auto custom-scrollbar overscroll-contain"
          onscroll={handleModalScroll}
        >
          <table id="modal-preview-table" class="w-full text-left text-xs border-collapse">
            <thead class="sticky top-0 z-10 bg-white shadow-[0_1px_0_0_#E8E7E3]">
              <tr class="bg-white text-[#78716C] font-mono text-[11px] whitespace-nowrap">
                <th
                  style="width: 50px;"
                  class="py-3 px-3 font-normal text-center text-[#A8A29E] bg-[#FAF9F5] border-r border-[#F0EFEB] select-none"
                >
                  #
                </th>
                {#each headers as h, idx}
                  {@const col1Based = idx + 1}
                  {@const ent = entities.find((e) => e.columnIndex === col1Based)}
                  {@const isPii = !!ent}
                  {@const isDisabled = disabledCols.includes(col1Based)}
                  <th
                    class="py-3 px-3 font-semibold text-[#1C1917] select-none border-r border-[#F0EFEB] min-w-[160px] max-w-[340px]"
                  >
                    <div class="flex items-center justify-between gap-1.5 w-full min-w-0 overflow-hidden pr-1">
                      <span
                        class="truncate min-w-0 flex-1 whitespace-nowrap {isDisabled ? 'line-through text-stone-400' : ''}"
                        title={h}
                      >
                        {h}
                      </span>
                      {#if isPii}
                        <button
                          type="button"
                          onclick={() => tableState.toggleColumn(sheetName, col1Based)}
                          title={isDisabled ? 'Включить деперсонализацию' : 'Исключить колонку'}
                          class="shrink-0 cursor-pointer"
                        >
                          {#if isDisabled}
                            <span
                              class="text-[10px] text-stone-500 bg-stone-100 border border-stone-200 px-1 py-0.5 rounded font-mono line-through"
                            >
                              ПДн
                            </span>
                          {:else}
                            <span
                              class="text-[10px] text-emerald-700 bg-emerald-50 border border-emerald-200 px-1 py-0.5 rounded font-mono font-medium"
                            >
                              ПДн: Замена
                            </span>
                          {/if}
                        </button>
                      {/if}
                    </div>
                  </th>
                {/each}
              </tr>
            </thead>

            <tbody class="divide-y divide-[#F0EFEB] text-[13px]">
              {#each tableState.previewRows as row, i}
                <tr class="hover:bg-[#FAF9F5] transition-colors whitespace-nowrap">
                  <td
                    class="py-3 px-3 font-mono text-center text-[#A8A29E] text-xs bg-[#FAF9F5] border-r border-[#F0EFEB] select-none"
                  >
                    {String(row.index || i + 1).padStart(2, '0')}
                  </td>
                  {#each headers as _, cIdx}
                    {@const col1Based = cIdx + 1}
                    {@const ent = entities.find((e) => e.columnIndex === col1Based)}
                    {@const isPii = !!ent}
                    {@const isDisabled = disabledCols.includes(col1Based)}
                    {@const origVal = row.before?.[cIdx] ?? ''}
                    {@const anonVal = row.after?.[cIdx] ?? origVal}

                    {#if tableState.isBefore}
                      <td
                        class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate max-w-[340px] {isPii
                          ? isDisabled
                            ? 'text-[#78716C] bg-stone-50/50'
                            : 'font-medium text-rose-700 bg-rose-50/50'
                          : 'text-[#57534E]'}"
                        title={origVal}
                      >
                        {origVal}
                      </td>
                    {:else}
                      <td
                        class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate max-w-[340px] {isDisabled
                          ? 'text-[#78716C] bg-stone-50/70 italic'
                          : isPii
                            ? 'font-medium text-[#1C1917] bg-emerald-50/40'
                            : 'text-[#57534E]'}"
                        title={isDisabled ? origVal : anonVal}
                      >
                        {isDisabled ? origVal : anonVal}
                      </td>
                    {/if}
                  {/each}
                </tr>
              {/each}

              {#if tableState.isLoadingRows}
                <tr>
                  <td
                    colspan={headers.length + 1}
                    class="py-3 text-center text-xs text-[#78716C] font-mono bg-[#FAF9F5]"
                  >
                    <span class="inline-block animate-spin mr-2">⏳</span>
                    Подгрузка следующих строк...
                  </td>
                </tr>
              {/if}
            </tbody>
          </table>

          {#if tableState.previewRows.length < totalRowCount}
            <div class="p-4 text-center border-t border-[#F0EFEB]">
              <button
                type="button"
                onclick={loadMoreRows}
                disabled={tableState.isLoadingRows}
                class="px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs active:scale-[0.99] transition cursor-pointer disabled:opacity-50"
              >
                {tableState.isLoadingRows ? 'Загрузка...' : 'Загрузить еще 50 строк'}
              </button>
            </div>
          {/if}
        </div>

        <!-- Modal Footer -->
        <div
          class="px-5 py-2.5 bg-[#FAF9F5] border-t border-[#E8E7E3] text-xs text-[#78716C] flex items-center justify-between font-mono shrink-0"
        >
          <span id="modal-columns-summary">
            {sheetName ? `Лист: ${sheetName} • ` : ''}Колонок: {headers.length} • ПДн: {entities.length}
          </span>
          <span class="text-[#065F46] font-semibold">
            Листайте вниз для динамической подгрузки строк файла
          </span>
        </div>
      </div>
    </div>
  {/if}
{/if}
