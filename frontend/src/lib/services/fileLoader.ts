import { InspectFile, SelectFile } from '../api/wails';
import { appState } from '../stores/appState.svelte';
import { tableState } from '../stores/tableState.svelte';

/**
 * Loads and inspects a file, updating appState and tableState accordingly.
 */
export async function loadFile(path: string): Promise<boolean> {
  if (!path) return false;

  try {
    const info = await InspectFile(path);
    if (!info) {
      appState.showToast('Не удалось получить информацию о файле', 'error');
      return false;
    }

    appState.setFilePath(path);
    appState.setFileInfo(info);

    // Reset table state for new file
    tableState.reset();

    // Initialize rows for table mode
    if (info.fileType === 'table') {
      if (info.sheets && info.sheets.length > 0 && info.sheets[0]) {
        tableState.setActiveSheet(0);
        tableState.setPreviewRows(info.sheets[0].previewRows || info.previewRows || []);
      } else {
        tableState.setPreviewRows(info.previewRows || []);
      }
    }

    return true;
  } catch (err) {
    console.error('Error loading file:', err);
    appState.showToast(`Ошибка открытия файла: ${err}`, 'error');
    return false;
  }
}

/**
 * Opens native file chooser dialog and loads the selected file.
 */
export async function selectAndLoadFile(): Promise<boolean> {
  try {
    const selected = await SelectFile();
    if (selected) {
      return await loadFile(selected);
    }
    return false;
  } catch (err) {
    console.error('Error selecting file:', err);
    appState.showToast(`Ошибка диалога выбора файла: ${err}`, 'error');
    return false;
  }
}

/**
 * Resets current file state and table state, returning to empty dropzone.
 */
export function clearCurrentFile(): void {
  appState.reset();
  tableState.reset();
}

