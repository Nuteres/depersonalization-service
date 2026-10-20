import type {
  AnonymizationStrategy,
  AppInfo,
  FileInfo,
  ProcessResult,
  ProcessingMode,
  RestoreResult
} from '../types/domain';

export type AppMode = 'depersonalize' | 'restore';
export type ToastType = 'info' | 'success' | 'error' | 'warning';

export interface ToastMessage {
  id: number;
  message: string;
  type: ToastType;
}

export interface ProgressState {
  percent: number;
  processed: number;
  total: number;
  title: string;
}

export type SuccessResult = (ProcessResult | RestoreResult) & {
  outputPath?: string;
  keyPath?: string;
  archivePath?: string;
  archivePassword?: string;
  archivePasswordGenerated?: boolean;
};

class AppState {
  currentAppMode = $state<AppMode>('depersonalize');
  processingMode = $state<ProcessingMode>('replace');
  strategy = $state<AnonymizationStrategy>('tokens');
  currentFilePath = $state<string>('');
  currentFileInfo = $state<FileInfo | null>(null);
  isFileLoading = $state<boolean>(false);
  loadingFileName = $state<string>('');
  loadingFileSize = $state<string>('');
  loadingFileStatus = $state<string>('');
  currentSalt = $state<string>('152fz-secure-salt');
  isProcessing = $state<boolean>(false);
  progress = $state<ProgressState>({
    percent: 0,
    processed: 0,
    total: 0,
    title: ''
  });
  showProgressModal = $state<boolean>(false);
  successResult = $state<SuccessResult | null>(null);
  showSuccessModal = $state<boolean>(false);
  showAuditModal = $state<boolean>(false);
  toast = $state<ToastMessage | null>(null);
  appInfo = $state<AppInfo | null>(null);
  pdfRedactionBoxes = $state<import('../types/domain').PDFRedactionBox[]>([]);
  pdfRedactTerms = $state<string[]>([]);

  private _toastTimer: ReturnType<typeof setTimeout> | null = null;
  private _toastCounter = 0;

  setMode(mode: AppMode): void {
    if (this.currentAppMode !== mode) {
      this.currentAppMode = mode;
      this.successResult = null;
      this.showSuccessModal = false;
    }
  }

  setStrategy(strategy: AnonymizationStrategy): void {
    this.strategy = strategy;
    if (strategy === 'delete') {
      this.processingMode = 'delete';
    } else {
      this.processingMode = 'replace';
    }
  }

  setProcessingMode(mode: ProcessingMode): void {
    this.processingMode = mode;
    if (mode === 'delete') {
      this.strategy = 'delete';
    } else if (this.strategy === 'delete') {
      this.strategy = 'tokens';
    }
  }

  setFilePath(path: string): void {
    this.currentFilePath = path;
  }

  setFileInfo(info: FileInfo | null): void {
    this.currentFileInfo = info;
  }

  startFileLoading(fileName: string, status?: string, sizeStr?: string): void {
    this.isFileLoading = true;
    this.loadingFileName = fileName;
    this.loadingFileSize = sizeStr || '';
    this.loadingFileStatus = status || 'Чтение документа и первичный анализ ПДн (152-ФЗ)...';
  }

  updateFileLoadingStatus(status: string): void {
    this.loadingFileStatus = status;
  }

  finishFileLoading(): void {
    this.isFileLoading = false;
    this.loadingFileName = '';
    this.loadingFileSize = '';
    this.loadingFileStatus = '';
  }

  setSalt(salt: string): void {
    this.currentSalt = salt;
  }

  setAppInfo(info: AppInfo | null): void {
    this.appInfo = info;
  }

  showToast(message: string, type: ToastType = 'info'): void {
    if (this._toastTimer) {
      clearTimeout(this._toastTimer);
      this._toastTimer = null;
    }
    const id = ++this._toastCounter;
    this.toast = { id, message, type };

    if (typeof window !== 'undefined') {
      this._toastTimer = setTimeout(() => {
        if (this.toast?.id === id) {
          this.toast = null;
        }
      }, 4000);
    }
  }

  hideToast(): void {
    if (this._toastTimer) {
      clearTimeout(this._toastTimer);
      this._toastTimer = null;
    }
    this.toast = null;
  }

  startProgress(title: string): void {
    this.isProcessing = true;
    this.showProgressModal = true;
    this.successResult = null;
    this.showSuccessModal = false;
    this.progress = {
      percent: 0,
      processed: 0,
      total: 0,
      title
    };
  }

  updateProgress(percent: number, processed: number, total: number): void {
    const clampedPct = Math.min(100, Math.max(0, percent));
    this.progress = {
      ...this.progress,
      percent: clampedPct,
      processed,
      total
    };
  }

  finishProgress(): void {
    this.isProcessing = false;
    this.showProgressModal = false;
  }

  showSuccess(result: SuccessResult): void {
    this.isProcessing = false;
    this.showProgressModal = false;
    this.successResult = result;
    this.showSuccessModal = true;
  }

  closeSuccessModal(): void {
    this.showSuccessModal = false;
    this.successResult = null;
  }

  openAuditModal(): void {
    this.showAuditModal = true;
  }

  closeAuditModal(): void {
    this.showAuditModal = false;
  }

  reset(): void {
    this.currentFilePath = '';
    this.currentFileInfo = null;
    this.isFileLoading = false;
    this.loadingFileName = '';
    this.loadingFileSize = '';
    this.loadingFileStatus = '';
    this.processingMode = 'replace';
    this.strategy = 'tokens';
    this.isProcessing = false;
    this.showProgressModal = false;
    this.successResult = null;
    this.showSuccessModal = false;
    this.progress = {
      percent: 0,
      processed: 0,
      total: 0,
      title: ''
    };
  }
}

export const appState = new AppState();
