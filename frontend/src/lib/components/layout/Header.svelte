<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

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
        <span class="text-base font-bold tracking-tight text-[#1C1917]">Деперсонализация ПД (152-ФЗ)</span>
        <span id="app-env-badge" class="text-[10px] font-mono font-medium px-2 py-0.5 rounded-full bg-[#E8E7E3] text-[#44403C]">
          {envBadge}
        </span>
      </div>
    </div>
  </div>

  <!-- Mode Switcher Tabs (Unified Right-Aligned Global Control) -->
  <div class="flex items-center bg-[#F0EFEB] p-1 text-xs font-medium border border-[#E8E7E3] self-start sm:self-center">
    <button
      id="mode-tab-depersonalize"
      type="button"
      onclick={() => appState.setMode('depersonalize')}
      class={appState.currentAppMode === 'depersonalize'
        ? 'px-3.5 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition flex items-center space-x-1.5 cursor-pointer'
        : 'px-3.5 py-1.5 text-[#78716C] hover:text-[#1C1917] transition flex items-center space-x-1.5 cursor-pointer'}
    >
      <span class="w-2 h-2 rounded-full {appState.currentAppMode === 'depersonalize' ? 'bg-emerald-600' : 'bg-stone-400'}"></span>
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
      <span class="w-2 h-2 rounded-full {appState.currentAppMode === 'restore' ? 'bg-emerald-600' : 'bg-stone-400'}"></span>
      <span>Восстановление данных</span>
    </button>
  </div>
</header>
