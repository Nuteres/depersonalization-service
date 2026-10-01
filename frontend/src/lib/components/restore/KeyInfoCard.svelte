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
    return isValid ? '✔ Валиден' : '⚠ Ошибка / Пароль';
  });

  const badgeClass = $derived.by(() => {
    if (isInspecting) {
      return 'text-[10px] px-1.5 py-0.5 font-medium bg-amber-50 text-amber-700 border border-amber-200';
    }
    return isValid
      ? 'text-[10px] px-1.5 py-0.5 font-medium bg-emerald-50 text-emerald-700 border border-emerald-200'
      : 'text-[10px] px-1.5 py-0.5 font-medium bg-rose-50 text-rose-700 border border-rose-200';
  });
</script>

<div id="restore-key-info" class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] text-xs font-mono space-y-1.5">
  <div class="flex items-center justify-between">
    <span class="font-bold text-[#1C1917] truncate max-w-[170px]" id="restore-key-name" title={keyFileName}>
      {keyFileName}
    </span>
    <span id="restore-key-status-badge" class={badgeClass}>
      {badgeText}
    </span>
  </div>

  <div class="text-[11px] text-[#78716C] truncate" id="restore-key-path" title={keyPath}>
    {keyPath}
  </div>

  {#if keyInfo && keyInfo.valid}
    <div class="text-[11px] text-[#57534E]" id="restore-key-meta">
      Исходный: {keyInfo.originalFileName || 'данные'} • Записей: {keyInfo.recordsCount.toLocaleString('ru-RU')} • {keyInfo.createdAt}
    </div>
  {:else}
    <div id="restore-key-error" class="text-[11px] text-rose-700 bg-rose-50 p-1.5 border border-rose-200">
      {keyInfo?.error || 'Неверный формат ключа или требуется пароль расшифровки'}
    </div>
  {/if}

  <div class="pt-1 text-right">
    <button
      type="button"
      onclick={onReselect}
      class="text-[11px] text-[#1C1917] hover:underline font-semibold cursor-pointer"
    >
      Сменить ключ...
    </button>
  </div>
</div>
