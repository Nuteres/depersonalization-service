<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import type { DocxTextSnippet, TextParagraphPreview, TextParagraphSegment } from '../../types/domain';

  // View state
  let isBefore = $state(false); // false = Blackout Redacted, true = Original
  let hoveredEntity = $state<{
    type: string;
    original: string;
    anonymized: string;
    isDisabled: boolean;
    x: number;
    y: number;
  } | null>(null);

  const fileInfo = $derived(appState.currentFileInfo);
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
    FIO: 'Черная векторная плашка',
    SNILS: 'Черная векторная плашка',
    PASSPORT: 'Черная векторная плашка',
    PHONE: 'Черная векторная плашка',
    INN: 'Черная векторная плашка',
    BANK_CARD: 'Черная векторная плашка',
    CARD: 'Черная векторная плашка',
    DATE: 'Черная векторная плашка',
    EMAIL: 'Черная векторная плашка',
    ADDRESS: 'Черная векторная плашка',
    GRADE: 'Черная векторная плашка',
    EMPLOYEE_ID: 'Черная векторная плашка'
  };

  function getEntityTypeName(type: string): string {
    return typeLabels[type] || type || 'ПДн';
  }

  function getEntityAction(type: string): string {
    return typeActionLabels[type] || 'Черная векторная плашка';
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

  // Linear entity marks for navigation
  interface FlatEntityMark {
    globalIndex: number;
    paraIndex: number;
    segIndex: number;
    type: string;
    original: string;
    anonymized: string;
    elementId: string;
  }

  const allEntityMarks = $derived.by<FlatEntityMark[]>(() => {
    const list: FlatEntityMark[] = [];
    let gIdx = 0;
    paragraphPreviews.forEach((para, pIdx) => {
      para.segments.forEach((seg, sIdx) => {
        if (seg.isEntity) {
          gIdx++;
          list.push({
            globalIndex: gIdx,
            paraIndex: pIdx,
            segIndex: sIdx,
            type: seg.type || 'FIO',
            original: seg.original || seg.text,
            anonymized: seg.anonymized || seg.text,
            elementId: `pdf-entity-mark-${gIdx}`
          });
        }
      });
    });
    return list;
  });

  const entityIndexMap = $derived.by<Record<string, number>>(() => {
    const map: Record<string, number> = {};
    for (const m of allEntityMarks) {
      map[`${m.paraIndex}_${m.segIndex}`] = m.globalIndex;
    }
    return map;
  });



  function handleMarkHover(event: MouseEvent, seg: TextParagraphSegment) {
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

{#if fileInfo && fileInfo.fileType === 'pdf'}
  <div
    id="pdf-inspection-box"
    class="bg-white border border-[#E8E7E3] shadow-sm flex flex-col select-text"
  >
    <!-- Compact Header Toolbar (Single Row) -->
    <div
      class="px-5 py-3 border-b border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-[#FAF9F5]"
    >
      <!-- Title & Document Stats -->
      <div class="flex items-center space-x-2">
        <span class="w-2 h-2 rounded-none bg-emerald-600 animate-pulse"></span>
        <span
          class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono"
        >
          Предпросмотр PDF
        </span>
        <span class="text-xs text-[#78716C] font-mono">
          ▪ {allEntityMarks.length} плашек ПДн
        </span>
      </div>

      <!-- Controls Right: Before/After Toggle -->
      <div class="flex items-center space-x-3">
        <!-- Toggle: Cleaned vs Original -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium shadow-2xs">
          <button
            type="button"
            onclick={() => (isBefore = false)}
            class="flex items-center space-x-1.5 px-3 py-1 {!isBefore
              ? 'bg-neutral-900 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-none {!isBefore ? 'bg-emerald-400' : 'bg-neutral-400'}"></span>
            <span>С плашками</span>
          </button>
          <button
            type="button"
            onclick={() => (isBefore = true)}
            class="flex items-center space-x-1.5 px-3 py-1 {isBefore
              ? 'bg-amber-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-none {isBefore ? 'bg-amber-300' : 'bg-amber-500'}"></span>
            <span>Исходный текст</span>
          </button>
        </div>
      </div>
    </div>

      <!-- Main Clean Paper Document View (Scrollable Page Container) -->
      <div
        id="pdf-sheet-viewport"
        class="p-4 sm:p-8 md:p-10 bg-[#EDEBE6] overflow-y-auto max-h-[720px] custom-scrollbar relative"
      >
        <!-- PDF Page Sheet (Full Height Block, grows with content) -->
        <div
          id="pdf-sheet-paper"
          class="mx-auto w-full max-w-3xl bg-white shadow-lg border border-[#D5D3CC] p-6 sm:p-10 md:p-12 text-[#1C1917] font-sans text-[14px] sm:text-[15px] leading-[1.8] min-h-[300px] h-auto"
        >
          <!-- Paragraphs Render Area -->
          {#if paragraphPreviews.length > 0}
            <div class="space-y-1">
              {#each paragraphPreviews as para, pIdx}
                <p class="mb-1.5 whitespace-pre-wrap leading-relaxed text-left break-words">
                  {#each para.segments as seg, sIdx}
                    {#if seg.isEntity}
                      {@const globalIdx = entityIndexMap[`${pIdx}_${sIdx}`]}
                      {@const entType = seg.type || 'FIO'}
                      {@const disabled = isEntityTypeDisabled(entType)}

                      {#if isBefore}
                        <!-- Original text mode -->
                        <span
                          id={`pdf-entity-mark-${globalIdx}`}
                          class="font-semibold bg-amber-50 text-amber-900 px-1 py-0.5 rounded-none border-b-2 border-amber-500 transition-colors"
                        >
                          {seg.original || seg.text}
                        </span>
                      {:else if disabled}
                        <!-- Blackout Disabled (Revealed original text with dashed plate) -->
                        <button
                          type="button"
                          id={`pdf-entity-mark-${globalIdx}`}
                          onclick={() => toggleEntityType(entType)}
                          onmouseenter={(e) => handleMarkHover(e, seg)}
                          onmouseleave={handleMarkLeave}
                          class="inline-flex items-center gap-1 px-1.5 py-0.5 mx-0.5 bg-stone-100 border border-dashed border-stone-400 text-stone-700 font-mono text-xs rounded-none transition-all cursor-pointer hover:bg-stone-200 hover:border-stone-600 align-baseline select-none"
                        >
                          <span class="font-sans font-medium text-stone-900">{seg.original || seg.text}</span>
                          <span class="text-[9px] text-stone-500 font-mono bg-stone-200 px-1 rounded-none">
                            ▫ Без плашки
                          </span>
                        </button>
                      {:else}
                        <!-- Vector Blackout Redaction Plate (Solid Black Bar) -->
                        <button
                          type="button"
                          id={`pdf-entity-mark-${globalIdx}`}
                          onclick={() => toggleEntityType(entType)}
                          onmouseenter={(e) => handleMarkHover(e, seg)}
                          onmouseleave={handleMarkLeave}
                          class="inline-flex items-center gap-1.5 px-2 py-0.5 mx-0.5 bg-[#1C1917] hover:bg-neutral-800 text-white font-mono text-[11px] font-bold rounded-none shadow-xs transition-all cursor-pointer hover:ring-2 hover:ring-indigo-500 hover:ring-offset-1 align-baseline select-none"
                        >
                          <span class="tracking-wider">[ ПДн СКРЫТО ]</span>
                          <span
                            class="text-[9px] uppercase px-1 py-0.2 bg-neutral-700 text-neutral-200 font-mono font-normal rounded-none"
                          >
                            {getEntityTypeName(entType)}
                          </span>
                        </button>
                      {/if}
                    {:else}
                      <!-- Regular Text Run -->
                      <span>{seg.text}</span>
                    {/if}
                  {/each}
                </p>
              {/each}
            </div>
          {:else if fileInfo.pdfTextSnippet}
            <!-- Fallback raw snippet view -->
            <div class="whitespace-pre-wrap font-mono text-xs text-[#1C1917] leading-6">
              {isBefore ? fileInfo.pdfTextSnippet : (fileInfo.pdfRedactedSnippet || fileInfo.pdfTextSnippet)}
            </div>
          {/if}
        </div>
      </div>


    <!-- Hover Floating Tooltip on Entity Blackout Plate -->
    {#if hoveredEntity}
      <div
        class="fixed z-50 transform -translate-x-1/2 -translate-y-full pointer-events-none bg-[#1C1917] text-white p-3 shadow-2xl border border-neutral-700 text-xs rounded-none min-w-[280px] max-w-sm transition-opacity duration-150"
        style="left: {hoveredEntity.x}px; top: {hoveredEntity.y}px;"
      >
        <div class="flex items-center justify-between gap-3 mb-2 pb-1.5 border-b border-neutral-700">
          <span class="font-bold text-[11px] uppercase tracking-wider text-emerald-400 font-mono">
            {getEntityTypeName(hoveredEntity.type)}
          </span>
          <span
            class="text-[10px] font-mono px-1.5 py-0.5 rounded-none {hoveredEntity.isDisabled
              ? 'bg-stone-700 text-stone-300'
              : 'bg-emerald-950 text-emerald-300 border border-emerald-700'}"
          >
            {hoveredEntity.isDisabled ? '▫ Плашка снята' : '▪ Закрыто плашкой'}
          </span>
        </div>

        <div class="space-y-1.5 font-mono text-[11px] leading-snug">
          <div class="text-neutral-400">
            Исходное: <span class="text-neutral-100 font-semibold">{hoveredEntity.original}</span>
          </div>
          <div class="text-neutral-400">
            В PDF:
            <span class="text-neutral-200">
              {hoveredEntity.isDisabled ? hoveredEntity.original : '[ ПДн СКРЫТО ]'}
            </span>
          </div>
        </div>

        <div class="mt-2.5 pt-1.5 border-t border-neutral-800 text-[10px] text-neutral-400 flex items-center gap-1.5">
          <span class="text-emerald-400">💡</span>
          <span>Нажмите на плашку для переключения</span>
        </div>
      </div>
    {/if}
  </div>
{/if}
