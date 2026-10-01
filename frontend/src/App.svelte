<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/layout/Header.svelte';
  import ProgressModal from './lib/components/common/ProgressModal.svelte';
  import SuccessModal from './lib/components/common/SuccessModal.svelte';
  import Toast from './lib/components/common/Toast.svelte';
  import { appState } from './lib/stores/appState.svelte';
  import { GetAppInfo, EventsOn } from './lib/api/wails';
  import type { ProgressEventData } from './lib/types/domain';

  onMount(() => {
    // Load application runtime info
    GetAppInfo()
      .then((info) => {
        appState.setAppInfo(info);
      })
      .catch((err) => {
        console.warn('Failed to load AppInfo:', err);
      });

    // Subscribe to streaming progress events from backend
    const unsubProcess = EventsOn('process-progress', (data: ProgressEventData) => {
      if (data) {
        appState.updateProgress(data.percent, data.processed, data.total);
      }
    });

    const unsubRestore = EventsOn('restore-progress', (data: ProgressEventData) => {
      if (data) {
        appState.updateProgress(data.percent, data.processed, data.total);
      }
    });

    return () => {
      unsubProcess();
      unsubRestore();
    };
  });
</script>

<div class="min-h-screen flex flex-col p-6 sm:p-10 max-w-6xl mx-auto selection:bg-amber-100">
  <Header />

  <main class="flex-1 flex flex-col pt-8">
    {#if appState.currentAppMode === 'depersonalize'}
      <div id="depersonalization-view" class="flex-1 flex flex-col gap-6">
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-8 text-center text-[#78716C]">
          <p class="text-sm font-medium">Режим деперсонализации данных (152-ФЗ)</p>
          <p class="text-xs text-[#A8A29E] mt-1">Вид деперсонализации будет подключен в следующем шаге.</p>
        </div>
      </div>
    {:else}
      <div id="restoration-view" class="flex-1 flex flex-col gap-6">
        <div class="border border-[#E8E7E3] bg-[#FAF9F5] p-8 text-center text-[#78716C]">
          <p class="text-sm font-medium">Режим обратного восстановления данных</p>
          <p class="text-xs text-[#A8A29E] mt-1">Вид восстановления будет подключен в следующем шаге.</p>
        </div>
      </div>
    {/if}
  </main>

  <ProgressModal />
  <SuccessModal />
  <Toast />
</div>
