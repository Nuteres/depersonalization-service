<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { selectAndLoadFile, loadFile } from '../../services/fileLoader';

  interface Props {
    onOpenSaveModal: () => void;
  }

  const { onOpenSaveModal }: Props = $props();

  let isDraggingOver = $state(false);

  function formatFileSize(bytes: number): string {
    if (!bytes || bytes <= 0) return '';
    if (bytes < 1024) return `${bytes} Б`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} КБ`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
  }

  const unitsLabel = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (!info) return '';
    if (info.fileType === 'pdf') {
      return `${info.pageCount} стр.`;
    }
    if (info.fileType === 'text') {
      return `${info.paragraphCount || info.rowCount} абз.`;
    }
    if (info.sheets && info.sheets.length > 1) {
      const totalR = info.sheets.reduce((acc, s) => acc + (s.rowCount || 0), 0);
      return `${info.sheets.length} листов (${totalR.toLocaleString('ru-RU')} строк)`;
    }
    return `${(info.rowCount || 0).toLocaleString('ru-RU')} строк`;
  });

  const fileSizeLabel = $derived.by(() => {
    const size = appState.currentFileInfo?.size;
    return size ? formatFileSize(size) : '';
  });

  const formatLabel = $derived.by(() => {
    const ext = appState.currentFileInfo?.extension || '';
    return ext.toUpperCase().replace('.', '');
  });

  const fileExtBadge = $derived.by(() => {
    const ext = appState.currentFileInfo?.extension || 'xlsx';
    return `.${ext.replace('.', '').toLowerCase()}`;
  });

  const descLabel = $derived.by(() => {
    const info = appState.currentFileInfo;
    if (!info) return '';
    const entityCount = info.entities ? info.entities.length : 0;
    if (info.fileType === 'text') {
      return `Документ содержит ${info.paragraphCount || info.rowCount} абзацев. Найдено ${entityCount} категорий ПДн. Форматирование, шрифты и стили документа сохраняются без изменений.`;
    }
    if (info.sheets && info.sheets.length > 1) {
      const totalEntities = info.sheets.reduce(
        (acc, s) => acc + (s.entities ? s.entities.length : 0),
        0
      );
      return `Книга Excel содержит ${info.sheets.length} листа. Всего найдено ${totalEntities} категорий ПДн. Переключайтесь между вкладками листов для настройки колонок каждого листа.`;
    }
    if (info.fileType === 'pdf') {
      return 'Чувствительные персональные данные вырезаются физически и закрываются плашками.';
    }
    return `Обнаружено категорий ПДн: ${entityCount}. Чувствительные поля подготовлены к безопасной деперсонализации по 152-ФЗ.`;
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
      if (filePath && (filePath.includes('/') || filePath.includes('\\'))) {
        await loadFile(filePath);
      }
    }
  }

  const supportedFormats = [
    '.xlsx',
    '.xlsb',
    '.ods',
    '.xls',
    '.csv',
    '.docx',
    '.odt',
    '.pdf'
  ];
</script>

{#if !appState.currentFileInfo}
  <!-- NO FILE SELECTED: Large dashed drop zone -->
  <div
    id="empty-state-dropzone"
    class="border-2 border-dashed {isDraggingOver
      ? 'border-[#1C1917] bg-[#F5F5F4] scale-[1.005]'
      : 'border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-white'} p-8 sm:p-10 text-center transition-all cursor-pointer flex flex-col items-center justify-center gap-3 shadow-2xs select-none"
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
    <span class="text-4xl">📁</span>
    <div class="font-bold text-base text-[#1C1917]">
      {isDraggingOver ? 'Отпустите файл для загрузки' : 'Выберите файл или перетащите его сюда'}
    </div>
    <div class="text-xs text-[#78716C] max-w-md leading-relaxed">
      Поддерживаются таблицы Excel (.xlsx, .xlsb, .xls), OpenDocument (.ods, .odt), Word (.docx), CSV (.csv) и документы PDF (.pdf). Все данные обрабатываются локально без выхода в сеть.
    </div>

    <!-- Format pill badges -->
    <div class="flex flex-wrap items-center justify-center gap-1.5 mt-1 max-w-lg">
      {#each supportedFormats as format}
        <span
          class="text-[11px] font-mono font-medium px-2 py-0.5 rounded bg-white border border-[#E8E7E3] text-[#44403C] shadow-2xs"
        >
          {format}
        </span>
      {/each}
    </div>

    <button
      type="button"
      onclick={(e) => {
        e.stopPropagation();
        handleSelectFile();
      }}
      class="mt-2 px-5 py-2.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs active:scale-[0.99] transition cursor-pointer"
    >
      Выбрать файл
    </button>
  </div>
{:else}
  <!-- FILE SELECTED: File Info Card & Hero Action Bar -->
  <div
    class="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-2"
    ondragover={handleDragOver}
    ondragenter={handleDragEnter}
    ondragleave={handleDragLeave}
    ondrop={handleDrop}
    role="region"
    aria-label="Загруженный файл"
  >
    <div class="flex-1 min-w-0">
      <div id="file-meta-bar" class="flex flex-wrap items-center gap-2 text-xs font-mono text-[#78716C] uppercase mb-1.5">
        <span>Файл загружен</span>
        <span>•</span>
        <span>{unitsLabel}</span>
        {#if fileSizeLabel}
          <span>•</span>
          <span>{fileSizeLabel}</span>
        {/if}
        {#if formatLabel}
          <span>•</span>
          <span>{formatLabel}</span>
        {/if}
      </div>

      <h2
        id="file-name-title"
        class="text-xl sm:text-2xl font-bold tracking-tight text-[#1C1917] flex items-start gap-2 break-words"
      >
        <span class="text-xl shrink-0 mt-0.5 select-none">📄</span>
        <span class="break-words" title={appState.currentFileInfo.name}>{appState.currentFileInfo.name}</span>
      </h2>

      <p id="file-desc-text" class="text-sm text-[#78716C] mt-1 max-w-2xl leading-relaxed">
        {descLabel}
      </p>
    </div>

    <!-- Action Toolbar in Hero -->
    <div class="flex flex-wrap items-center gap-3 self-start md:self-end shrink-0">
      <button
        id="btn-change-file-hero"
        type="button"
        onclick={handleSelectFile}
        class="px-3.5 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold shadow-xs transition flex items-center space-x-1.5 active:scale-[0.99] cursor-pointer"
        title="Выбрать другой файл"
      >
        <span>📁</span>
        <span>Сменить файл...</span>
      </button>

      <button
        id="btn-save"
        type="button"
        onclick={onOpenSaveModal}
        class="bg-[#1C1917] hover:bg-[#292524] text-white text-xs font-semibold px-5 py-2 transition active:scale-[0.99] flex items-center justify-center space-x-2 shadow-sm whitespace-nowrap cursor-pointer"
      >
        <span>💾</span>
        <span>Сохранить и обезличить</span>
        <span class="font-mono text-[11px] text-[#A8A29E]">({fileExtBadge})</span>
      </button>
    </div>
  </div>
{/if}
