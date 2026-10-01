<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import type { DocxTextSnippet } from '../../types/domain';

  let isBefore = $state(false);

  const snippets = $derived<DocxTextSnippet[]>(
    appState.currentFileInfo?.textSnippets || []
  );

  const typeLabels: Record<string, string> = {
    FIO: 'ФИО',
    SNILS: 'СНИЛС',
    PASSPORT: 'Паспорт РФ',
    PHONE: 'Телефон',
    INN: 'ИНН',
    BANK_CARD: 'Банковская карта',
    DATE: 'Дата',
    EMAIL: 'Email'
  };

  function getEntityTypeName(type: string): string {
    return typeLabels[type] || type || 'ПДн';
  }
</script>

{#if appState.currentFileInfo && (appState.currentFileInfo.fileType === 'text' || appState.currentFileInfo.fileType === 'docx' || appState.currentFileInfo.fileType === 'odt')}
  <div id="text-inspection-box" class="bg-white border border-[#E8E7E3] shadow-sm flex flex-col">
    <!-- Header Toolbar -->
    <div
      class="p-4 border-b border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-[#FAF9F5]"
    >
      <div class="flex items-center space-x-2">
        <span class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono">
          Предпросмотр фрагментов текста
        </span>
        <span id="text-snippet-count-label" class="text-xs text-[#78716C] font-mono">
          ({snippets.length} фрагментов)
        </span>
      </div>

      <div class="flex items-center space-x-3">
        <!-- Toggle: Before vs After -->
        <div class="inline-flex border border-[#E8E7E3] bg-white p-0.5 text-xs font-medium">
          <button
            type="button"
            onclick={() => (isBefore = false)}
            class="px-3 py-1 {!isBefore
              ? 'bg-[#1C1917] text-white font-semibold'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            Очищенный текст
          </button>
          <button
            type="button"
            onclick={() => (isBefore = true)}
            class="px-3 py-1 {isBefore
              ? 'bg-[#1C1917] text-white font-semibold'
              : 'text-[#78716C] hover:text-[#1C1917]'} transition cursor-pointer"
          >
            Исходный с ПДн
          </button>
        </div>
      </div>
    </div>

    <!-- Snippet cards container -->
    <div
      id="text-snippets-container"
      class="p-5 space-y-3.5 max-h-[500px] overflow-y-auto custom-scrollbar bg-[#FAF9F5]/40 divide-y divide-[#F0EFEB]"
    >
      {#if snippets.length === 0}
        <div class="text-center py-8 text-xs text-[#78716C] font-mono">
          В документе не обнаружено фрагментов персональных данных для замены
        </div>
      {:else}
        {#each snippets as snip, idx}
          {@const typeName = getEntityTypeName(snip.type)}
          <div class="pt-3.5 first:pt-0">
            <div class="flex items-center justify-between mb-1.5 font-mono text-[11px]">
              <div class="flex items-center space-x-2">
                <span
                  class="px-2 py-0.5 bg-[#FAF9F5] border border-[#E8E7E3] text-[#1C1917] font-semibold text-[10px] uppercase"
                >
                  {typeName}
                </span>
                <span class="text-[#78716C]">Фрагмент #{idx + 1}</span>
              </div>
              <span class="text-[10px] text-[#A8A29E]">
                {isBefore ? 'Исходный вид' : 'Обезличенный вид'}
              </span>
            </div>

            <div class="bg-white border border-[#E8E7E3] p-3 text-xs leading-relaxed text-[#1C1917] font-sans">
              {#if !isBefore}
                <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                  <div class="space-y-1 flex-1 min-w-0">
                    <div class="text-[11px] text-[#78716C] font-mono">До деперсонализации:</div>
                    <div
                      class="p-2 bg-rose-50/50 border border-rose-200/60 text-rose-900 rounded-none font-mono text-[11px] break-words"
                    >
                      {snip.original}
                    </div>
                  </div>
                  <div class="text-center text-[#78716C] font-bold text-xs shrink-0 self-center px-1">
                    ➔
                  </div>
                  <div class="space-y-1 flex-1 min-w-0">
                    <div class="text-[11px] text-[#78716C] font-mono">После деперсонализации:</div>
                    <div
                      class="p-2 bg-emerald-50/60 border border-emerald-200 text-emerald-950 rounded-none font-mono text-[11px] break-words"
                    >
                      {snip.anonymized}
                    </div>
                  </div>
                </div>
              {:else}
                <div class="space-y-1">
                  <div class="text-[11px] text-[#78716C] font-mono">Исходный фрагмент:</div>
                  <div
                    class="p-2 bg-rose-50/50 border border-rose-200/60 text-rose-900 rounded-none font-mono text-[11px] break-words"
                  >
                    {snip.original}
                  </div>
                </div>
              {/if}
            </div>
          </div>
        {/each}
      {/if}
    </div>
  </div>
{/if}
