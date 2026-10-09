<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
</script>

<div
  id="mode-selector-bar"
  class="bg-white border border-[#E8E7E3] px-4 py-2.5 flex items-center justify-between shadow-2xs"
>
  <div class="flex items-center gap-3">
    <span class="text-xs font-bold text-[#1C1917] uppercase tracking-wider font-mono">
      Режим обработки:
    </span>
    <div class="inline-flex bg-[#F0EFEB] p-0.5 border border-[#E8E7E3] text-xs font-medium">
      <button
        id="mode-btn-replace"
        type="button"
        onclick={() => appState.setProcessingMode('replace')}
        class={appState.processingMode === 'replace'
          ? 'px-3.5 py-1 bg-[#1C1917] text-white font-semibold shadow-xs cursor-pointer'
          : 'px-3.5 py-1 text-[#78716C] hover:text-[#1C1917] cursor-pointer transition'}
      >
        Замена
      </button>
      <button
        id="mode-btn-delete"
        type="button"
        onclick={() => appState.setProcessingMode('delete')}
        class={appState.processingMode === 'delete'
          ? 'px-3.5 py-1 bg-[#1C1917] text-white font-semibold shadow-xs cursor-pointer'
          : 'px-3.5 py-1 text-[#78716C] hover:text-[#1C1917] cursor-pointer transition'}
      >
        Удаление
      </button>
    </div>
  </div>
</div>
