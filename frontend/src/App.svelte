<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/layout/Header.svelte';
  import DepersonalizeView from './lib/components/depersonalize/DepersonalizeView.svelte';
  import RestoreView from './lib/components/restore/RestoreView.svelte';
  import ProgressModal from './lib/components/common/ProgressModal.svelte';
  import SuccessModal from './lib/components/common/SuccessModal.svelte';
  import Toast from './lib/components/common/Toast.svelte';
  import { appState } from './lib/stores/appState.svelte';
  import { restoreState } from './lib/stores/restoreState.svelte';
  import { GetAppInfo, EventsOn } from './lib/api/wails';
  import { loadFile } from './lib/services/fileLoader';
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

    const unsubDrop = EventsOn('app:file-dropped', (filePath: string) => {
      if (filePath) {
        if (filePath.toLowerCase().endsWith('.depmap')) {
          appState.setMode('restore');
          restoreState.loadKeyFile(filePath);
        } else if (appState.currentAppMode === 'depersonalize') {
          loadFile(filePath);
        } else if (appState.currentAppMode === 'restore') {
          restoreState.loadDataFile(filePath);
        }
      }
    });

    // Prevent default browser file open when dropping outside dropzones
    const preventDefaults = (e: DragEvent) => {
      e.preventDefault();
    };
    window.addEventListener('dragover', preventDefaults);
    window.addEventListener('drop', preventDefaults);

    return () => {
      unsubProcess();
      unsubRestore();
      unsubDrop();
      window.removeEventListener('dragover', preventDefaults);
      window.removeEventListener('drop', preventDefaults);
    };
  });
</script>

<div class="min-h-screen flex flex-col p-6 sm:p-10 max-w-6xl mx-auto selection:bg-amber-100">
  <Header />

  <main class="flex-1 flex flex-col pt-8">
    {#if appState.currentAppMode === 'depersonalize'}
      <DepersonalizeView />
    {:else if appState.currentAppMode === 'restore'}
      <RestoreView />
    {/if}
  </main>

  <ProgressModal />
  <SuccessModal />
  <Toast />
</div>
