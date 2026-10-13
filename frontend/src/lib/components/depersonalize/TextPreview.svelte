<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import type {
    DocxTextSnippet,
    TextDocumentBlock,
    TextParagraphPreview,
    TextParagraphSegment
  } from '../../types/domain';

  // View state
  let isBefore = $state(false); // false = Anonymized, true = Original
  let currentActiveChangeIdx = $state<number>(1);
  let hoveredEntity = $state<{
    type: string;
    original: string;
    anonymized: string;
    isDisabled: boolean;
    x: number;
    y: number;
  } | null>(null);

  const fileInfo = $derived(appState.currentFileInfo);
  const documentBlocks = $derived<TextDocumentBlock[]>(
    fileInfo?.documentBlocks || []
  );
  const paragraphPreviews = $derived<TextParagraphPreview[]>(
    fileInfo?.paragraphPreviews || []
  );
  const snippets = $derived<DocxTextSnippet[]>(
    fileInfo?.textSnippets || []
  );

  const disabledCols = $derived<number[]>(
    tableState.disabledColumnsBySheet[''] || []
  );

  const typeLabels: Record<string, string> = {
    FIO: 'ФИО',
    SNILS: 'СНИЛС',
    PASSPORT: 'Паспорт РФ',
    PHONE: 'Телефон',
    INN: 'ИНН',
    BANK_CARD: 'Банк / Счёт',
    CARD: 'Банковская карта',
    DATE: 'Дата',
    EMAIL: 'Email',
    ADDRESS: 'Адрес',
    GRADE: 'Грейд',
    EMPLOYEE_ID: 'Табельный номер'
  };

  const typeActionLabels: Record<string, string> = {
    FIO: 'Обезличивание ФИО',
    SNILS: 'Обезличивание СНИЛС',
    PASSPORT: 'Обезличивание паспортов',
    PHONE: 'Обезличивание номеров',
    INN: 'Обезличивание ИНН',
    BANK_CARD: 'Обезличивание карт',
    CARD: 'Обезличивание карт',
    DATE: 'Обезличивание дат',
    EMAIL: 'Обезличивание email',
    ADDRESS: 'Обезличивание адресов',
    GRADE: 'Обезличивание грейдов',
    EMPLOYEE_ID: 'Обезличивание табельных номеров'
  };

  function getEntityTypeName(type: string): string {
    return typeLabels[type] || type || 'ПДн';
  }

  function getEntityAction(type: string): string {
    return typeActionLabels[type] || 'Деперсонализация';
  }

  function getEntityInfoForType(type: string) {
    return fileInfo?.entities?.find((e) => e.type === type);
  }

  function isEntityTypeDisabled(type: string): boolean {
    const ent = getEntityInfoForType(type);
    if (!ent) return false;
    return disabledCols.includes(ent.columnIndex);
  }

  function toggleEntityType(type: string) {
    const ent = getEntityInfoForType(type);
    if (ent) {
      tableState.toggleColumn('', ent.columnIndex);
      if (hoveredEntity && hoveredEntity.type === type) {
        hoveredEntity = {
          ...hoveredEntity,
          isDisabled: !hoveredEntity.isDisabled
        };
      }
    }
  }

  // Count total entity segments across all document blocks or paragraphs
  interface FlatEntityMark {
    globalIndex: number;
    key: string;
    type: string;
    original: string;
    anonymized: string;
    elementId: string;
  }

  // Pre-calculate entity marks for linear navigation across paragraphs and table cells
  const allEntityMarks = $derived.by<FlatEntityMark[]>(() => {
    const list: FlatEntityMark[] = [];
    let gIdx = 0;

    if (documentBlocks.length > 0) {
      documentBlocks.forEach((block, bIdx) => {
        if ((block.type === 'paragraph' || block.type === 'heading') && block.segments) {
          block.segments.forEach((seg, sIdx) => {
            if (seg.isEntity) {
              gIdx++;
              list.push({
                globalIndex: gIdx,
                key: `${bIdx}_${sIdx}`,
                type: seg.type || 'FIO',
                original: seg.original || seg.text,
                anonymized: seg.anonymized || seg.text,
                elementId: `doc-entity-mark-${gIdx}`
              });
            }
          });
        } else if (block.type === 'table' && block.table) {
          block.table.rows.forEach((row, rIdx) => {
            row.cells.forEach((cell, cIdx) => {
              cell.segments.forEach((seg, sIdx) => {
                if (seg.isEntity) {
                  gIdx++;
                  list.push({
                    globalIndex: gIdx,
                    key: `${bIdx}_${rIdx}_${cIdx}_${sIdx}`,
                    type: seg.type || 'FIO',
                    original: seg.original || seg.text,
                    anonymized: seg.anonymized || seg.text,
                    elementId: `doc-entity-mark-${gIdx}`
                  });
                }
              });
            });
          });
        }
      });
    } else {
      paragraphPreviews.forEach((para, pIdx) => {
        para.segments.forEach((seg, sIdx) => {
          if (seg.isEntity) {
            gIdx++;
            list.push({
              globalIndex: gIdx,
              key: `${pIdx}_${sIdx}`,
              type: seg.type || 'FIO',
              original: seg.original || seg.text,
              anonymized: seg.anonymized || seg.text,
              elementId: `doc-entity-mark-${gIdx}`
            });
          }
        });
      });
    }

    return list;
  });

  // Calculate global index lookup map for fast rendering: key -> globalIndex
  const entityIndexMap = $derived.by<Record<string, number>>(() => {
    const map: Record<string, number> = {};
    for (const m of allEntityMarks) {
      map[m.key] = m.globalIndex;
    }
    return map;
  });

  function scrollToEntity(globalIdx: number) {
    if (globalIdx < 1 || globalIdx > allEntityMarks.length) return;
    currentActiveChangeIdx = globalIdx;
    const elem = document.getElementById(`doc-entity-mark-${globalIdx}`);
    if (elem) {
      elem.scrollIntoView({ behavior: 'smooth', block: 'center' });
      elem.classList.add('ring-4', 'ring-indigo-500/70', 'ring-offset-2');
      setTimeout(() => {
        elem.classList.remove('ring-4', 'ring-indigo-500/70', 'ring-offset-2');
      }, 1800);
    }
  }

  function nextChange() {
    if (allEntityMarks.length === 0) return;
    const currentPos = allEntityMarks.findIndex(
      (m) => m.globalIndex === currentActiveChangeIdx
    );
    if (currentPos === -1 || currentPos >= allEntityMarks.length - 1) {
      const first = allEntityMarks[0];
      if (first) {
        scrollToEntity(first.globalIndex);
      }
    } else {
      const next = allEntityMarks[currentPos + 1];
      if (next) {
        scrollToEntity(next.globalIndex);
      }
    }
  }

  function prevChange() {
    if (allEntityMarks.length === 0) return;
    const currentPos = allEntityMarks.findIndex(
      (m) => m.globalIndex === currentActiveChangeIdx
    );
    if (currentPos <= 0) {
      const last = allEntityMarks[allEntityMarks.length - 1];
      if (last) {
        scrollToEntity(last.globalIndex);
      }
    } else {
      const prev = allEntityMarks[currentPos - 1];
      if (prev) {
        scrollToEntity(prev.globalIndex);
      }
    }
  }

  function handleMarkHover(
    event: MouseEvent,
    seg: TextParagraphSegment
  ) {
    const target = event.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();
    const type = seg.type || 'FIO';
    hoveredEntity = {
      type,
      original: seg.original || seg.text,
      anonymized: seg.anonymized || seg.text,
      isDisabled: isEntityTypeDisabled(type),
      x: rect.left + rect.width / 2,
      y: rect.top - 8
    };
  }

  function handleMarkLeave() {
    hoveredEntity = null;
  }
</script>

{#if fileInfo && (fileInfo.fileType === 'text' || fileInfo.format === 'docx' || fileInfo.format === 'odt')}
  <div
    id="text-inspection-box"
    class="bg-white border border-[#E8E7E3] shadow-sm flex flex-col select-text"
  >
    <!-- Compact Header Toolbar (Single Row) -->
    <div
      class="px-5 py-3 border-b border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-[#FAF9F5]"
    >
      <!-- Title & Document Stats -->
      <div class="flex items-center space-x-2">
        <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
        <span
          class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono"
        >
          Предпросмотр документа ({fileInfo.format.toUpperCase()})
        </span>
        <span class="text-xs text-[#78716C] font-mono">
          ▪ {allEntityMarks.length > 0 ? allEntityMarks.length : snippets.length} фрагментов ПДн
        </span>
      </div>

      <!-- Controls Right: Change Navigator & Before/After Toggle -->
      <div class="flex items-center space-x-3">
        {#if allEntityMarks.length > 0}
          <div class="inline-flex items-center border border-[#E8E7E3] bg-white text-xs font-mono shadow-2xs">
            <button
              type="button"
              onclick={prevChange}
              title="Предыдущий фрагмент ПДн"
              class="px-2.5 py-1 text-[#57534E] hover:text-[#1C1917] hover:bg-[#FAF9F5] transition cursor-pointer border-r border-[#E8E7E3] select-none"
            >
              ◀
            </button>
            <span class="px-2.5 py-1 text-[11px] text-[#1C1917] font-medium select-none whitespace-nowrap">
              {currentActiveChangeIdx} / {allEntityMarks.length}
            </span>
            <button
              type="button"
              onclick={nextChange}
              title="Следующий фрагмент ПДн"
              class="px-2.5 py-1 text-[#57534E] hover:text-[#1C1917] hover:bg-[#FAF9F5] transition cursor-pointer border-l border-[#E8E7E3] select-none"
            >
              ▶
            </button>
          </div>
        {/if}

        <!-- Toggle: Cleaned vs Original -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium shadow-2xs">
          <button
            type="button"
            onclick={() => (isBefore = false)}
            class="flex items-center space-x-1.5 px-3 py-1 {!isBefore
              ? 'bg-emerald-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-full {!isBefore ? 'bg-emerald-300' : 'bg-emerald-500'}"></span>
            <span>Обезличенный вид</span>
          </button>
          <button
            type="button"
            onclick={() => (isBefore = true)}
            class="flex items-center space-x-1.5 px-3 py-1 {isBefore
              ? 'bg-amber-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-full {isBefore ? 'bg-amber-300' : 'bg-amber-500'}"></span>
            <span>Исходный вид</span>
          </button>
        </div>
      </div>
    </div>

      <!-- Word-like Document Canvas (Block-based scrolling container, no height collapse) -->
      <div
        id="word-sheet-viewport"
        class="p-4 sm:p-8 md:p-10 bg-[#EDEBE6] overflow-y-auto max-h-[720px] custom-scrollbar relative"
      >
        <!-- Word Page Sheet (Full Height Block, grows with content) -->
        <div
          id="word-sheet-paper"
          class="mx-auto w-full max-w-3xl bg-white shadow-lg border border-[#D5D3CC] p-6 sm:p-10 md:p-12 text-[#1C1917] font-sans text-[14px] sm:text-[15px] leading-[1.8] min-h-[300px] h-auto"
        >
          {#if documentBlocks.length > 0}
            <!-- Structured Document Blocks: Paragraphs, Headings and Tables -->
            <div class="space-y-3">
              {#each documentBlocks as block, bIdx}
                {#if block.type === 'heading' && block.segments}
                  <h3 class="mt-5 mb-2 font-bold text-[16px] sm:text-[17px] text-[#1C1917] tracking-tight break-words">
                    {#each block.segments as seg, sIdx}{#if !seg.isEntity}<span>{seg.text}</span>{:else}{@const gIdx = entityIndexMap[`${bIdx}_${sIdx}`] || 0}{@const isTypeDisabled = isEntityTypeDisabled(seg.type || '')}{@const markClass = isTypeDisabled
                        ? 'bg-stone-100 text-stone-600 border-dashed border-stone-400 hover:bg-stone-200 hover:border-stone-600'
                        : !isBefore
                        ? appState.processingMode === 'delete'
                          ? 'bg-rose-100/95 text-rose-950 border-rose-400 hover:bg-rose-200'
                          : 'bg-emerald-100/95 text-emerald-950 border-emerald-400 hover:bg-emerald-200'
                        : 'bg-amber-100/95 text-amber-950 border-amber-400 hover:bg-amber-200'}<button
                        type="button"
                        id="doc-entity-mark-{gIdx}"
                        onmouseenter={(e) => handleMarkHover(e, seg)}
                        onmouseleave={handleMarkLeave}
                        onclick={() => {
                          currentActiveChangeIdx = gIdx;
                          toggleEntityType(seg.type || '');
                        }}
                        class="inline px-1.5 py-0.5 rounded-[3px] border font-medium text-[13.5px] font-mono shadow-xs transition-all duration-150 cursor-pointer text-left align-baseline {markClass} hover:scale-[1.02]"
                      >{#if isTypeDisabled}<span class="line-through decoration-stone-400">{seg.original || seg.text}</span>{:else if !isBefore}{appState.processingMode === 'delete' ? '[УДАЛЕНО]' : (seg.anonymized || seg.text)}{:else}{seg.original || seg.text}{/if}</button>{/if}{/each}
                  </h3>
                {:else if block.type === 'paragraph' && block.segments}
                  <p class="mb-3 whitespace-pre-wrap leading-relaxed text-justify break-words">
                    {#each block.segments as seg, sIdx}{#if !seg.isEntity}<span>{seg.text}</span>{:else}{@const gIdx = entityIndexMap[`${bIdx}_${sIdx}`] || 0}{@const isTypeDisabled = isEntityTypeDisabled(seg.type || '')}{@const markClass = isTypeDisabled
                        ? 'bg-stone-100 text-stone-600 border-dashed border-stone-400 hover:bg-stone-200 hover:border-stone-600'
                        : !isBefore
                        ? appState.processingMode === 'delete'
                          ? 'bg-rose-100/95 text-rose-950 border-rose-400 hover:bg-rose-200'
                          : 'bg-emerald-100/95 text-emerald-950 border-emerald-400 hover:bg-emerald-200'
                        : 'bg-amber-100/95 text-amber-950 border-amber-400 hover:bg-amber-200'}<button
                        type="button"
                        id="doc-entity-mark-{gIdx}"
                        onmouseenter={(e) => handleMarkHover(e, seg)}
                        onmouseleave={handleMarkLeave}
                        onclick={() => {
                          currentActiveChangeIdx = gIdx;
                          toggleEntityType(seg.type || '');
                        }}
                        class="inline px-1.5 py-0.5 rounded-[3px] border font-medium text-[13.5px] font-mono shadow-xs transition-all duration-150 cursor-pointer text-left align-baseline {markClass} hover:scale-[1.02]"
                      >{#if isTypeDisabled}<span class="line-through decoration-stone-400">{seg.original || seg.text}</span>{:else if !isBefore}{appState.processingMode === 'delete' ? '[УДАЛЕНО]' : (seg.anonymized || seg.text)}{:else}{seg.original || seg.text}{/if}</button>{/if}{/each}
                  </p>
                {:else if block.type === 'table' && block.table}
                  <!-- Word/ODT Table Simulation -->
                  <div class="my-5 overflow-x-auto border border-[#D5D3CC] shadow-xs bg-white rounded-none">
                    <table class="w-full border-collapse text-left text-xs font-sans">
                      <tbody>
                        {#each block.table.rows as row, rIdx}
                          <tr class="{row.isHeader ? 'bg-[#F5F4F0] font-semibold text-[#1C1917] border-b-2 border-[#D5D3CC]' : 'border-b border-[#E8E7E3] hover:bg-[#FAF9F5]/70'}">
                            {#each row.cells as cell, cIdx}
                              <td class="p-2 sm:p-2.5 border-r border-[#E8E7E3] last:border-r-0 align-top break-words whitespace-pre-wrap">
                                {#each cell.segments as seg, sIdx}{#if !seg.isEntity}<span>{seg.text}</span>{:else}{@const gIdx = entityIndexMap[`${bIdx}_${rIdx}_${cIdx}_${sIdx}`] || 0}{@const isTypeDisabled = isEntityTypeDisabled(seg.type || '')}{@const markClass = isTypeDisabled
                                    ? 'bg-stone-100 text-stone-600 border-dashed border-stone-400 hover:bg-stone-200 hover:border-stone-600'
                                    : !isBefore
                                    ? appState.processingMode === 'delete'
                                      ? 'bg-rose-100/95 text-rose-950 border-rose-400 hover:bg-rose-200'
                                      : 'bg-emerald-100/95 text-emerald-950 border-emerald-400 hover:bg-emerald-200'
                                    : 'bg-amber-100/95 text-amber-950 border-amber-400 hover:bg-amber-200'}<button
                                    type="button"
                                    id="doc-entity-mark-{gIdx}"
                                    onmouseenter={(e) => handleMarkHover(e, seg)}
                                    onmouseleave={handleMarkLeave}
                                    onclick={() => {
                                      currentActiveChangeIdx = gIdx;
                                      toggleEntityType(seg.type || '');
                                    }}
                                    class="inline px-1 py-0.5 rounded-[3px] border font-medium text-[12px] font-mono shadow-xs transition-all duration-150 cursor-pointer text-left align-baseline {markClass} hover:scale-[1.02]"
                                  >{#if isTypeDisabled}<span class="line-through decoration-stone-400">{seg.original || seg.text}</span>{:else if !isBefore}{appState.processingMode === 'delete' ? '[УДАЛЕНО]' : (seg.anonymized || seg.text)}{:else}{seg.original || seg.text}{/if}</button>{/if}{/each}
                              </td>
                            {/each}
                          </tr>
                        {/each}
                      </tbody>
                    </table>
                  </div>
                {/if}
              {/each}
            </div>
          {:else if paragraphPreviews.length > 0}
            <!-- Fallback: Full Document Paragraphs Rendered with Highlighted Spans -->
            <div>
              {#each paragraphPreviews as para, pIdx}
                <p class="mb-4 last:mb-0 whitespace-pre-wrap leading-relaxed text-justify break-words">
                  {#each para.segments as seg, sIdx}
                    {#if !seg.isEntity}
                      <span>{seg.text}</span>
                    {:else}
                      {@const gIdx = entityIndexMap[`${pIdx}_${sIdx}`] || 0}
                      {@const isTypeDisabled = isEntityTypeDisabled(seg.type || '')}
                      {@const markClass = isTypeDisabled
                        ? 'bg-stone-100 text-stone-600 border-dashed border-stone-400 hover:bg-stone-200 hover:border-stone-600'
                        : !isBefore
                        ? appState.processingMode === 'delete'
                          ? 'bg-rose-100/95 text-rose-950 border-rose-400 hover:bg-rose-200'
                          : 'bg-emerald-100/95 text-emerald-950 border-emerald-400 hover:bg-emerald-200'
                        : 'bg-amber-100/95 text-amber-950 border-amber-400 hover:bg-amber-200'}
                      
                      <button
                        type="button"
                        id="doc-entity-mark-{gIdx}"
                        onmouseenter={(e) => handleMarkHover(e, seg)}
                        onmouseleave={handleMarkLeave}
                        onclick={() => {
                          currentActiveChangeIdx = gIdx;
                          toggleEntityType(seg.type || '');
                        }}
                        class="inline px-1.5 py-0.5 rounded-[3px] border font-medium text-[13.5px] font-mono shadow-xs transition-all duration-150 cursor-pointer text-left align-baseline {markClass} hover:scale-[1.02]"
                      >
                        {#if isTypeDisabled}
                          <!-- When disabled, always show original text -->
                          <span class="line-through decoration-stone-400">{seg.original || seg.text}</span>
                        {:else if !isBefore}
                          {appState.processingMode === 'delete' ? '[УДАЛЕНО]' : (seg.anonymized || seg.text)}
                        {:else}
                          {seg.original || seg.text}
                        {/if}
                      </button>
                    {/if}
                  {/each}
                </p>
              {/each}
            </div>
          {:else if snippets.length === 0}
            <div class="text-center py-16 text-xs text-[#78716C] font-mono">
              В документе не обнаружено фрагментов персональных данных для замены
            </div>
          {:else}
            <div>
              {#each snippets as snip, sIdx}
                {@const isTypeDisabled = isEntityTypeDisabled(snip.type)}
                <p class="mb-4 last:mb-0 whitespace-pre-wrap leading-relaxed">
                  <span class="text-[#78716C] font-mono text-xs mr-2">[{sIdx + 1}]</span>
                  {#if isTypeDisabled}
                    <button
                      type="button"
                      onclick={() => toggleEntityType(snip.type)}
                      class="bg-stone-100 text-stone-600 font-medium px-1.5 py-0.5 rounded-[3px] border border-dashed border-stone-400 transition hover:bg-stone-200 cursor-pointer font-mono text-[13px] line-through"
                    >
                      {snip.original}
                    </button>
                  {:else if !isBefore}
                    <button
                      type="button"
                      onclick={() => toggleEntityType(snip.type)}
                      class="{appState.processingMode === 'delete' ? 'bg-rose-100/90 text-rose-950 border-rose-300 hover:bg-rose-200' : 'bg-emerald-100/90 text-emerald-950 border-emerald-300 hover:bg-emerald-200'} font-medium px-1.5 py-0.5 rounded-[3px] border transition cursor-pointer font-mono text-[13px]"
                    >
                      {appState.processingMode === 'delete' ? '[УДАЛЕНО]' : snip.anonymized}
                    </button>
                  {:else}
                    <button
                      type="button"
                      onclick={() => toggleEntityType(snip.type)}
                      class="bg-amber-100/90 text-amber-950 font-medium px-1.5 py-0.5 rounded-[3px] border border-amber-300 transition hover:bg-amber-200 cursor-pointer font-mono text-[13px]"
                    >
                      {snip.original}
                    </button>
                  {/if}
                </p>
              {/each}
            </div>
          {/if}
        </div>

        <!-- Floating Popover / Tooltip on Hover -->
        {#if hoveredEntity}
          <div
            class="fixed z-50 pointer-events-none transform -translate-x-1/2 -translate-y-full mb-2 bg-[#1C1917] text-white p-3.5 shadow-2xl border border-stone-700 rounded-none max-w-sm font-sans animate-fade-in"
            style="left: {hoveredEntity.x}px; top: {hoveredEntity.y}px;"
          >
            <div class="flex items-center justify-between gap-2 border-b border-stone-700 pb-2 mb-2 font-mono text-[10px]">
              <div class="flex items-center space-x-1.5">
                <span
                  class="px-1.5 py-0.5 {hoveredEntity.isDisabled
                    ? 'bg-stone-800 border border-stone-600 text-stone-400'
                    : 'bg-emerald-950 border border-emerald-600 text-emerald-300'} font-bold uppercase"
                >
                  {getEntityTypeName(hoveredEntity.type)}
                </span>
                <span
                  class="px-1.5 py-0.5 {hoveredEntity.isDisabled
                    ? 'bg-amber-950 border border-amber-600 text-amber-300'
                    : appState.processingMode === 'delete'
                      ? 'bg-rose-950 border border-rose-600 text-rose-300'
                      : 'bg-emerald-950 border border-emerald-600 text-emerald-300'} text-[9px] font-bold"
                >
                  {hoveredEntity.isDisabled ? '○ ОТКЛЮЧЕНО' : appState.processingMode === 'delete' ? '● УДАЛЯЕТСЯ' : '● ЗАМЕНЯЕТСЯ'}
                </span>
              </div>
              <span class="text-stone-400 text-[10px]">
                {getEntityAction(hoveredEntity.type)}
              </span>
            </div>

            <div class="space-y-1.5 text-xs">
              <div class="flex items-start justify-between gap-2">
                <span class="text-stone-400 font-mono text-[11px] shrink-0">До:</span>
                <span class="font-mono text-rose-300 text-[11px] break-all text-right font-medium">
                  {hoveredEntity.original}
                </span>
              </div>
              <div class="flex items-start justify-between gap-2">
                <span class="text-stone-400 font-mono text-[11px] shrink-0">{appState.processingMode === 'delete' ? 'После удаления:' : 'После:'}</span>
                <span class="font-mono {hoveredEntity.isDisabled ? 'text-stone-400 line-through' : appState.processingMode === 'delete' ? 'text-rose-300' : 'text-emerald-300'} text-[11px] break-all text-right font-medium">
                  {hoveredEntity.isDisabled ? hoveredEntity.original : (appState.processingMode === 'delete' ? '[УДАЛЕНО]' : hoveredEntity.anonymized)}
                </span>
              </div>
            </div>

            <div class="mt-2.5 pt-2 border-t border-stone-800 text-[10px] text-stone-400 font-mono flex items-center justify-between">
              <span>💡 Кликните для {hoveredEntity.isDisabled ? 'включения' : 'отключения'} замены</span>
            </div>
          </div>
        {/if}
      </div>
  </div>
{/if}
