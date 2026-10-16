<script lang="ts">
  import { onMount } from 'svelte';
  import Header from './lib/components/layout/Header.svelte';
  import DepersonalizeView from './lib/components/depersonalize/DepersonalizeView.svelte';
  import RestoreView from './lib/components/restore/RestoreView.svelte';
  import ProgressModal from './lib/components/common/ProgressModal.svelte';
  import SuccessModal from './lib/components/common/SuccessModal.svelte';
  import Toast from './lib/components/common/Toast.svelte';
  import AuditModal from './lib/components/common/AuditModal.svelte';
  import { appState } from './lib/stores/appState.svelte';
  import { restoreState } from './lib/stores/restoreState.svelte';
  import { GetAppInfo, EventsOn, OnFileDrop } from './lib/api/wails';
  import { loadFile } from './lib/services/fileLoader';
  import { zoomState } from './lib/stores/zoomState.svelte';
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

    // Initialize UI zoom from saved settings
    zoomState.init();

    // Mouse wheel zoom (Ctrl + Wheel / Cmd + Wheel)
    const handleWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault();
        if (e.deltaY < 0) {
          zoomState.zoomIn(zoomState.stepWheel);
        } else if (e.deltaY > 0) {
          zoomState.zoomOut(zoomState.stepWheel);
        }
        zoomState.triggerIndicator();
      }
    };
    window.addEventListener('wheel', handleWheel, { passive: false });

    // Keyboard shortcuts for zooming (Ctrl/Cmd + / - / 0)
    const handleKeydown = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.metaKey) {
        if (e.key === '=' || e.key === '+' || e.code === 'Equal' || e.code === 'NumpadAdd') {
          e.preventDefault();
          zoomState.zoomIn(zoomState.stepButtons);
          zoomState.triggerIndicator();
        } else if (e.key === '-' || e.key === '_' || e.code === 'Minus' || e.code === 'NumpadSubtract') {
          e.preventDefault();
          zoomState.zoomOut(zoomState.stepButtons);
          zoomState.triggerIndicator();
        } else if (e.key === '0' || e.code === 'Digit0' || e.code === 'Numpad0') {
          e.preventDefault();
          zoomState.resetZoom();
          zoomState.triggerIndicator();
        }
      }
    };
    window.addEventListener('keydown', handleKeydown);

    return () => {
      unsubProcess();
      unsubRestore();
      unsubDrop();
      unsubNativeDrop();
      window.removeEventListener('dragover', preventDefaults);
      window.removeEventListener('drop', preventDefaults);
      window.removeEventListener('wheel', handleWheel);
      window.removeEventListener('keydown', handleKeydown);
    };
  });
</script>

<div class="min-h-screen flex flex-col p-6 sm:p-10 max-w-6xl mx-auto selection:bg-amber-100">
  <Header />

  <main class="flex-1 flex flex-col pt-5 sm:pt-6">
    {#if appState.currentAppMode === 'depersonalize'}
      <DepersonalizeView />
    {:else if appState.currentAppMode === 'restore'}
      <RestoreView />
    {/if}
  </main>

  <ProgressModal />
  <SuccessModal />
  <AuditModal />
  <Toast />

  {#if zoomState.showIndicator}
    <div
      id="zoom-indicator-overlay"
      class="fixed bottom-8 left-1/2 -translate-x-1/2 z-50 pointer-events-none transition-all duration-200"
    >
      <div class="bg-[#1C1917]/90 text-white font-mono text-xs px-3.5 py-1.5 border border-stone-700 rounded-md shadow-xl flex items-center space-x-2 backdrop-blur-xs animate-surface-enter">
        <span class="text-stone-400">Масштаб:</span>
        <span class="font-bold text-amber-300">{Math.round(zoomState.currentZoom * 100)}%</span>
      </div>
    </div>
  {/if}
</div>
