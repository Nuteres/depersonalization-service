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
  import { GetAppInfo, EventsOn, OnFileDrop } from './lib/api/wails';
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

    const handleDroppedFilePath = (filePath: string, x?: number, y?: number) => {
      if (!filePath) return;

      let isKeyZone = false;
      let isDataZone = false;

      if (typeof x === 'number' && typeof y === 'number') {
        const elem = document.elementFromPoint(x, y);
        isKeyZone = !!(elem?.closest('#restore-key-dropzone') || elem?.closest('#restore-key-info'));
        isDataZone = !!(elem?.closest('#restore-data-dropzone') || elem?.closest('#restore-data-info'));
      }

      if (filePath.toLowerCase().endsWith('.depmap') || isKeyZone) {
        appState.setMode('restore');
        restoreState.loadKeyFile(filePath);
      } else if (isDataZone || appState.currentAppMode === 'restore') {
        restoreState.loadDataFile(filePath);
      } else {
        if (appState.currentAppMode !== 'depersonalize') {
          appState.setMode('depersonalize');
        }
        loadFile(filePath);
      }
    };

    // Wails v2 native file drop handler (provides full absolute OS filesystem paths on Windows & macOS)
    const unsubNativeDrop = OnFileDrop((x: number, y: number, paths: string[]) => {
      if (paths && paths.length > 0 && paths[0]) {
        handleDroppedFilePath(paths[0], x, y);
      }
    });

    const unsubDrop = EventsOn('app:file-dropped', (filePath: string) => {
      if (filePath) {
        handleDroppedFilePath(filePath);
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
      unsubNativeDrop();
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
