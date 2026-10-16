<script lang="ts">
  import type { KeyFileInfo } from '../../types/domain';

  interface Props {
    keyInfo: KeyFileInfo | null;
    keyPath: string;
    isInspecting?: boolean;
    onReselect: () => void;
    onClear?: () => void;
  }

  let { keyInfo, keyPath, isInspecting = false, onReselect, onClear }: Props = $props();

  const keyFileName = $derived(keyPath.split('/').pop()?.split('\\').pop() || 'key.depmap');
  const isValid = $derived(!!keyInfo?.valid);

  const badgeText = $derived.by(() => {
    if (isInspecting) return 'Проверка...';
    return isValid ? 'Готов' : '.depmap';
  });

  const badgeClass = $derived.by(() => {
    if (isInspecting) {
      return 'text-[10px] px-1.5 py-0.5 font-medium bg-amber-50 text-amber-700 border border-amber-200 rounded';
    }
    return isValid
      ? 'text-[10px] bg-emerald-50 text-emerald-700 border border-emerald-200 px-1 py-0.5 rounded'
      : 'text-[10px] bg-[#FAF9F5] text-[#78716C] border border-[#E8E7E3] px-1.5 py-0.5 rounded font-mono';
  });
</script>

<div id="restore-key-info" class="flex-1 flex flex-col justify-between p-4 bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg text-xs font-sans hover-sheen">
  <div>
    <div class="flex items-start justify-between gap-2 mb-2">
      <div class="flex items-center gap-2 min-w-0">
        <svg class="w-4 h-4 text-emerald-700 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
        </svg>
        <span class="font-bold text-[#1C1917] truncate font-sans" id="restore-key-name" title={keyFileName}>
          {keyFileName}
        </span>
      </div>
      <div class="flex items-center gap-1.5 shrink-0">
        <span id="restore-key-status-badge" class="{badgeClass} font-mono">
          {badgeText}
        </span>
        {#if onClear}
          <button
            type="button"
            id="btn-remove-key-file"
            onclick={onClear}
            class="text-[#A8A29E] hover:text-rose-600 hover:bg-rose-50 p-1 rounded transition cursor-pointer"
            title="Удалить прикрепленный ключ"
            aria-label="Удалить прикрепленный ключ"
          >
            <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        {/if}
      </div>
    </div>

    <div class="text-[11px] text-[#78716C] break-all line-clamp-2 mb-2 font-sans" id="restore-key-path" title={keyPath}>
      {keyPath}
    </div>
  </div>

  <div class="pt-3 border-t border-[#E8E7E3] flex items-center justify-between gap-3 font-sans">
    <span class="text-[11px] text-[#57534E] truncate min-w-0" id="restore-key-meta">
      {#if isValid && keyInfo}
        {keyInfo.recordsCount.toLocaleString('ru-RU')} записей
      {:else}
        Зашифрован
      {/if}
    </span>
    <div class="flex items-center gap-2 shrink-0">
      <button
        type="button"
        id="btn-reselect-key"
        onclick={onReselect}
        class="text-[11px] text-[#1C1917] hover:underline font-semibold cursor-pointer whitespace-nowrap"
      >
        Сменить
      </button>
      {#if onClear}
        <span class="text-[#D6D3D1]">|</span>
        <button
          type="button"
          id="btn-clear-key-text"
          onclick={onClear}
          class="text-[11px] text-[#78716C] hover:text-rose-600 hover:underline cursor-pointer whitespace-nowrap"
        >
          Удалить
        </button>
      {/if}
    </div>
  </div>
</div>
