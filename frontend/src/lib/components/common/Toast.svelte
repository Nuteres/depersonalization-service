<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  const typeStyles = {
    success: {
      bg: 'bg-[#ECFDF5] border-[#A7F3D0] text-[#065F46]',
      icon: '✔'
    },
    error: {
      bg: 'bg-[#FEF2F2] border-[#FECACA] text-[#991B1B]',
      icon: '✕'
    },
    warning: {
      bg: 'bg-[#FFFBEB] border-[#FDE68A] text-[#92400E]',
      icon: '⚠'
    },
    info: {
      bg: 'bg-[#EFF6FF] border-[#BFDBFE] text-[#1E40AF]',
      icon: 'ℹ'
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
    <div class="flex items-start gap-3 p-3.5 border text-xs font-mono {currentStyle.bg}">
      <span class="font-bold shrink-0">{currentStyle.icon}</span>
      <div class="flex-1 font-sans text-xs break-words">
        {appState.toast.message}
      </div>
      <button
        type="button"
        onclick={() => appState.hideToast()}
        class="text-stone-400 hover:text-stone-700 transition cursor-pointer shrink-0 font-mono text-sm leading-none ml-1"
        aria-label="Закрыть уведомление"
      >
        ✕
      </button>
    </div>
  </div>
{/if}
