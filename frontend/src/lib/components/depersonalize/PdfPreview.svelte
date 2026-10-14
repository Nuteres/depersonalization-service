<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { loadPdf, type PDFDocumentProxy } from '../../utils/pdfWorker';
  import { GetFileBase64 } from '../../api/wails';
  import type { PDFRedactionBox } from '../../types/domain';

  // View state
  let isBefore = $state(false); // false = Anonymized, true = Original
  let isLoading = $state(false);
  let loadError = $state<string | null>(null);
  let disabledInstanceIds = $state<Set<string>>(new Set());

  let hoveredEntity = $state<{
    type: string;
    original: string;
    anonymized: string;
    isDisabled: boolean;
    x: number;
    y: number;
  } | null>(null);

  interface PageOverlayEntity {
    instanceId: string;
    pageNum: number;
    type: string;
    original: string;
    anonymized: string;
    cssRect: { left: number; top: number; width: number; height: number };
    pdfBox: PDFRedactionBox;
  }

  interface RenderedPageData {
    pageNum: number;
    width: number;
    height: number;
    entities: PageOverlayEntity[];
  }

  let pagesData = $state<RenderedPageData[]>([]);
  let canvasRefs: Record<number, HTMLCanvasElement> = {};

  const fileInfo = $derived(appState.currentFileInfo);

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

  function getEntityTypeName(type: string): string {
    return typeLabels[type] || type || 'ПДн';
  }

  // Count total detected entities across all pages
  const allDetectedEntities = $derived.by<PageOverlayEntity[]>(() => {
    return pagesData.flatMap((p) => p.entities);
  });

  // Calculate active PDF redaction boxes and keep appState synchronized
  $effect(() => {
    const activeBoxes: PDFRedactionBox[] = [];
    for (const ent of allDetectedEntities) {
      if (!disabledInstanceIds.has(ent.instanceId)) {
        activeBoxes.push(ent.pdfBox);
      }
    }
    appState.pdfRedactionBoxes = activeBoxes;
  });

  function toggleInstance(instanceId: string) {
    const next = new Set(disabledInstanceIds);
    if (next.has(instanceId)) {
      next.delete(instanceId);
    } else {
      next.add(instanceId);
    }
    disabledInstanceIds = next;

    if (hoveredEntity) {
      hoveredEntity = {
        ...hoveredEntity,
        isDisabled: next.has(instanceId)
      };
    }
  }

  function handleChipHover(event: MouseEvent, ent: PageOverlayEntity) {
    const target = event.currentTarget as HTMLElement;
    const rect = target.getBoundingClientRect();
    hoveredEntity = {
      type: ent.type,
      original: ent.original,
      anonymized: ent.anonymized,
      isDisabled: disabledInstanceIds.has(ent.instanceId),
      x: rect.left + rect.width / 2,
      y: rect.top - 8
    };
  }

  function handleChipLeave() {
    hoveredEntity = null;
  }

  function base64ToUint8Array(base64: string): Uint8Array {
    const binary = atob(base64);
    const len = binary.length;
    const bytes = new Uint8Array(len);
    for (let i = 0; i < len; i++) {
      bytes[i] = binary.charCodeAt(i);
    }
    return bytes;
  }

  // Redaction marker for PDF preview (100% unified Blackout Redaction)
  function makeAnonymizedValue(): string {
    return '[ ПДн СКРЫТО ]';
  }

  // Regex patterns for Russian personal data detection
  const piiPatterns: Array<{ type: string; regex: RegExp }> = [
    {
      type: 'PHONE',
      regex: /(?:(?:\+?\s*7|8)(?:[\s\-\(\).]*\d){10})/g
    },
    {
      type: 'PASSPORT',
      regex: /(?:(?:паспорт|серия|номер|№)[:\s]*)?\b\d{4}\s+\d{6}\b|\b\d{2}\s+\d{2}\s+\d{6}\b/gi
    },
    {
      type: 'SNILS',
      regex: /\b\d{3}[-\s]\d{3}[-\s]\d{3}[-\s]\d{2}\b/g
    },
    {
      type: 'INN',
      regex: /(?:ИНН[:\s]*)?\b\d{10}\b|\b\d{12}\b/gi
    },
    {
      type: 'CARD',
      regex: /\b(?:\d{4}[\s\-]?){4}\b/g
    },
    {
      type: 'EMAIL',
      regex: /[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}/g
    },
    {
      type: 'DATE',
      regex: /\b\d{1,2}\s+(?:января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\s+\d{4}(?:\s*г\.?)?|\b\d{2}[./]\d{2}[./]\d{4}\b/gi
    },
    {
      type: 'FIO',
      regex: /\b[А-ЯЁ][а-яё]+(?:-[А-ЯЁ][а-яё]+)?\s+[А-ЯЁ][а-яё]+\s+[А-ЯЁ][а-яё]+(?:ович|евич|ич|овна|евна|ична)\b|\b[А-ЯЁ][а-яё]+\s+[А-ЯЁ]\.\s*[А-ЯЁ]\.|\b[А-ЯЁ]\.\s*[А-ЯЁ]\.\s+[А-ЯЁ][а-яё]+\b/g
    },
    {
      type: 'ADDRESS',
      regex: /(?:г\.|город|ул\.|улица|пр-т|проспект|д\.|дом|кв\.|квартира)\s+[^,;\n]{2,30}(?:,\s*(?:ул\.|д\.|кв\.)\s*[^,;\n]{1,20})+/gi
    }
  ];

  let currentLoadingPath = '';

  $effect(() => {
    const path = fileInfo?.path;
    if (path && fileInfo.fileType === 'pdf' && path !== currentLoadingPath) {
      currentLoadingPath = path;
      loadPdfDocument(path);
    }
  });

  async function loadPdfDocument(path: string) {
    isLoading = true;
    loadError = null;
    pagesData = [];
    disabledInstanceIds = new Set();

    try {
      const base64Data = await GetFileBase64(path);
      if (!base64Data) {
        throw new Error('Не удалось прочитать содержимое файла');
      }

      const bytes = base64ToUint8Array(base64Data);
      const pdf = await loadPdf(bytes);
      const numPages = pdf.numPages;
      const targetScale = 1.35; // Standard desktop reading scale

      const pagesList: RenderedPageData[] = [];
      let globalCounter = 0;

      for (let pNum = 1; pNum <= numPages; pNum++) {
        const page = await pdf.getPage(pNum);
        const viewport = page.getViewport({ scale: targetScale });
        const textContent = await page.getTextContent();

        // Build continuous page string and map character ranges to text items
        interface TextItemRange {
          itemIndex: number;
          startChar: number;
          endChar: number;
          str: string;
          pdfBox: PDFRedactionBox;
          cssRect: { left: number; top: number; width: number; height: number };
        }

        const itemRanges: TextItemRange[] = [];
        let pageStr = '';

        for (let i = 0; i < textContent.items.length; i++) {
          const item = textContent.items[i] as any;
          if (!item.str) continue;

          const str = item.str;
          const start = pageStr.length;
          pageStr += str + ' ';
          const end = pageStr.length - 1;

          // Convert PDF coordinates to viewport coordinates
          const tx = item.transform[4];
          const ty = item.transform[5];
          const tw = item.width || 10;
          const th = item.height || 12;

          // PDF bounding box in raw 72 DPI points
          const pdfBox: PDFRedactionBox = {
            page: pNum,
            x: tx,
            y: ty - 2, // Slight vertical padding
            width: tw,
            height: th + 4
          };

          const viewRect = viewport.convertToViewportRectangle([tx, ty - 2, tx + tw, ty + th + 2]);
          const cssLeft = Math.min(viewRect[0], viewRect[2]);
          const cssTop = Math.min(viewRect[1], viewRect[3]);
          const cssWidth = Math.max(Math.abs(viewRect[2] - viewRect[0]), 8);
          const cssHeight = Math.max(Math.abs(viewRect[3] - viewRect[1]), 12);

          itemRanges.push({
            itemIndex: i,
            startChar: start,
            endChar: end,
            str,
            pdfBox,
            cssRect: { left: cssLeft, top: cssTop, width: cssWidth, height: cssHeight }
          });
        }

        // Run detection on pageStr
        const pageEntities: PageOverlayEntity[] = [];

        // 1. Detect via known snippets from Go inspector if available
        if (fileInfo?.textSnippets && fileInfo.textSnippets.length > 0) {
          for (const snip of fileInfo.textSnippets) {
            if (!snip.original || snip.original.length < 3) continue;
            let idx = pageStr.indexOf(snip.original);
            while (idx !== -1) {
              const matchEnd = idx + snip.original.length;
              const overlapping = itemRanges.filter(
                (r) => r.startChar < matchEnd && r.endChar > idx
              );

              if (overlapping.length > 0) {
                globalCounter++;
                const minCssLeft = Math.min(...overlapping.map((r) => r.cssRect.left));
                const minCssTop = Math.min(...overlapping.map((r) => r.cssRect.top));
                const maxCssRight = Math.max(...overlapping.map((r) => r.cssRect.left + r.cssRect.width));
                const maxCssBottom = Math.max(...overlapping.map((r) => r.cssRect.top + r.cssRect.height));

                const minPdfX = Math.min(...overlapping.map((r) => r.pdfBox.x));
                const minPdfY = Math.min(...overlapping.map((r) => r.pdfBox.y));
                const maxPdfRight = Math.max(...overlapping.map((r) => r.pdfBox.x + r.pdfBox.width));
                const maxPdfTop = Math.max(...overlapping.map((r) => r.pdfBox.y + r.pdfBox.height));

                pageEntities.push({
                  instanceId: `${pNum}_${globalCounter}`,
                  pageNum: pNum,
                  type: snip.type || 'FIO',
                  original: snip.original,
                  anonymized: '[ ПДн СКРЫТО ]',
                  cssRect: {
                    left: minCssLeft,
                    top: minCssTop,
                    width: maxCssRight - minCssLeft,
                    height: maxCssBottom - minCssTop
                  },
                  pdfBox: {
                    page: pNum,
                    x: minPdfX,
                    y: minPdfY,
                    width: maxPdfRight - minPdfX,
                    height: maxPdfTop - minPdfY
                  }
                });
              }
              idx = pageStr.indexOf(snip.original, matchEnd);
            }
          }
        }

        // 2. Client-side PII pattern matching
        for (const pat of piiPatterns) {
          pat.regex.lastIndex = 0;
          let match: RegExpExecArray | null;
          while ((match = pat.regex.exec(pageStr)) !== null) {
            const matchedText = match[0].trim();
            if (matchedText.length < 3) continue;

            const matchStart = match.index;
            const matchEnd = match.index + match[0].length;

            // Avoid duplicate overlays if already covered by snippets
            const alreadyMatched = pageEntities.some(
              (e) => e.original.includes(matchedText) || matchedText.includes(e.original)
            );
            if (alreadyMatched) continue;

            const overlapping = itemRanges.filter(
              (r) => r.startChar < matchEnd && r.endChar > matchStart
            );

            if (overlapping.length > 0) {
              globalCounter++;
              const minCssLeft = Math.min(...overlapping.map((r) => r.cssRect.left));
              const minCssTop = Math.min(...overlapping.map((r) => r.cssRect.top));
              const maxCssRight = Math.max(...overlapping.map((r) => r.cssRect.left + r.cssRect.width));
              const maxCssBottom = Math.max(...overlapping.map((r) => r.cssRect.top + r.cssRect.height));

              const minPdfX = Math.min(...overlapping.map((r) => r.pdfBox.x));
              const minPdfY = Math.min(...overlapping.map((r) => r.pdfBox.y));
              const maxPdfRight = Math.max(...overlapping.map((r) => r.pdfBox.x + r.pdfBox.width));
              const maxPdfTop = Math.max(...overlapping.map((r) => r.pdfBox.y + r.pdfBox.height));

              pageEntities.push({
                instanceId: `${pNum}_${globalCounter}`,
                pageNum: pNum,
                type: pat.type,
                original: matchedText,
                anonymized: '[ ПДн СКРЫТО ]',
                cssRect: {
                  left: minCssLeft,
                  top: minCssTop,
                  width: maxCssRight - minCssLeft,
                  height: maxCssBottom - minCssTop
                },
                pdfBox: {
                  page: pNum,
                  x: minPdfX,
                  y: minPdfY,
                  width: maxPdfRight - minPdfX,
                  height: maxPdfTop - minPdfY
                }
              });
            }
          }
        }

        pagesList.push({
          pageNum: pNum,
          width: Math.floor(viewport.width),
          height: Math.floor(viewport.height),
          entities: pageEntities
        });
      }

      pagesData = pagesList;

      // Render canvases on next animation frame once DOM elements mount
      requestAnimationFrame(async () => {
        const dpr = window.devicePixelRatio || 1;
        for (let pNum = 1; pNum <= numPages; pNum++) {
          const canvas = canvasRefs[pNum];
          if (!canvas) continue;

          const page = await pdf.getPage(pNum);
          const viewport = page.getViewport({ scale: targetScale });

          canvas.width = Math.floor(viewport.width * dpr);
          canvas.height = Math.floor(viewport.height * dpr);
          canvas.style.width = `${Math.floor(viewport.width)}px`;
          canvas.style.height = `${Math.floor(viewport.height)}px`;

          const ctx = canvas.getContext('2d');
          if (ctx) {
            ctx.scale(dpr, dpr);
            await page.render({
              canvasContext: ctx,
              viewport
            }).promise;
          }
        }
      });
    } catch (err: any) {
      console.error('Failed to render PDF with PDF.js:', err);
      loadError = err?.message || 'Ошибка загрузки документа PDF';
    } finally {
      isLoading = false;
    }
  }
</script>

{#if fileInfo && fileInfo.fileType === 'pdf'}
  <div
    id="pdf-inspection-box"
    class="bg-white border border-[#E8E7E3] shadow-sm flex flex-col select-text"
  >
    <!-- Compact Header Toolbar (Sticky) -->
    <div
      class="px-5 py-3 border-b border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-[#FAF9F5] sticky top-0 z-40"
    >
      <!-- Title & Document Stats -->
      <div class="flex items-center space-x-2">
        <span class="w-2 h-2 rounded-none bg-emerald-500"></span>
        <span
          class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono"
        >
          Предпросмотр документа (PDF)
        </span>
        <span class="text-xs text-[#78716C] font-mono">
          ▪ {pagesData.length > 0 ? pagesData.length : (fileInfo.pageCount || 1)} стр. ▪ {allDetectedEntities.length} фрагментов ПДн
        </span>
      </div>

      <!-- Controls Right: Before/After Toggle -->
      <div class="flex items-center space-x-3">
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium shadow-2xs">
          <button
            type="button"
            onclick={() => (isBefore = false)}
            class="flex items-center space-x-1.5 px-3 py-1 {!isBefore
              ? 'bg-emerald-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-none {!isBefore ? 'bg-emerald-300' : 'bg-emerald-500'}"></span>
            <span>Обезличенный вид</span>
          </button>
          <button
            type="button"
            onclick={() => (isBefore = true)}
            class="flex items-center space-x-1.5 px-3 py-1 {isBefore
              ? 'bg-amber-700 text-white font-semibold shadow-xs'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            <span class="w-1.5 h-1.5 rounded-none {isBefore ? 'bg-amber-300' : 'bg-amber-500'}"></span>
            <span>Исходный вид</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Continuous Scroll Viewport Container -->
    <div
      id="pdf-sheet-viewport"
      class="p-4 sm:p-8 md:p-10 bg-[#EDEBE6] overflow-y-auto max-h-[760px] custom-scrollbar relative flex flex-col items-center gap-6"
    >
      {#if isLoading}
        <div class="flex flex-col items-center justify-center py-24 space-y-3">
          <div class="w-8 h-8 border-3 border-stone-300 border-t-emerald-600 rounded-full animate-spin"></div>
          <span class="text-xs font-mono text-[#78716C]">Рендеринг страниц через Mozilla PDF.js...</span>
        </div>
      {:else if loadError}
        <div class="text-center py-20 px-6 max-w-md mx-auto bg-white border border-rose-200 p-6 shadow-sm">
          <div class="text-rose-600 font-bold text-sm mb-2">Не удалось отобразить PDF</div>
          <p class="text-xs text-[#78716C] font-mono leading-relaxed">{loadError}</p>
        </div>
      {:else if pagesData.length > 0}
        <!-- Render each page sequentially in continuous scroll -->
        {#each pagesData as page (page.pageNum)}
          <div
            class="pdf-page-sheet relative bg-white shadow-lg border border-[#D5D3CC] transition-shadow hover:shadow-xl"
            style="width: {page.width}px; height: {page.height}px;"
          >
            <!-- Page Number Pill -->
            <div class="absolute top-3 right-3 font-mono text-[10px] text-[#A8A29E] bg-[#FAF9F5] border border-[#E8E7E3] px-2 py-0.5 z-10 pointer-events-none select-none">
              Стр. {page.pageNum} из {pagesData.length}
            </div>

            <!-- Authentic HTML5 Canvas Rendered Pixel-Perfect by PDF.js -->
            <canvas
              bind:this={canvasRefs[page.pageNum]}
              class="block"
            ></canvas>

            <!-- Word-Style Interactive PII Overlay Chips -->
            <div class="absolute inset-0 pointer-events-none">
              {#each page.entities as ent (ent.instanceId)}
                {@const isDisabled = disabledInstanceIds.has(ent.instanceId)}
                {@const markClass = isDisabled
                  ? 'bg-stone-100/90 text-stone-600 border-dashed border-stone-400 hover:bg-stone-200'
                  : !isBefore
                  ? appState.processingMode === 'delete'
                    ? 'bg-neutral-900 text-neutral-100 border-neutral-950 hover:bg-neutral-800'
                    : 'bg-emerald-100/95 text-emerald-950 border-emerald-500 hover:bg-emerald-200 shadow-xs'
                  : 'bg-amber-100/95 text-amber-950 border-amber-500 hover:bg-amber-200 shadow-xs'}

                <button
                  type="button"
                  id="pdf-entity-{ent.instanceId}"
                  onclick={() => toggleInstance(ent.instanceId)}
                  onmouseenter={(e) => handleChipHover(e, ent)}
                  onmouseleave={handleChipLeave}
                  class="absolute pointer-events-auto px-1 py-0.5 rounded-[2px] border font-mono font-medium text-[11px] leading-tight cursor-pointer transition-all duration-150 text-left overflow-hidden text-ellipsis whitespace-nowrap {markClass} hover:scale-[1.02]"
                  style="left: {ent.cssRect.left}px; top: {ent.cssRect.top}px; min-width: {ent.cssRect.width}px; height: {ent.cssRect.height}px;"
                >
                  {#if isDisabled}
                    <span class="line-through decoration-stone-500">{ent.original}</span>
                  {:else if !isBefore}
                    {ent.anonymized}
                  {:else}
                    {ent.original}
                  {/if}
                </button>
              {/each}
            </div>
          </div>
        {/each}
      {:else}
        <div class="text-center py-20 text-xs text-[#78716C] font-mono">
          Документ пуст или не содержит страниц для отображения
        </div>
      {/if}

      <!-- Floating Popover / Tooltip on Hover (Word-Style 1-in-1 with TextPreview) -->
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
                  ? 'bg-stone-800 border border-stone-600 text-stone-400'
                  : 'bg-emerald-950 border border-emerald-600 text-emerald-300'} text-[9px] font-bold"
              >
                {hoveredEntity.isDisabled ? '▫ ОТКЛЮЧЕНО' : '▪ ПДн СКРЫТО'}
              </span>
            </div>
            <span class="text-stone-400 text-[10px]">
              Вымарывание PDF
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
              <span class="text-stone-400 font-mono text-[11px] shrink-0">После сокрытия:</span>
              <span class="font-mono {hoveredEntity.isDisabled ? 'text-stone-400 line-through' : 'text-emerald-300'} text-[11px] break-all text-right font-medium">
                {hoveredEntity.isDisabled ? hoveredEntity.original : hoveredEntity.anonymized}
              </span>
            </div>
          </div>

          <div class="mt-2.5 pt-2 border-t border-stone-800 text-[10px] text-stone-400 font-mono flex items-center justify-between">
            <span>💡 Кликните для {hoveredEntity.isDisabled ? 'включения' : 'отключения'} сокрытия</span>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}
