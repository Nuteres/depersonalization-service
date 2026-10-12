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

  const TYPE_NAMES: Record<string, string> = {
    FIO: 'ФИО',
    LAST_NAME: 'Фамилия',
    FIRST_NAME: 'Имя',
    PATRONYMIC: 'Отчество',
    SNILS: 'СНИЛС',
    INN: 'ИНН',
    PASSPORT: 'Паспорт',
    PHONE: 'Телефон',
    BANK_CARD: 'Карта',
    CARD: 'Карта',
    DATE: 'Дата',
    BIRTHDAY: 'Дата',
    EMAIL: 'Email',
    ADDRESS: 'Адрес',
    GRADE: 'Грейд',
    EMPLOYEE_ID: 'Табель'
  };

  function getTypeBadge(type: string): string {
    return TYPE_NAMES[type] || type || 'ПДн';
  }

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
          {activeCount} из {totalCount} в обработке
        </span>
      </div>

      <!-- Entity Cards Grid -->
      <div
        id="entity-cards-container"
        class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3"
      >
        {#each entities as ent}
          {@const isDisabled = disabledCols.includes(ent.columnIndex)}
          <div
            class="group relative bg-white border transition-all cursor-pointer select-none p-3.5 flex flex-col justify-between {isDisabled
              ? 'border-dashed border-stone-300 bg-stone-50/60 opacity-60'
              : 'border-[#E8E7E3] hover:border-[#1C1917] shadow-2xs'}"
            onclick={() => toggle(ent.columnIndex)}
            role="button"
            tabindex="0"
            onkeydown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') toggle(ent.columnIndex);
            }}
          >
            <!-- Header: Column/Category label + Single Toggle Switch -->
            <div class="flex items-center justify-between gap-2 mb-2">
              <span class="text-[11px] font-mono font-semibold tracking-wider text-[#A8A29E] uppercase">
                {appState.currentFileInfo?.fileType === 'text' ? 'КАТЕГОРИЯ' : 'КОЛОНКА'} #{ent.columnIndex}
              </span>

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
                  : 'bg-emerald-600'}"
              >
                <span
                  class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out {isDisabled
                    ? 'translate-x-0'
                    : 'translate-x-4'}"
                ></span>
              </button>
            </div>

            <!-- Title: fixed min-height ensures no vertical jumps across columns -->
            <div class="min-h-[1.375rem] flex items-center mb-1.5">
              {#if ent.columnName && ent.columnName.trim()}
                <div
                  class="font-semibold text-sm text-[#1C1917] truncate leading-snug {isDisabled ? 'text-stone-400' : ''}"
                  title={ent.columnName}
                >
                  {ent.columnName}
                </div>
              {:else}
                <div
                  class="text-sm font-normal text-stone-400 italic truncate"
                >
                  (Без названия)
                </div>
              {/if}
            </div>

            <!-- Footer: Type Badge + Description -->
            <div class="flex items-center gap-2 mt-auto pt-1">
              <span
                class="shrink-0 px-1.5 py-0.5 rounded text-[10px] font-mono font-semibold uppercase {isDisabled
                  ? 'bg-stone-200/70 text-stone-500'
                  : 'bg-emerald-50 text-emerald-700 border border-emerald-200/80'}"
              >
                {getTypeBadge(ent.type)}
              </span>
              <span
                class="text-xs text-[#78716C] truncate leading-normal"
              >
                {ent.description}
              </span>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}
