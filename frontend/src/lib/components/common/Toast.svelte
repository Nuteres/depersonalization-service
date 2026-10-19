<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  const typeStyles = {
    success: {
      bg: 'bg-[#ECFDF5] border-[#A7F3D0] text-[#065F46]'
    },
    error: {
      bg: 'bg-[#FEF2F2] border-[#FECACA] text-[#991B1B]'
    },
    warning: {
      bg: 'bg-[#FFFBEB] border-[#FDE68A] text-[#92400E]'
    },
    info: {
      bg: 'bg-[#EFF6FF] border-[#BFDBFE] text-[#1E40AF]'
    }
  };

  const currentStyle = $derived.by(() => {
    if (!appState.toast) return typeStyles.info;
    return typeStyles[appState.toast.type] || typeStyles.info;
  });
</script>

{#if appState.toast}
  <div
    class="fixed bottom-6 right-6 z-50 max-w-md w-full shadow-xl transition-all"
    role="alert"
  >
    <div class="flex items-center gap-3 p-3.5 border rounded-lg text-xs font-mono animate-surface-enter {currentStyle.bg}">
      <span class="shrink-0 flex items-center justify-center w-4 h-4">
        {#if appState.toast.type === 'success'}
          <svg class="w-4 h-4 text-emerald-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M5 13l4 4L19 7" />
          </svg>
        {:else if appState.toast.type === 'error'}
          <svg class="w-4 h-4 text-rose-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M6 18L18 6M6 6l12 12" />
          </svg>
        {:else if appState.toast.type === 'warning'}
          <svg class="w-4 h-4 text-amber-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
        {:else}
          <svg class="w-4 h-4 text-blue-600 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        {/if}
      </span>
      <div class="flex-1 font-sans text-xs break-words">
        {appState.toast.message}
      </div>
      <button
        type="button"
        onclick={() => appState.hideToast()}
        class="text-stone-400 hover:text-stone-700 transition cursor-pointer shrink-0 flex items-center justify-center w-5 h-5 rounded hover:bg-black/5 pressable"
        aria-label="Закрыть уведомление"
      >
        <svg class="w-3.5 h-3.5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
      </button>
    </div>
  </div>
{/if}
