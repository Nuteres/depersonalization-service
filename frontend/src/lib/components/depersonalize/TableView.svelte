<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import { zoomState } from '../../stores/zoomState.svelte';
  import { GetSheetRows } from '../../api/wails';
  import type { EntityInfo, ProcessingMode } from '../../types/domain';

  let loadAttemptCompleted = $state(false);

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

  let textMeasureCtx: CanvasRenderingContext2D | null = null;

  function measureTextPx(text: string, font?: string): number {
    if (!text) return 0;
    try {
      if (!textMeasureCtx && typeof document !== 'undefined') {
        const c = document.createElement('canvas');
        textMeasureCtx = c.getContext('2d');
      }
      if (textMeasureCtx) {
        textMeasureCtx.font =
          font || '600 12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
        return Math.ceil(textMeasureCtx.measureText(String(text)).width);
      }
    } catch {
      // Fallback
    }
    return Math.ceil(String(text).length * 8.8);
  }

  function calculateOptimalColumnWidth(h: string, col1Based: number): number {
    const isPiiCol = entities.some((e) => e.columnIndex === col1Based);

    // 1. Measure header text width (bold 12px font)
    const headerTextW = measureTextPx(
      h,
      '600 12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif'
    );

    // 2. Badge "[ ПДн ]" takes ~48px
    const badgeW = isPiiCol ? 48 : 0;

    // 3. Spacing: th px-3 (24px) + gap-1.5 (6px) + col-resizer handle (10px) + safety buffer (16px) = 56px
    let maxNeeded = headerTextW + badgeW + 56;

    // 4. Sample preview rows (up to 15 rows)
    const sampleRows = tableState.previewRows.slice(0, 15);
    const cIdx = col1Based - 1;
    for (const row of sampleRows) {
      const vBefore = row.before?.[cIdx] != null ? String(row.before[cIdx]) : '';
      const vAfter = row.after?.[cIdx] != null ? String(row.after[cIdx]) : '';

      if (vBefore) {
        const font = isPiiCol
          ? '400 12px "JetBrains Mono", monospace'
          : '400 13px -apple-system, BlinkMacSystemFont, sans-serif';
        const wBefore = measureTextPx(vBefore, font) + 36;
        if (wBefore > maxNeeded) maxNeeded = wBefore;
      }
      if (vAfter) {
        const font = isPiiCol
          ? '500 12px "JetBrains Mono", monospace'
          : '400 13px -apple-system, BlinkMacSystemFont, sans-serif';
        const wAfter = measureTextPx(vAfter, font) + 36;
        if (wAfter > maxNeeded) maxNeeded = wAfter;
      }
    }

    // Minimum 130px, maximum default 520px
    return Math.min(520, Math.max(130, Math.ceil(maxNeeded)));
  }

  // Auto-calculate column widths whenever headers are available and uninitialized
  $effect(() => {
    if (headers.length > 0) {
      if (tableState.columnWidths.length !== headers.length + 1) {
        const widths = [50]; // Column 0: '#'
        headers.forEach((h, idx) => {
          widths.push(calculateOptimalColumnWidth(h, idx + 1));
        });
        tableState.setColumnWidths(widths);
      }
    }
  });

  const totalTableWidth = $derived.by(() => {
    if (tableState.columnWidths.length === 0) return 0;
    return tableState.columnWidths.reduce((a, b) => a + b, 0);
  });

  function initResize(e: MouseEvent, colIndex: number) {
    e.preventDefault();
    e.stopPropagation();

    const startX = e.pageX;
    const startWidth = tableState.columnWidths[colIndex] || 180;
    const resizer = e.currentTarget as HTMLElement;
    resizer?.classList.add('resizing');
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    function onMouseMove(moveEvent: MouseEvent) {
      const diff = moveEvent.pageX - startX;
      const newWidth = Math.max(30, Math.round(startWidth + diff));
      tableState.setColumnWidth(colIndex, newWidth);
    }

    function onMouseUp() {
      resizer?.classList.remove('resizing');
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  // Automatically pre-load rows to fill the viewport when expanded preview opens
  $effect(() => {
    if (tableState.isFullscreen) {
      untrack(() => {
        if (
          !tableState.isLoadingRows &&
          tableState.previewRows.length < 50 &&
          tableState.previewRows.length < totalRowCount &&
          !loadAttemptCompleted
        ) {
          loadMoreRows();
        }
      });
    } else {
      loadAttemptCompleted = false;
    }
  });

  let lastObservedStrategy = $state(appState.strategy);

  $effect(() => {
    const currentStrategy = appState.strategy;
    if (currentStrategy !== lastObservedStrategy) {
      lastObservedStrategy = currentStrategy;
      if (appState.currentFilePath) {
        untrack(() => {
          refreshPreviewRows(currentStrategy);
        });
      }
    }
  });

  async function refreshPreviewRows(strat: string) {
    if (!appState.currentFilePath) return;
    try {
      const count = Math.max(10, Math.min(tableState.previewRows.length, 50));
      const updatedRows = await GetSheetRows(
        appState.currentFilePath,
        sheetName,
        0,
        count,
        appState.currentSalt,
        disabledCols,
        strat
      );
      if (updatedRows && updatedRows.length > 0) {
        tableState.setPreviewRows(updatedRows);
      }
    } catch (err) {
      console.error('Failed to refresh preview rows on strategy change:', err);
    }
  }

  async function loadMoreRows() {
    if (tableState.isLoadingRows) return;
    if (tableState.previewRows.length >= totalRowCount) {
      loadAttemptCompleted = true;
      return;
    }
    if (!appState.currentFilePath) return;

    tableState.setIsLoadingRows(true);
    let noMore = false;
    try {
      const offset = tableState.previewRows.length;
      const limit = 50;
      const newRows = await GetSheetRows(
        appState.currentFilePath,
        sheetName,
        offset,
        limit,
        appState.currentSalt,
        disabledCols,
        appState.strategy
      );
      if (newRows && newRows.length > 0) {
        tableState.appendPreviewRows(newRows);
        if (newRows.length < limit || tableState.previewRows.length >= totalRowCount) {
          noMore = true;
        }

        // Auto-fill until container has scrollable overflow
        if (!noMore && tableState.isFullscreen && tableState.previewRows.length < totalRowCount) {
          setTimeout(() => {
            const container = document.getElementById('modal-table-scroll-container');
            if (container && container.scrollHeight <= container.clientHeight + 100) {
              loadMoreRows();
            }
          }, 60);
        }
      } else {
        noMore = true;
      }
    } catch (err) {
      console.error('Failed to load table rows batch:', err);
      noMore = true;
    } finally {
      tableState.setIsLoadingRows(false);
      if (noMore) {
        loadAttemptCompleted = true;
      }
    }
  }

  function handleModalScroll(e: Event) {
    const target = e.currentTarget as HTMLElement;
    if (!target || tableState.isLoadingRows) return;
    if (target.scrollTop + target.clientHeight >= target.scrollHeight - 350) {
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
          {sheetName ? `[Лист: ${sheetName}] ` : ''}▪ Показано {Math.min(10, tableState.previewRows.length)} из {totalRowCount.toLocaleString('ru-RU')} строк
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
      <table
        id="preview-table"
        class="resizable-table w-full text-left text-xs border-collapse"
        style={totalTableWidth > 0 ? `width: ${totalTableWidth}px; min-width: 100%;` : 'width: 100%;'}
      >
        <thead class="sticky top-0 z-10 bg-white shadow-[0_1px_0_0_#E8E7E3]">
          <tr id="table-header-row" class="bg-white text-[#78716C] font-mono text-[11px] whitespace-nowrap">
            <th
              style="width: {tableState.columnWidths[0] || 50}px;"
              class="py-3 px-3 font-normal text-center text-[#A8A29E] bg-[#FAF9F5] border-r border-[#F0EFEB] select-none"
            >
              #
            </th>
            {#each headers as h, idx}
              {@const col1Based = idx + 1}
              {@const ent = entities.find((e) => e.columnIndex === col1Based)}
              {@const isPii = !!ent}
              {@const isDisabled = disabledCols.includes(col1Based)}
              {@const colW = tableState.columnWidths[col1Based] || 180}
              <th
                style="width: {colW}px;"
                class="py-3 px-3 font-semibold text-[#1C1917] select-none border-r border-[#F0EFEB]"
              >
                <div class="flex items-center justify-between gap-1.5 w-full min-w-0 overflow-hidden pr-2">
                  <span
                    class="truncate min-w-0 flex-1 whitespace-nowrap {isDisabled ? 'line-through text-stone-400' : ''} {!h || !h.trim() ? 'text-stone-400 italic font-normal' : ''}"
                    title={h && h.trim() ? h : `Колонка ${col1Based}`}
                  >
                    {h && h.trim() ? h : `Колонка ${col1Based}`}
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
                          class="text-[10px] text-emerald-700 bg-emerald-50 border-emerald-200 border px-1.5 py-0.5 rounded font-mono font-medium"
                        >
                          ПДн
                        </span>
                      {/if}
                    </button>
                  {/if}
                </div>
                <div
                  class="col-resizer"
                  onmousedown={(e) => initResize(e, col1Based)}
                  role="button"
                  tabindex="-1"
                  aria-label="Изменить ширину колонки"
                ></div>
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
                  {@const displayVal = isDisabled
                    ? origVal
                    : isPii && appState.processingMode === 'delete'
                      ? '[УДАЛЕНО]'
                      : anonVal}

                  {#if tableState.isBefore}
                    <!-- Before view -->
                    <td
                      class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate {isPii
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
                      class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate {isDisabled
                        ? 'text-[#78716C] bg-stone-50/70 italic'
                        : isPii
                          ? appState.processingMode === 'delete'
                            ? 'font-medium text-rose-700 bg-rose-50/40'
                            : 'font-medium text-[#1C1917] bg-emerald-50/40'
                          : 'text-[#57534E]'}"
                      title={displayVal}
                    >
                      {displayVal}
                    </td>
                  {/if}
                {/each}
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  </div>

  <!-- EXPANDED FULLSCREEN MODAL -->
  {#if tableState.isFullscreen}
    <div
      id="expanded-preview-modal"
      class="fixed inset-0 bg-stone-900/60 backdrop-blur-xs flex items-center justify-center p-2 sm:p-4 z-50 overflow-hidden overscroll-contain"
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
        class="bg-white border border-[#E8E7E3] shadow-2xl w-full h-full max-h-full max-w-full flex flex-col overflow-hidden overscroll-contain"
      >
        <!-- Modal Header -->
        <div
          class="px-4 py-2.5 sm:px-5 sm:py-3 border-b border-[#E8E7E3] flex flex-wrap items-center justify-between bg-[#FAF9F5] shrink-0 gap-2 sm:gap-3"
        >
          <div class="flex flex-wrap items-center gap-2 sm:gap-3 min-w-0 flex-1">
            <span class="font-bold text-xs sm:text-sm tracking-tight text-[#1C1917] whitespace-nowrap">
              Полный предпросмотр таблицы
            </span>
            <span
              class="text-xs font-mono font-semibold text-[#1C1917] bg-[#E8E7E3] px-2 py-0.5 max-w-[160px] sm:max-w-xs md:max-w-md truncate"
              title={appState.currentFileInfo.name}
            >
              {appState.currentFileInfo.name}
            </span>
            <span class="text-xs text-[#78716C] font-mono whitespace-nowrap">
              {sheetName ? `[Лист: ${sheetName}] ` : ''}(показано {tableState.previewRows.length.toLocaleString('ru-RU')} из {totalRowCount.toLocaleString('ru-RU')} строк)
            </span>
          </div>

          <div class="flex items-center space-x-2 sm:space-x-3 shrink-0 ml-auto">
            <!-- Modal Zoom Controls -->
            <div class="flex items-center bg-white text-xs font-medium border border-[#E8E7E3]" id="modal-zoom-controls">
              <button
                type="button"
                onclick={() => zoomState.zoomOut()}
                disabled={zoomState.currentZoom <= zoomState.minZoom}
                class="px-2 py-1 hover:bg-[#FAF9F5] text-[#78716C] hover:text-[#1C1917] transition cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
                title="Уменьшить масштаб (Ctrl + -)"
              >
                –
              </button>
              <button
                type="button"
                onclick={() => zoomState.resetZoom()}
                class="px-2 py-1 font-mono text-[11px] font-semibold text-[#1C1917] hover:bg-[#FAF9F5] transition cursor-pointer min-w-[44px] text-center"
                title="Сбросить масштаб к 100% (Ctrl + 0)"
              >
                {Math.round(zoomState.currentZoom * 100)}%
              </button>
              <button
                type="button"
                onclick={() => zoomState.zoomIn()}
                disabled={zoomState.currentZoom >= zoomState.maxZoom}
                class="px-2 py-1 hover:bg-[#FAF9F5] text-[#78716C] hover:text-[#1C1917] transition cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
                title="Увеличить масштаб (Ctrl + +)"
              >
                +
              </button>
            </div>

            <!-- Toggle Before vs After inside Modal -->
            <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
              <button
                type="button"
                onclick={() => tableState.setIsBefore(false)}
                class="px-2.5 sm:px-3 py-1 {!tableState.isBefore
                  ? 'bg-[#1C1917] text-white font-semibold'
                  : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer text-xs"
              >
                Очищенные данные
              </button>
              <button
                type="button"
                onclick={() => tableState.setIsBefore(true)}
                class="px-2.5 sm:px-3 py-1 {tableState.isBefore
                  ? 'bg-[#1C1917] text-white font-semibold'
                  : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer text-xs"
              >
                Исходные с ПДн
              </button>
            </div>

            <!-- Close Modal Button -->
            <button
              type="button"
              onclick={() => tableState.setIsFullscreen(false)}
              class="px-3 py-1.5 bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold shadow-xs transition flex items-center space-x-1 cursor-pointer shrink-0"
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
          <table
            id="modal-preview-table"
            class="resizable-table w-full text-left text-xs border-collapse"
            style={totalTableWidth > 0 ? `width: ${totalTableWidth}px; min-width: 100%;` : 'width: 100%;'}
          >
            <thead class="sticky top-0 z-20 bg-white shadow-[0_1px_0_0_#E8E7E3]">
              <tr class="bg-white text-[#78716C] font-mono text-[11px] whitespace-nowrap">
                <th
                  style="width: {tableState.columnWidths[0] || 50}px;"
                  class="py-3 px-3 font-normal text-center text-[#A8A29E] bg-[#FAF9F5] border-r border-[#F0EFEB] select-none sticky top-0"
                >
                  #
                </th>
                {#each headers as h, idx}
                  {@const col1Based = idx + 1}
                  {@const ent = entities.find((e) => e.columnIndex === col1Based)}
                  {@const isPii = !!ent}
                  {@const isDisabled = disabledCols.includes(col1Based)}
                  {@const colW = tableState.columnWidths[col1Based] || 180}
                  <th
                    style="width: {colW}px;"
                    class="py-3 px-3 font-semibold text-[#1C1917] select-none border-r border-[#F0EFEB] bg-white sticky top-0"
                  >
                    <div class="flex items-center justify-between gap-1.5 w-full min-w-0 overflow-hidden pr-2">
                      <span
                        class="truncate min-w-0 flex-1 whitespace-nowrap {isDisabled ? 'line-through text-stone-400' : ''} {!h || !h.trim() ? 'text-stone-400 italic font-normal' : ''}"
                        title={h && h.trim() ? h : `Колонка ${col1Based}`}
                      >
                        {h && h.trim() ? h : `Колонка ${col1Based}`}
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
                              class="text-[10px] text-emerald-700 bg-emerald-50 border-emerald-200 border px-1.5 py-0.5 rounded font-mono font-medium"
                            >
                              ПДн
                            </span>
                          {/if}
                        </button>
                      {/if}
                    </div>
                    <div
                      class="col-resizer"
                      onmousedown={(e) => initResize(e, col1Based)}
                      role="button"
                      tabindex="-1"
                      aria-label="Изменить ширину колонки"
                    ></div>
                  </th>
                {/each}
              </tr>
            </thead>

            <tbody class="divide-y divide-[#F0EFEB] text-[13px]">
              {#if tableState.previewRows.length === 0}
                <tr>
                  <td colspan={headers.length + 1} class="py-8 text-center text-xs text-[#78716C] font-mono">
                    В таблице нет строк данных для отображения
                  </td>
                </tr>
              {:else}
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
                      {@const displayVal = isDisabled
                        ? origVal
                        : isPii && appState.processingMode === 'delete'
                          ? '[УДАЛЕНО]'
                          : anonVal}

                      {#if tableState.isBefore}
                        <td
                          class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate {isPii
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
                          class="py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate {isDisabled
                            ? 'text-[#78716C] bg-stone-50/70 italic'
                            : isPii
                              ? appState.processingMode === 'delete'
                                ? 'font-medium text-rose-700 bg-rose-50/40'
                                : 'font-medium text-[#1C1917] bg-emerald-50/40'
                              : 'text-[#57534E]'}"
                          title={displayVal}
                        >
                          {displayVal}
                        </td>
                      {/if}
                    {/each}
                  </tr>
                {/each}
              {/if}

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

          {#if tableState.previewRows.length < totalRowCount && !loadAttemptCompleted}
            <div class="p-3 text-center border-t border-[#F0EFEB] bg-[#FAF9F5]">
              <button
                type="button"
                onclick={() => {
                  loadAttemptCompleted = false;
                  loadMoreRows();
                }}
                disabled={tableState.isLoadingRows}
                class="px-4 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs active:scale-[0.99] transition cursor-pointer disabled:opacity-50"
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
            {sheetName ? `Лист: ${sheetName} ▪ ` : ''}Колонок: {headers.length} ▪ ПДн: {entities.length}
          </span>
          <span class="text-[#065F46] font-semibold">
            Листайте вниз для динамической подгрузки строк файла
          </span>
        </div>
      </div>
    </div>
  {/if}
{/if}
