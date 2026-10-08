<script lang="ts">
  import type { KeyFileInfo } from '../../types/domain';

  interface Props {
    keyInfo: KeyFileInfo | null;
    keyPath: string;
    isInspecting?: boolean;
    onReselect: () => void;
  }

  let { keyInfo, keyPath, isInspecting = false, onReselect }: Props = $props();

  const keyFileName = $derived(keyPath.split('/').pop()?.split('\\').pop() || 'key.depmap');
  const isValid = $derived(!!keyInfo?.valid);

  const badgeText = $derived.by(() => {
    if (isInspecting) return 'Проверка...';
    return isValid ? '✔ Готов' : '.DEPMAP';
  });

  const badgeClass = $derived.by(() => {
    if (isInspecting) {
      return 'text-[10px] px-1.5 py-0.5 font-medium bg-amber-50 text-amber-700 border border-amber-200';
    }
    return isValid
      ? 'text-[10px] bg-emerald-50 text-emerald-700 border border-emerald-200 px-1 py-0.5'
      : 'text-[10px] bg-[#FAF9F5] text-[#78716C] border border-[#E8E7E3] px-1.5 py-0.5';
  });
</script>

<div id="restore-key-info" class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] text-xs font-mono space-y-1">
  <div class="flex items-center justify-between">
    <span class="font-bold text-[#1C1917] truncate max-w-[180px]" id="restore-key-name" title={keyFileName}>
      {keyFileName}
    </span>
    <span id="restore-key-status-badge" class={badgeClass}>
      {badgeText}
    </span>
  </div>

  <div class="text-[11px] text-[#78716C] truncate" id="restore-key-path" title={keyPath}>
    {keyPath}
  </div>

  <div class="flex items-center justify-between pt-1">
    <span class="text-[11px] text-[#57534E]" id="restore-key-meta">
      {#if isValid && keyInfo}
        {keyInfo.recordsCount.toLocaleString('ru-RU')} записей
      {:else}
        Зашифрован
      {/if}
    </span>
    <button
      type="button"
      onclick={onReselect}
      class="text-[11px] text-[#1C1917] hover:underline font-semibold cursor-pointer"
    >
      Сменить ключ
    </button>
  </div>
</div>
