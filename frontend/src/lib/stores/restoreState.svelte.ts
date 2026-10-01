import type { FileInfo, KeyFileInfo } from '../types/domain';
import { InspectFile, InspectKeyFile, SelectFile, SelectKeyFile } from '../api/wails';
import { appState } from './appState.svelte';

const SUPPORTED_EXTS = ['xlsx', 'csv', 'tsv', 'xlsm', 'xlsb', 'xls', 'ods', 'docx', 'odt'];

class RestoreState {
  dataPath = $state<string>('');
  dataInfo = $state<FileInfo | null>(null);
  keyPath = $state<string>('');
  keyInfo = $state<KeyFileInfo | null>(null);
  password = $state<string>('');
  isInspectingKey = $state<boolean>(false);
  isLoadingData = $state<boolean>(false);

  private _pwdDebounce: ReturnType<typeof setTimeout> | null = null;

  setDataFile(path: string, info: FileInfo | null): void {
    this.dataPath = path;
    this.dataInfo = info;
  }

  setKeyFile(path: string): void {
    this.keyPath = path;
    this.keyInfo = null;
  }

  setPassword(pwd: string): void {
    this.password = pwd;
    if (this._pwdDebounce) {
      clearTimeout(this._pwdDebounce);
    }
    this._pwdDebounce = setTimeout(() => {
      if (this.keyPath) {
        this.inspectKey();
      }
    }, 300);
  }

  async loadDataFile(path: string): Promise<boolean> {
    if (!path) return false;
    const ext = path.split('.').pop()?.toLowerCase() || '';
    if (!SUPPORTED_EXTS.includes(ext)) {
      appState.showToast(
        'Пожалуйста, выберите поддерживаемый файл данных (.xlsx, .ods, .csv, .docx, .odt)',
        'warning'
      );
      return false;
    }

    this.dataPath = path;
    this.isLoadingData = true;
    try {
      const info = await InspectFile(path);
      this.dataInfo = info;
      return true;
    } catch (err) {
      console.warn('Could not inspect restore data file:', err);
      this.dataInfo = null;
      return true;
    } finally {
      this.isLoadingData = false;
    }
  }

  async selectDataFile(): Promise<boolean> {
    try {
      const selected = await SelectFile();
      if (selected) {
        return await this.loadDataFile(selected);
      }
      return false;
    } catch (err) {
      console.error('Failed to select restore data file:', err);
      appState.showToast(`Ошибка при выборе файла данных: ${err}`, 'error');
      return false;
    }
  }

  async loadKeyFile(path: string): Promise<boolean> {
    if (!path) return false;
    this.keyPath = path;
    return await this.inspectKey();
  }

  async selectKeyFile(): Promise<boolean> {
    try {
      const selected = await SelectKeyFile();
      if (selected) {
        return await this.loadKeyFile(selected);
      }
      return false;
    } catch (err) {
      console.error('Failed to select key file:', err);
      appState.showToast(`Ошибка при выборе файла ключа: ${err}`, 'error');
      return false;
    }
  }

  async inspectKey(): Promise<boolean> {
    if (!this.keyPath) return false;
    this.isInspectingKey = true;
    try {
      const kInfo = await InspectKeyFile(this.keyPath, this.password);
      this.keyInfo = kInfo;
      return !!kInfo?.valid;
    } catch (err) {
      this.keyInfo = {
        path: this.keyPath,
        originalFileName: '',
        createdAt: '',
        recordsCount: 0,
        valid: false,
        error: String(err)
      };
      return false;
    } finally {
      this.isInspectingKey = false;
    }
  }

  reset(): void {
    if (this._pwdDebounce) {
      clearTimeout(this._pwdDebounce);
      this._pwdDebounce = null;
    }
    this.dataPath = '';
    this.dataInfo = null;
    this.keyPath = '';
    this.keyInfo = null;
    this.password = '';
    this.isInspectingKey = false;
    this.isLoadingData = false;
  }
}

export const restoreState = new RestoreState();
