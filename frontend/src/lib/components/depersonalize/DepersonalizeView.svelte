<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import HeroBanner from './HeroBanner.svelte';
  import SheetTabs from './SheetTabs.svelte';
  import EntityGrid from './EntityGrid.svelte';
  import TableView from './TableView.svelte';
  import TextPreview from './TextPreview.svelte';
  import PdfPreview from './PdfPreview.svelte';
  import SaveDialogModal from './SaveDialogModal.svelte';

  let isSaveModalOpen = $state(false);

  function openSaveModal() {
    isSaveModalOpen = true;
  }

  function closeSaveModal() {
    isSaveModalOpen = false;
  }
</script>

<div id="depersonalization-view" class="flex-1 flex flex-col gap-6 pb-12">
  <HeroBanner onOpenSaveModal={openSaveModal} />

  {#if !appState.isFileLoading && appState.currentFileInfo}
    {#if appState.currentFileInfo.sheets && appState.currentFileInfo.sheets.length > 1}
      <SheetTabs />
    {/if}

    <EntityGrid />

    {#if appState.currentFileInfo.fileType === 'table'}
      <TableView />
    {:else if appState.currentFileInfo.fileType === 'text' || appState.currentFileInfo.fileType === 'docx' || appState.currentFileInfo.fileType === 'odt'}
      <TextPreview />
    {:else if appState.currentFileInfo.fileType === 'pdf'}
      <PdfPreview />
    {/if}
  {/if}

  <SaveDialogModal isOpen={isSaveModalOpen} onClose={closeSaveModal} />
</div>
