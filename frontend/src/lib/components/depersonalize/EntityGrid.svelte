<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { tableState } from '../../stores/tableState.svelte';
  import type { EntityInfo } from '../../types/domain';

  const activeSheetName = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (info?.sheets && info.sheets.length > 0) {
      return info.sheets[tableState.activeSheetIndex]?.name || '';
    }
    return '';
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
    tableState.disabledColumnsBySheet[activeSheetName] || []
  );

  const totalCount = $derived(entities.length);
  const activeCount = $derived(
    entities.filter((ent) => !disabledCols.includes(ent.columnIndex)).length
  );

  function toggle(colIndex: number) {
    tableState.toggleColumn(activeSheetName, colIndex);
  }
</script>

{#if appState.currentFileInfo}
  <div>
    {#if entities.length === 0}
      <!-- Zero PII Entities Found -->
      <div class="bg-white border border-[#E8E7E3] p-5 text-xs text-[#78716C] flex items-center space-x-3 shadow-2xs">
        <span class="text-base text-emerald-600 font-bold shrink-0">✔</span>
        <div>
          <div class="font-semibold text-[#1C1917] text-sm">В файле не обнаружено персональных данных</div>
          <div class="text-xs text-[#78716C] mt-0.5 leading-relaxed">
            В таблице нет колонок ФИО, СНИЛС, ИНН, Паспортов, Телефонов, Почты или Дат. Все исходные колонки будут сохранены без изменений.
          </div>
        </div>
      </div>
    {:else}
      <!-- Header bar for entities -->
      <div id="entity-cards-header" class="flex items-center justify-between mb-3">
        <div class="flex items-center space-x-2">
          <span class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono">Обнаруженные категории ПДн</span>
          <span class="hidden sm:inline text-[11px] text-[#78716C]">(нажмите на карточку, чтобы исключить из обработки)</span>
        </div>
        <span id="entity-count-badge" class="text-xs font-mono text-[#78716C]">
          {activeCount} из {totalCount} активных
        </span>
      </div>

      <!-- Entity Cards Grid -->
      <div
        id="entity-cards-container"
        class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3.5"
      >
        {#each entities as ent}
          {@const isDisabled = disabledCols.includes(ent.columnIndex)}
          <div
            class="group relative bg-white border {isDisabled
              ? 'border-dashed border-stone-300 opacity-80'
              : 'border-[#E8E7E3] hover:border-[#1C1917]'} p-4 flex flex-col justify-between shadow-xs transition-all cursor-pointer select-none"
            onclick={() => toggle(ent.columnIndex)}
            role="button"
            tabindex="0"
            onkeydown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') toggle(ent.columnIndex);
            }}
          >
            <div>
              <div class="flex items-center justify-between text-xs text-[#78716C] mb-2 font-mono">
                <span>
                  {appState.currentFileInfo?.fileType === 'text' ? 'КАТЕГОРИЯ ПДн' : 'КОЛОНКА'} #{ent.columnIndex}
                </span>

                <button
                  type="button"
                  onclick={(e) => {
                    e.stopPropagation();
                    toggle(ent.columnIndex);
                  }}
                  class="focus:outline-none flex items-center space-x-1.5 px-2 py-0.5 rounded text-[11px] font-mono font-medium transition cursor-pointer {isDisabled
                    ? 'bg-stone-100 text-stone-500 hover:bg-stone-200'
                    : appState.processingMode === 'delete'
                      ? 'bg-rose-50 text-rose-700 border border-rose-200 hover:bg-rose-100'
                      : 'bg-emerald-50 text-emerald-700 border border-emerald-200 hover:bg-emerald-100'}"
                >
                  <span
                    class="w-2 h-2 rounded-full {isDisabled ? 'bg-stone-400' : 'bg-emerald-500'}"
                  ></span>
                  <span>{isDisabled ? 'Отключено' : '● Токенизация'}</span>
                </button>
              </div>

              <div class="flex items-start justify-between gap-2">
                <div class="font-bold text-sm text-[#1C1917] truncate" title={ent.columnName}>
                  {ent.columnName}
                </div>

                <!-- Notion-like Toggle Switch -->
                <button
                  type="button"
                  onclick={(e) => {
                    e.stopPropagation();
                    toggle(ent.columnIndex);
                  }}
                  title={isDisabled ? 'Включить в обработку' : 'Исключить из обработки'}
                  class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none {isDisabled
                    ? 'bg-stone-300'
                    : appState.processingMode === 'delete'
                      ? 'bg-rose-600'
                      : 'bg-emerald-600'}"
                >
                  <span
                    class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out {isDisabled
                      ? 'translate-x-0'
                      : 'translate-x-4'}"
                  ></span>
                </button>
              </div>

              <div class="text-xs text-[#78716C] mt-1 line-clamp-2" title={ent.description}>
                {ent.description}
              </div>
            </div>

            <div
              class="mt-4 pt-3 border-t border-[#F0EFEB] text-xs font-mono -mx-4 -mb-4 p-2.5 transition-colors {isDisabled
                ? 'text-[#78716C] bg-[#F5F5F4]'
                : 'text-[#065F46] bg-[#ECFDF5]'}"
            >
              {isDisabled ? '○ Исходные данные без изменений' : `➜ ${ent.action}`}
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}
