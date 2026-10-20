<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { selectAndLoadFile, loadFile, clearCurrentFile } from '../../services/fileLoader';

  interface Props {
    onOpenSaveModal: () => void;
  }

  const { onOpenSaveModal }: Props = $props();

  let isDraggingOver = $state(false);

  const fileExtBadge = $derived.by(() => {
    const ext = appState.currentFileInfo?.extension || 'xlsx';
    return `.${ext.replace('.', '').toLowerCase()}`;
  });

  async function handleSelectFile() {
    await selectAndLoadFile();
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    isDraggingOver = true;
  }

  function handleDragEnter(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    isDraggingOver = true;
  }

  function handleDragLeave(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    isDraggingOver = false;
  }

  async function handleDrop(e: DragEvent) {
    e.preventDefault();
    isDraggingOver = false;

    if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      const file = e.dataTransfer.files[0];
      const filePath = (file as unknown as { path?: string })?.path;
      // Only load directly if filePath is an absolute path (e.g. on macOS / Electron)
      // Otherwise, Wails OnFileDrop will receive the full OS path from the native drop handler.
      if (file && filePath && (filePath.includes('/') || filePath.includes('\\'))) {
        await loadFile(filePath, file.size);
      }
    }
  }

  const supportedFormats = [
    '.xlsx',
    '.xlsb',
    '.ods',
    '.xods',
    '.xls',
    '.csv',
    '.docx',
    '.odt',
    '.txt',
    '.pdf'
  ];
</script>

{#if appState.isFileLoading}
  <!-- FILE LOADING & INSPECTING STATE -->
  <div
    id="file-loading-card"
    class="border border-[#E8E7E3] bg-[#FAF9F5] p-8 text-center flex flex-col items-center justify-center gap-3 select-none rounded-lg shadow-2xs hover-sheen"
  >
    <div class="relative flex items-center justify-center w-8 h-8">
      <div class="w-8 h-8 border-2 border-[#E8E7E3] border-t-[#1C1917] rounded-full animate-spin"></div>
    </div>

    <!-- File Name & Primary Status -->
    <div class="flex flex-col items-center gap-1 max-w-md">
      <div class="font-semibold text-sm text-[#1C1917] flex items-center justify-center gap-2 flex-wrap">
        <span>Анализ файла:</span>
        <span class="font-mono text-xs text-[#1C1917] bg-[#F0EFEB] px-2 py-0.5 border border-[#E8E7E3] rounded truncate max-w-[280px]">
          {appState.loadingFileName}
        </span>
        {#if appState.loadingFileSize}
          <span class="font-mono text-[11px] text-[#78716C]">
            {appState.loadingFileSize}
          </span>
        {/if}
      </div>
      <div class="text-xs text-[#78716C]">
        Поиск персональных данных...
      </div>
    </div>
  </div>
{:else if !appState.currentFileInfo}
  <!-- NO FILE SELECTED: Large dashed drop zone -->
  <div
    id="empty-state-dropzone"
    class="border border-dashed {isDraggingOver
      ? 'border-[#1C1917] bg-[#F5F5F4]'
      : 'border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-white'} p-8 text-center transition-all cursor-pointer flex flex-col items-center justify-center gap-2.5 rounded-lg shadow-2xs hover-sheen select-none"
    onclick={handleSelectFile}
    ondragover={handleDragOver}
    ondragenter={handleDragEnter}
    ondragleave={handleDragLeave}
    ondrop={handleDrop}
    role="button"
    tabindex="0"
    onkeydown={(e) => {
      if (e.key === 'Enter' || e.key === ' ') handleSelectFile();
    }}
  >
    <svg class="w-8 h-8 text-stone-400 mb-1" fill="none" viewBox="0 0 24 24" stroke="currentColor">
      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"/>
    </svg>
    <div class="font-semibold text-sm text-[#1C1917]">
      {isDraggingOver ? 'Отпустите файл' : 'Выберите или перетащите файл'}
    </div>

    <!-- Format pill badges -->
    <div class="flex flex-wrap items-center justify-center gap-1.5 max-w-lg">
      {#each supportedFormats as format}
        <span
          class="text-[11px] font-mono font-medium px-2 py-0.5 rounded bg-white border border-[#E8E7E3] text-[#44403C] shadow-2xs"
        >
          {format}
        </span>
      {/each}
    </div>
  </div>
{:else}
  <!-- FILE SELECTED: File Info Card & Hero Action Bar -->
  <div
    id="hero-file-card"
    class="bg-white border border-[#E8E7E3] rounded-lg px-5 py-3 flex flex-col sm:flex-row sm:items-center justify-between gap-4 shadow-2xs hover-sheen transition-all select-none"
    ondragover={handleDragOver}
    ondragenter={handleDragEnter}
    ondragleave={handleDragLeave}
    ondrop={handleDrop}
    role="region"
    aria-label="Загруженный файл"
  >
    <!-- Left: File Icon Badge + File Name + Extension Pill -->
    <div class="flex items-center space-x-3 min-w-0 flex-1">
      <div class="w-7 h-7 bg-emerald-50 rounded-md border border-emerald-200/80 flex items-center justify-center text-emerald-700 shrink-0 select-none">
        <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
        </svg>
      </div>
      <div class="flex items-center space-x-2 min-w-0">
        <h2
          id="file-name-title"
          class="text-sm font-bold tracking-tight text-[#1C1917] truncate leading-snug"
          title={appState.currentFileInfo.name}
        >
          {appState.currentFileInfo.name}
        </h2>
        <span class="text-[11px] font-mono font-medium px-2 py-0.5 rounded bg-[#F0EFEB] border border-[#E8E7E3] text-[#44403C] shrink-0">
          {fileExtBadge}
        </span>
      </div>
    </div>

    <!-- Right: Action Toolbar in Hero -->
    <div class="flex items-center gap-2 self-start sm:self-center shrink-0">
      <button
        id="btn-change-file-hero"
        type="button"
        onclick={handleSelectFile}
        class="h-7 px-3 bg-white hover:bg-[#FAF9F5] text-[#1C1917] border border-[#E8E7E3] rounded-md text-xs font-semibold shadow-2xs pressable flex items-center justify-center cursor-pointer"
        title="Выбрать другой файл"
      >
        <span>Сменить</span>
      </button>

      <button
        id="btn-close-file-hero"
        type="button"
        onclick={clearCurrentFile}
        class="h-7 px-3 bg-white hover:bg-rose-50 hover:text-rose-600 text-[#78716C] border border-[#E8E7E3] hover:border-rose-300 rounded-md text-xs font-semibold shadow-2xs pressable flex items-center justify-center cursor-pointer"
        title="Удалить прикрепленный файл"
      >
        <span>Удалить</span>
      </button>

      <button
        id="btn-save"
        type="button"
        onclick={onOpenSaveModal}
        disabled={appState.isProcessing}
        class="h-7 px-3.5 bg-[#1C1917] hover:bg-[#292524] disabled:bg-stone-200 disabled:text-stone-400 disabled:cursor-not-allowed text-white text-xs font-semibold rounded-md shadow-xs pressable flex items-center justify-center cursor-pointer"
        title="Сохранить и обезличить файл"
      >
        <span>Сохранить</span>
      </button>
    </div>
  </div>
{/if}
