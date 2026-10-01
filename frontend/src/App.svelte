<script lang="ts">
  let appTitle = $state('Деперсонализация ПД (152-ФЗ)');
</script>

<main class="min-h-screen flex flex-col bg-slate-900 text-slate-100">
  <header class="border-b border-slate-800 bg-slate-900/80 backdrop-blur px-6 py-4 flex items-center justify-between sticky top-0 z-30">
    <div class="flex items-center gap-3">
      <div class="w-9 h-9 rounded-lg bg-blue-600/20 border border-blue-500/30 flex items-center justify-center text-blue-400">
        <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
        </svg>
      </div>
      <div>
        <h1 class="text-base font-semibold text-white tracking-tight">{appTitle}</h1>
        <p class="text-xs text-slate-400">Модуль маскирования и обратимого восстановления данных</p>
      </div>
    </div>
  </header>

  <div class="flex-1 max-w-7xl w-full mx-auto p-6">
    <div class="rounded-xl border border-slate-800 bg-slate-900/50 p-8 text-center text-slate-400">
      <p>Инициализация интерфейса Svelte 5...</p>
    </div>
  </div>
</main>
