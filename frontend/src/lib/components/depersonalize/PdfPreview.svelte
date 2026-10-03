<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import type { DocxTextSnippet, TextParagraphPreview, TextParagraphSegment } from '../../types/domain';

  // View modes
  let viewMode = $state<'doc' | 'snippets'>('doc');
  let isBefore = $state(false); // false = Blackout Redacted, true = Original
  let selectedEntityType = $state<string>('ALL');
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

  const filteredEntityMarks = $derived.by<FlatEntityMark[]>(() => {
    if (selectedEntityType === 'ALL') {
      return allEntityMarks;
    }
    return allEntityMarks.filter((m) => m.type === selectedEntityType);
  });

  const availableEntityTypes = $derived.by(() => {
    const map = new Map<string, number>();
    for (const m of allEntityMarks) {
      map.set(m.type, (map.get(m.type) || 0) + 1);
    }
    return Array.from(map.entries()).map(([type, count]) => ({
      type,
      name: getEntityTypeName(type),
      count,
      disabled: isEntityTypeDisabled(type)
    }));
  });

  function scrollToEntity(globalIdx: number) {
    if (globalIdx < 1 || globalIdx > allEntityMarks.length) return;
    currentActiveChangeIdx = globalIdx;
    const elem = document.getElementById(`pdf-entity-mark-${globalIdx}`);
    if (elem) {
      elem.scrollIntoView({ behavior: 'smooth', block: 'center' });
      elem.classList.add('ring-4', 'ring-indigo-500/80', 'ring-offset-2');
      setTimeout(() => {
        elem.classList.remove('ring-4', 'ring-indigo-500/80', 'ring-offset-2');
      }, 1800);
    }
  }

  function nextChange() {
    if (filteredEntityMarks.length === 0) return;
    const currentPos = filteredEntityMarks.findIndex(
      (m) => m.globalIndex === currentActiveChangeIdx
    );
    if (currentPos === -1 || currentPos >= filteredEntityMarks.length - 1) {
      const first = filteredEntityMarks[0];
      if (first) {
        scrollToEntity(first.globalIndex);
      }
    } else {
      const next = filteredEntityMarks[currentPos + 1];
      if (next) {
        scrollToEntity(next.globalIndex);
      }
    }
  }

  function prevChange() {
    if (filteredEntityMarks.length === 0) return;
    const currentPos = filteredEntityMarks.findIndex(
      (m) => m.globalIndex === currentActiveChangeIdx
    );
    if (currentPos <= 0) {
      const last = filteredEntityMarks[filteredEntityMarks.length - 1];
      if (last) {
        scrollToEntity(last.globalIndex);
      }
    } else {
      const prev = filteredEntityMarks[currentPos - 1];
      if (prev) {
        scrollToEntity(prev.globalIndex);
      }
    }
  }

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
    <!-- Top Main Header Toolbar -->
    <div
      class="px-5 py-3.5 border-b border-[#E8E7E3] flex flex-col lg:flex-row lg:items-center justify-between gap-3.5 bg-[#FAF9F5]"
    >
      <!-- Title & Document Stats -->
      <div class="flex items-center flex-wrap gap-2.5">
        <div class="flex items-center space-x-2">
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-600 animate-pulse"></span>
          <span
            class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono"
          >
            Предпросмотр PDF (Векторные плашки Blackout)
          </span>
        </div>
        <span class="text-xs text-[#78716C] font-mono">
          · {allEntityMarks.length} плашек ПДн
        </span>
        {#if fileInfo.pageCount}
          <span class="text-xs text-[#A8A29E] font-mono">
            · {fileInfo.pageCount} {fileInfo.pageCount === 1 ? 'страница' : 'страницы'}
          </span>
        {/if}
        {#if paragraphPreviews.length}
          <span class="text-xs text-[#A8A29E] font-mono">
            · {paragraphPreviews.length} строк/абзацев
          </span>
        {/if}
      </div>

      <!-- Controls Right: Mode Switcher & Before/After Toggle -->
      <div class="flex items-center flex-wrap gap-3">
        <!-- View Mode: Document Sheet vs Snippet Cards -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
          <button
            type="button"
            onclick={() => (viewMode = 'doc')}
            class="flex items-center space-x-1.5 px-3 py-1.5 {viewMode === 'doc'
              ? 'bg-[#1C1917] text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
            </svg>
            <span>Вид документа PDF</span>
          </button>
          <button
            type="button"
            onclick={() => (viewMode = 'snippets')}
            class="flex items-center space-x-1.5 px-3 py-1.5 {viewMode === 'snippets'
              ? 'bg-[#1C1917] text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
            </svg>
            <span>Реестр плашек ({allEntityMarks.length})</span>
          </button>
        </div>

        <!-- Toggle: Cleaned vs Original -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
          <button
            type="button"
            onclick={() => (isBefore = false)}
            class="flex items-center space-x-1 px-3 py-1.5 {!isBefore
              ? 'bg-neutral-900 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-full {!isBefore ? 'bg-emerald-400' : 'bg-neutral-400'}"></span>
            <span>С плашками [ ПДн СКРЫТО ]</span>
          </button>
          <button
            type="button"
            onclick={() => (isBefore = true)}
            class="flex items-center space-x-1 px-3 py-1.5 {isBefore
              ? 'bg-amber-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-full {isBefore ? 'bg-amber-300' : 'bg-amber-500'}"></span>
            <span>Исходный текст</span>
          </button>
        </div>
      </div>
    </div>

    {#if viewMode === 'doc'}
      <!-- Secondary Sub-toolbar: Entity Filter Pills & Change Navigator -->
      <div
        class="px-5 py-2.5 border-b border-[#E8E7E3] bg-[#F5F4F0] flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs"
      >
        <!-- Category Filter Pills with status -->
        <div class="flex items-center flex-wrap gap-1.5">
          <span class="text-[11px] font-mono text-[#78716C] mr-1 uppercase font-medium">
            Фильтр:
          </span>
          <button
            type="button"
            onclick={() => {
              selectedEntityType = 'ALL';
              currentActiveChangeIdx = 1;
            }}
            class="px-2.5 py-1 text-[11px] font-mono transition cursor-pointer rounded-none border {selectedEntityType ===
            'ALL'
              ? 'bg-[#1C1917] text-white border-[#1C1917] font-semibold'
              : 'bg-white text-[#78716C] border-[#E8E7E3] hover:text-[#1C1917] hover:border-[#1C1917]'}"
          >
            Все ({allEntityMarks.length})
          </button>

          {#each availableEntityTypes as et}
            <button
              type="button"
              onclick={() => {
                selectedEntityType = et.type;
                const first = allEntityMarks.find((m) => m.type === et.type);
                if (first) scrollToEntity(first.globalIndex);
              }}
              title={et.disabled ? 'Плашка отключена (данные не закрываются)' : 'Плашка активна (данные закрываются)'}
              class="flex items-center space-x-1.5 px-2.5 py-1 text-[11px] font-mono transition cursor-pointer rounded-none border {selectedEntityType ===
              et.type
                ? 'bg-[#1C1917] text-white border-[#1C1917] font-semibold'
                : 'bg-white text-[#78716C] border-[#E8E7E3] hover:text-[#1C1917] hover:border-[#1C1917]'} {et.disabled ? 'opacity-70' : ''}"
            >
              <span
                class="w-1.5 h-1.5 rounded-full {et.disabled
                  ? 'bg-stone-400'
                  : 'bg-emerald-500'}"
              ></span>
              <span>{et.name} ({et.count})</span>
              {#if et.disabled}
                <span class="text-[9px] text-stone-500 font-sans">(откл)</span>
              {/if}
            </button>
          {/each}
        </div>

        <!-- Linear Change Navigator -->
        {#if filteredEntityMarks.length > 0}
          <div class="flex items-center space-x-2 shrink-0">
            <span class="text-[11px] font-mono text-[#78716C]">
              Плашка {filteredEntityMarks.findIndex(
                (m) => m.globalIndex === currentActiveChangeIdx
              ) + 1} из {filteredEntityMarks.length}
            </span>
            <div class="inline-flex border border-[#E8E7E3] bg-white">
              <button
                type="button"
                onclick={prevChange}
                title="Предыдущая плашка"
                class="px-2 py-1 text-xs text-[#78716C] hover:text-[#1C1917] hover:bg-[#FAF9F5] border-r border-[#E8E7E3] cursor-pointer"
              >
                ◀
              </button>
              <button
                type="button"
                onclick={nextChange}
                title="Следующая плашка"
                class="px-2 py-1 text-xs text-[#78716C] hover:text-[#1C1917] hover:bg-[#FAF9F5] cursor-pointer"
              >
                ▶
              </button>
            </div>
          </div>
        {/if}
      </div>

      <!-- Main Clean Paper Document View (Scrollable Page Container) -->
      <div class="bg-[#E5E4DE] p-4 sm:p-10 flex justify-center overflow-x-auto min-h-[500px]">
        <div
          class="bg-white p-6 sm:p-12 md:p-16 max-w-3xl w-full shadow-lg border border-[#D6D3D1] text-[#1C1917] font-serif leading-relaxed text-[13px] relative flex flex-col gap-4"
        >
          <!-- Document Watermark Header -->
          <div
            class="flex items-center justify-between pb-4 border-b border-[#E8E7E3] text-[11px] font-sans text-[#78716C] mb-2"
          >
            <div class="flex items-center space-x-2">
              <span class="font-bold text-[#1C1917] font-mono uppercase tracking-wider">
                ДОКУМЕНТ PDF
              </span>
              <span>· {fileInfo.name}</span>
            </div>
            <div class="font-mono text-[10px] uppercase tracking-wider text-emerald-800 bg-emerald-50 px-2 py-0.5 border border-emerald-200">
              152-ФЗ Blackout Redaction
            </div>
          </div>

          <!-- Paragraphs Render Area -->
          {#if paragraphPreviews.length > 0}
            <div class="space-y-3.5">
              {#each paragraphPreviews as para, pIdx}
                <div class="group relative flex items-start">
                  <!-- Paragraph Number gutter -->
                  <span
                    class="w-6 shrink-0 text-[10px] font-mono text-[#D6D3D1] group-hover:text-[#A8A29E] select-none text-right pr-2 pt-0.5"
                  >
                    {para.index}
                  </span>

                  <!-- Paragraph Text Content -->
                  <p class="flex-1 text-justify text-[#1C1917] leading-relaxed">
                    {#each para.segments as seg, sIdx}
                      {#if seg.isEntity}
                        {@const globalIdx = entityIndexMap[`${pIdx}_${sIdx}`]}
                        {@const entType = seg.type || 'FIO'}
                        {@const disabled = isEntityTypeDisabled(entType)}

                        {#if isBefore}
                          <!-- Original text mode -->
                          <span
                            id={`pdf-entity-mark-${globalIdx}`}
                            class="font-semibold bg-amber-50 text-amber-900 px-1 py-0.5 rounded-xs border-b-2 border-amber-500 transition-colors"
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
                            class="inline-flex items-center gap-1 px-1.5 py-0.5 mx-0.5 bg-stone-100 border border-dashed border-stone-400 text-stone-700 font-mono text-xs rounded transition-all cursor-pointer hover:bg-stone-200 hover:border-stone-600 align-baseline select-none"
                            title="Плашка снята. Нажмите для повторного закрытия плашкой"
                          >
                            <span class="font-sans font-medium text-stone-900">{seg.original || seg.text}</span>
                            <span class="text-[9px] text-stone-500 font-mono bg-stone-200 px-1 rounded">
                              ○ Без плашки
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
                            class="inline-flex items-center gap-1.5 px-2 py-0.5 mx-0.5 bg-[#1C1917] hover:bg-neutral-800 text-white font-mono text-[11px] font-bold rounded shadow-xs transition-all cursor-pointer hover:ring-2 hover:ring-indigo-500 hover:ring-offset-1 align-baseline select-none"
                            title={`Закрыто плашкой [ ${getEntityTypeName(entType)} ]. Нажмите для снятия плашки.`}
                          >
                            <span class="tracking-wider">[ ПДн СКРЫТО ]</span>
                            <span
                              class="text-[9px] uppercase px-1 py-0.2 bg-neutral-700 text-neutral-200 font-mono font-normal rounded-xs"
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
                </div>
              {/each}
            </div>
          {:else if fileInfo.pdfTextSnippet}
            <!-- Fallback raw snippet view -->
            <div class="whitespace-pre-wrap font-mono text-xs text-[#1C1917] leading-6">
              {isBefore ? fileInfo.pdfTextSnippet : (fileInfo.pdfRedactedSnippet || fileInfo.pdfTextSnippet)}
            </div>
          {/if}

          <!-- Document Footer -->
          <div
            class="mt-12 pt-6 border-t border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 text-[11px] font-sans text-[#78716C]"
          >
            <div class="flex items-center space-x-2">
              <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
              <span>Физическое стирание символов в PDF стримах и закрашивание векторными плашками</span>
            </div>
            <div class="font-mono text-[11px] text-[#A8A29E]">
              152-ФЗ РФ · Конфиденциально
            </div>
          </div>
        </div>
      </div>
    {:else}
      <!-- Snippet Cards Mode (List of Redactions) -->
      <div class="p-6 bg-[#FAF9F5]">
        <div class="flex items-center justify-between mb-4">
          <div class="text-xs font-bold font-mono text-[#1C1917] uppercase tracking-wider">
            Реестр обнаруженных плашек ({snippets.length})
          </div>
          <div class="text-xs text-[#78716C]">
            Нажмите на карточку для включения или отключения закрытия плашкой
          </div>
        </div>

        {#if snippets.length > 0}
          <div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3.5">
            {#each snippets as snip}
              {@const entType = snip.type || 'FIO'}
              {@const disabled = isEntityTypeDisabled(entType)}
              <div
                class="bg-white border {disabled
                  ? 'border-dashed border-stone-300 opacity-75'
                  : 'border-[#E8E7E3] hover:border-[#1C1917]'} p-4 shadow-xs flex flex-col justify-between transition-all cursor-pointer select-none"
                onclick={() => toggleEntityType(entType)}
                role="button"
                tabindex="0"
                onkeydown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') toggleEntityType(entType);
                }}
              >
                <div>
                  <div class="flex items-center justify-between mb-2">
                    <span
                      class="px-2 py-0.5 text-[10px] font-mono uppercase font-bold rounded-xs {disabled
                        ? 'bg-stone-100 text-stone-500'
                        : 'bg-neutral-900 text-white'}"
                    >
                      {getEntityTypeName(entType)}
                    </span>
                    <span
                      class="text-[11px] font-mono {disabled
                        ? 'text-stone-400'
                        : 'text-emerald-700 font-semibold'}"
                    >
                      {disabled ? '○ Отключено' : '● Плашка активна'}
                    </span>
                  </div>

                  <div class="space-y-1.5 my-3">
                    <div class="text-xs">
                      <span class="text-[#A8A29E] text-[10px] font-mono block">Исходное значение:</span>
                      <span class="font-medium text-[#1C1917] bg-stone-50 px-1.5 py-0.5 rounded border border-stone-200 inline-block">
                        {snip.original}
                      </span>
                    </div>
                    <div class="text-xs">
                      <span class="text-[#A8A29E] text-[10px] font-mono block">Результат в PDF:</span>
                      {#if disabled}
                        <span class="text-stone-600 font-mono text-xs">
                          {snip.original} (без изменений)
                        </span>
                      {:else}
                        <span class="inline-block bg-[#1C1917] text-white font-mono text-xs px-2 py-0.5 rounded">
                          [ ПДн СКРЫТО ]
                        </span>
                      {/if}
                    </div>
                  </div>
                </div>

                <div class="pt-2 border-t border-[#F5F4F0] flex items-center justify-between text-[11px] text-[#78716C]">
                  <span>Действие: {getEntityAction(entType)}</span>
                  <span class="text-indigo-600 hover:underline font-mono text-[10px]">
                    {disabled ? 'Включить плашку' : 'Отключить'}
                  </span>
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <div class="p-8 text-center text-xs text-[#78716C] bg-white border border-[#E8E7E3]">
            Плашек персональных данных не обнаружено
          </div>
        {/if}
      </div>
    {/if}

    <!-- Hover Floating Tooltip on Entity Blackout Plate -->
    {#if hoveredEntity}
      <div
        class="fixed z-50 transform -translate-x-1/2 -translate-y-full pointer-events-none bg-[#1C1917] text-white p-3 shadow-2xl border border-neutral-700 text-xs rounded max-w-xs transition-opacity duration-150"
        style="left: {hoveredEntity.x}px; top: {hoveredEntity.y}px;"
      >
        <div class="flex items-center justify-between gap-3 mb-1.5 pb-1 border-b border-neutral-700">
          <span class="font-bold text-[11px] uppercase tracking-wider text-emerald-400 font-mono">
            {getEntityTypeName(hoveredEntity.type)}
          </span>
          <span
            class="text-[10px] font-mono px-1.5 py-0.2 rounded {hoveredEntity.isDisabled
              ? 'bg-stone-700 text-stone-300'
              : 'bg-emerald-950 text-emerald-300 border border-emerald-700'}"
          >
            {hoveredEntity.isDisabled ? '○ Плашка снята' : '● Закрыто плашкой'}
          </span>
        </div>

        <div class="space-y-1 font-mono text-[11px] leading-tight">
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

        <div class="mt-2 pt-1 border-t border-neutral-800 text-[10px] text-neutral-400 flex items-center justify-between">
          <span>Нажмите на плашку для переключения</span>
          <span class="text-emerald-400 font-mono">Клик ↲</span>
        </div>
      </div>
    {/if}
  </div>
{/if}
