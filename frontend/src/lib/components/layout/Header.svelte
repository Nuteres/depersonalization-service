<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { zoomState } from '../../stores/zoomState.svelte';

  const envBadge = $derived.by(() => {
    if (!appState.appInfo) return 'v1.0 ▪ Загрузка...';
    const { version, os, arch } = appState.appInfo;
    return arch ? `${version} ▪ ${os} (${arch})` : `${version} ▪ ${os}`;
  });
</script>

<header class="flex flex-col sm:flex-row sm:items-center justify-between pb-6 border-b border-[#E8E7E3] gap-4">
  <div class="flex items-center space-x-3.5">
    <div class="w-9 h-9 bg-[#1C1917] rounded-none flex items-center justify-center text-white font-mono text-sm font-bold shadow-sm">
      152
    </div>
    <div>
      <div class="flex items-center space-x-2">
        <span class="text-base font-bold tracking-tight text-[#1C1917]">Деперсонализация ПД</span>
        <span id="app-env-badge" class="text-[10px] font-mono font-medium px-2 py-0.5 rounded-none bg-[#E8E7E3] text-[#44403C]">
          {envBadge}
        </span>
      </div>
    </div>
  </div>

  <!-- Controls: Tabs + Audit Log Button -->
  <div class="flex items-center gap-2 self-start sm:self-center">
    <div class="flex items-center bg-[#F0EFEB] p-1 text-xs font-medium border border-[#E8E7E3]">
      <button
        id="mode-tab-depersonalize"
        type="button"
        onclick={() => appState.setMode('depersonalize')}
        class={appState.currentAppMode === 'depersonalize'
          ? 'px-3.5 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition flex items-center space-x-1.5 cursor-pointer'
          : 'px-3.5 py-1.5 text-[#78716C] hover:text-[#1C1917] transition flex items-center space-x-1.5 cursor-pointer'}
      >
        <span class="w-2 h-2 rounded-none {appState.currentAppMode === 'depersonalize' ? 'bg-emerald-600' : 'bg-stone-400'}"></span>
        <span>Деперсонализация ПД</span>
      </button>
      <button
        id="mode-tab-restore"
        type="button"
        onclick={() => appState.setMode('restore')}
        class={appState.currentAppMode === 'restore'
          ? 'px-3.5 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition flex items-center space-x-1.5 cursor-pointer'
          : 'px-3.5 py-1.5 text-[#78716C] hover:text-[#1C1917] transition flex items-center space-x-1.5 cursor-pointer'}
      >
        <span class="w-2 h-2 rounded-none {appState.currentAppMode === 'restore' ? 'bg-emerald-600' : 'bg-stone-400'}"></span>
        <span>Восстановление данных</span>
      </button>
    </div>

    <button
      id="btn-open-audit"
      type="button"
      onclick={() => appState.openAuditModal()}
      class="px-3 py-2 bg-[#F0EFEB] hover:bg-white text-[#1C1917] hover:shadow-xs border border-[#E8E7E3] text-xs font-medium transition cursor-pointer flex items-center gap-1.5"
      title="Журнал операций"
    >
      <span class="text-xs">📜</span>
      <span>Журнал</span>
    </button>

    <!-- Zoom Controls -->
    <div class="flex items-center bg-[#F0EFEB] text-xs font-medium border border-[#E8E7E3]" id="zoom-controls">
      <button
        id="btn-zoom-out"
        type="button"
        onclick={() => zoomState.zoomOut()}
        disabled={zoomState.currentZoom <= zoomState.minZoom}
        class="px-2.5 py-1.5 hover:bg-white text-[#78716C] hover:text-[#1C1917] transition cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
        title="Уменьшить масштаб (Ctrl + -)"
      >
        –
      </button>
      <button
        id="btn-zoom-reset"
        type="button"
        onclick={() => zoomState.resetZoom()}
        class="px-2 py-1.5 font-mono text-[11px] font-semibold text-[#1C1917] hover:bg-white transition cursor-pointer min-w-[48px] text-center"
        title="Сбросить масштаб к 100% (Ctrl + 0)"
      >
        {Math.round(zoomState.currentZoom * 100)}%
      </button>
      <button
        id="btn-zoom-in"
        type="button"
        onclick={() => zoomState.zoomIn()}
        disabled={zoomState.currentZoom >= zoomState.maxZoom}
        class="px-2.5 py-1.5 hover:bg-white text-[#78716C] hover:text-[#1C1917] transition cursor-pointer disabled:opacity-40 disabled:cursor-not-allowed font-bold"
        title="Увеличить масштаб (Ctrl + +)"
      >
        +
      </button>
    </div>
  </div>
</header>
