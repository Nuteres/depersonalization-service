<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { zoomState } from '../../stores/zoomState.svelte';

  const envBadge = $derived.by(() => {
    if (!appState.appInfo) return 'v1.13.0';
    const v = appState.appInfo.version || '1.13.0';
    return v.startsWith('v') ? v : `v${v}`;
  });
</script>

<header class="liquid-glass rounded-lg px-5 py-3 flex flex-col sm:flex-row sm:items-center justify-between gap-4 transition-all">
  <div class="flex items-center space-x-3">
    <div class="w-7 h-7 bg-[#1C1917] rounded-md flex items-center justify-center text-white font-mono text-xs font-bold shadow-xs select-none">
      152
    </div>
    <div class="flex items-center space-x-2">
      <span class="text-sm font-bold tracking-tight text-[#1C1917]">Деперсонализация</span>
      <span id="app-env-badge" class="text-[11px] font-mono font-medium px-2 py-0.5 rounded bg-[#F0EFEB] border border-[#E8E7E3] text-[#44403C]">
        {envBadge}
      </span>
    </div>
  </div>

  <!-- Controls: Tabs + Audit Log Button + Zoom -->
  <div class="flex items-center gap-2 self-start sm:self-center flex-wrap">
    <div class="flex items-center bg-[#F0EFEB] p-0.5 rounded-md text-xs font-medium border border-[#E8E7E3]">
      <button
        id="mode-tab-depersonalize"
        type="button"
        onclick={() => appState.setMode('depersonalize')}
        class={appState.currentAppMode === 'depersonalize'
          ? 'px-3 py-1 bg-white text-[#1C1917] rounded shadow-xs font-semibold pressable flex items-center space-x-1.5 cursor-pointer'
          : 'px-3 py-1 text-[#78716C] hover:text-[#1C1917] rounded pressable flex items-center space-x-1.5 cursor-pointer'}
      >
        <span class="w-1.5 h-1.5 rounded-full {appState.currentAppMode === 'depersonalize' ? 'bg-emerald-500' : 'bg-stone-300'}"></span>
        <span>Деперсонализация</span>
      </button>
      <button
        id="mode-tab-restore"
        type="button"
        onclick={() => appState.setMode('restore')}
        class={appState.currentAppMode === 'restore'
          ? 'px-3 py-1 bg-white text-[#1C1917] rounded shadow-xs font-semibold pressable flex items-center space-x-1.5 cursor-pointer'
          : 'px-3 py-1 text-[#78716C] hover:text-[#1C1917] rounded pressable flex items-center space-x-1.5 cursor-pointer'}
      >
        <span class="w-1.5 h-1.5 rounded-full {appState.currentAppMode === 'restore' ? 'bg-emerald-500' : 'bg-stone-300'}"></span>
        <span>Восстановление</span>
      </button>
    </div>

    <button
      id="btn-open-audit"
      type="button"
      onclick={() => appState.openAuditModal()}
      class="px-3 py-1 bg-white hover:bg-[#FAF9F5] text-[#1C1917] border border-[#E8E7E3] rounded-md text-xs font-medium shadow-2xs pressable flex items-center gap-1.5 cursor-pointer"
      title="Журнал операций"
    >
      <svg class="w-3.5 h-3.5 text-stone-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <span>Журнал</span>
    </button>

    <!-- Zoom Controls -->
    <div class="flex items-center bg-white rounded-md text-xs font-medium border border-[#E8E7E3] overflow-hidden shadow-2xs" id="zoom-controls">
      <button
        id="btn-zoom-out"
        type="button"
        onclick={() => zoomState.zoomOut()}
        disabled={zoomState.currentZoom <= zoomState.minZoom}
        class="px-2 py-1 hover:bg-[#F0EFEB] text-[#78716C] hover:text-[#1C1917] pressable cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
        title="Уменьшить масштаб"
      >
        –
      </button>
      <button
        id="btn-zoom-reset"
        type="button"
        onclick={() => zoomState.resetZoom()}
        class="px-2 py-1 font-mono text-[11px] font-semibold text-[#1C1917] border-x border-[#E8E7E3] hover:bg-[#F0EFEB] pressable cursor-pointer min-w-[44px] text-center"
        title="Сбросить масштаб"
      >
        {Math.round(zoomState.currentZoom * 100)}%
      </button>
      <button
        id="btn-zoom-in"
        type="button"
        onclick={() => zoomState.zoomIn()}
        disabled={zoomState.currentZoom >= zoomState.maxZoom}
        class="px-2 py-1 hover:bg-[#F0EFEB] text-[#78716C] hover:text-[#1C1917] pressable cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
        title="Увеличить масштаб"
      >
        +
      </button>
    </div>
  </div>
</header>
