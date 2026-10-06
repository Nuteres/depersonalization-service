import type {
  AppInfo,
  FileInfo,
  KeyFileInfo,
  PreviewRow,
  ProcessResult,
  RestoreResult
} from '../types/domain';

type AnyFunction = (...args: any[]) => void;

interface WailsAppBinding {
  GetAppInfo?: () => Promise<AppInfo>;
  InspectFile?: (path: string) => Promise<FileInfo>;
  GetTableRows?: (path: string, offset: number, limit: number, salt: string, disabledCols: number[]) => Promise<PreviewRow[]>;
  GetSheetRows?: (path: string, sheetName: string, offset: number, limit: number, salt: string, disabledCols: number[]) => Promise<PreviewRow[]>;
  ProcessFile?: (path: string, salt: string) => Promise<ProcessResult>;
  ProcessFileTo?: (path: string, outPath: string, salt: string, disabledCols: number[], createKey: boolean, keyPassword: string, keyPath: string) => Promise<ProcessResult>;
  ProcessFileWithSheets?: (
    path: string,
    outPath: string,
    salt: string,
    disabledCols: number[],
    disabledBySheet: Record<string, number[]>,
    createKey: boolean,
    keyPassword: string,
    keyPath: string
  ) => Promise<ProcessResult>;
  CancelCurrentOperation?: () => Promise<void>;
  SelectFile?: () => Promise<string>;
  SaveFileDialog?: (defaultName: string) => Promise<string>;
  SelectKeyFile?: () => Promise<string>;
  SaveKeyFileDialog?: (defaultName: string) => Promise<string>;
  InspectKeyFile?: (keyPath: string, password: string) => Promise<KeyFileInfo>;
  RestoreFileTo?: (inputPath: string, keyPath: string, password: string, destPath: string) => Promise<RestoreResult>;
  OpenContainingFolder?: (targetPath: string) => Promise<void>;
}

interface WailsRuntimeBinding {
  EventsOn?: (eventName: string, callback: AnyFunction) => void;
  EventsOff?: (eventName: string, ...additional: any[]) => void;
  EventsEmit?: (eventName: string, ...args: any[]) => void;
  OnFileDrop?: (callback: (x: number, y: number, paths: string[]) => void, printStack?: boolean) => void;
  OnFileDropOff?: () => void;
}

declare global {
  interface Window {
    go?: {
      desktop?: {
        DesktopApp?: WailsAppBinding;
      };
      main?: {
        App?: WailsAppBinding;
      };
    };
    runtime?: WailsRuntimeBinding;
  }
}

function getApp(): WailsAppBinding | undefined {
  if (typeof window === 'undefined') return undefined;
  const w = window as any;
  if (w.go?.desktop?.DesktopApp) return w.go.desktop.DesktopApp;
  if (w.go?.desktop?.desktopApp) return w.go.desktop.desktopApp;
  if (w.go?.main?.App) return w.go.main.App;
  if (w.go?.main?.app) return w.go.main.app;

  // Fallback: search for any bound object with InspectFile or SelectFile
  if (w.go && typeof w.go === 'object') {
    for (const pkg of Object.values(w.go) as any[]) {
      if (pkg && typeof pkg === 'object') {
        for (const target of Object.values(pkg) as any[]) {
          if (target && (typeof target.SelectFile === 'function' || typeof target.InspectFile === 'function')) {
            return target as WailsAppBinding;
          }
        }
      }
    }
  }
  return undefined;
}

function getRuntime(): WailsRuntimeBinding | undefined {
  if (typeof window === 'undefined') return undefined;
  return window.runtime;
}

// In-memory pub/sub fallback for dev mode when window.runtime is not available
const devEventListeners = new Map<string, Set<AnyFunction>>();

export async function GetAppInfo(): Promise<AppInfo> {
  const app = getApp();
  if (app?.GetAppInfo) {
    return await app.GetAppInfo();
  }
  return {
    version: 'v1.0-dev',
    os: 'Browser (Dev)',
    arch: 'wasm'
  };
}

export async function InspectFile(path: string): Promise<FileInfo> {
  const app = getApp();
  if (app?.InspectFile) {
    return await app.InspectFile(path);
  }
  const fileName = path.split('/').pop() || path.split('\\').pop() || 'sample.csv';
  return {
    path,
    name: fileName,
    extension: '.csv',
    fileType: 'table',
    format: 'csv',
    size: 1024,
    rowCount: 2,
    pageCount: 1,
    headers: ['ФИО', 'СНИЛС', 'Телефон'],
    entities: [
      { columnIndex: 1, columnName: 'ФИО', type: 'fio', description: 'ФИО', action: 'anonymize' },
      { columnIndex: 2, columnName: 'СНИЛС', type: 'snils', description: 'СНИЛС', action: 'anonymize' },
      { columnIndex: 3, columnName: 'Телефон', type: 'phone', description: 'Телефон', action: 'anonymize' }
    ],
    previewRows: [
      {
        index: 1,
        before: ['Иванов Иван Иванович', '112-233-445 95', '+7 (916) 123-45-67'],
        after: ['Смирнов Алексей Петрович', '443-112-998 12', '+7 (916) 999-88-77']
      },
      {
        index: 2,
        before: ['Петров Петр Петрович', '223-344-556 06', '+7 (903) 765-43-21'],
        after: ['Кузнецов Сергей Дмитриевич', '554-223-119 23', '+7 (903) 111-22-33']
      }
    ]
  };
}

export async function GetTableRows(
  path: string,
  offset: number,
  limit: number,
  salt: string,
  disabledCols: number[]
): Promise<PreviewRow[]> {
  const app = getApp();
  if (app?.GetTableRows) {
    return await app.GetTableRows(path, offset, limit, salt, disabledCols);
  }
  if (app?.GetSheetRows) {
    return await app.GetSheetRows(path, '', offset, limit, salt, disabledCols);
  }
  return [];
}

export async function GetSheetRows(
  path: string,
  sheetName: string,
  offset: number,
  limit: number,
  salt: string,
  disabledCols: number[]
): Promise<PreviewRow[]> {
  const app = getApp();
  if (app?.GetSheetRows) {
    return await app.GetSheetRows(path, sheetName, offset, limit, salt, disabledCols);
  }
  if (app?.GetTableRows) {
    return await app.GetTableRows(path, offset, limit, salt, disabledCols);
  }
  return [];
}

export async function ProcessFile(path: string, salt: string): Promise<ProcessResult> {
  const app = getApp();
  if (app?.ProcessFile) {
    return await app.ProcessFile(path, salt);
  }
  return {
    success: true,
    inputPath: path,
    outputPath: `${path}.anonymized`,
    rowsCount: 2,
    entitiesCount: 6,
    durationMs: 45,
    message: 'Файл успешно обезличен (Dev Mode)'
  };
}

export async function ProcessFileTo(
  path: string,
  outPath: string,
  salt: string,
  disabledCols: number[],
  createKey: boolean,
  keyPassword: string,
  keyPath: string
): Promise<ProcessResult> {
  const app = getApp();
  if (app?.ProcessFileTo) {
    return await app.ProcessFileTo(path, outPath, salt, disabledCols, createKey, keyPassword, keyPath);
  }
  const defaultKeyPath = keyPath || `${outPath || path}.depmap`;
  return {
    success: true,
    inputPath: path,
    outputPath: outPath || `${path}.anonymized`,
    keyPath: createKey ? defaultKeyPath : undefined,
    rowsCount: 2,
    entitiesCount: 6,
    durationMs: 50,
    message: 'Файл успешно обезличен (Dev Mode)'
  };
}

export async function ProcessFileWithSheets(
  path: string,
  outPath: string,
  salt: string,
  disabledCols: number[],
  disabledBySheet: Record<string, number[]>,
  createKey: boolean,
  keyPassword: string,
  keyPath: string
): Promise<ProcessResult> {
  const app = getApp();
  if (app?.ProcessFileWithSheets) {
    return await app.ProcessFileWithSheets(
      path,
      outPath,
      salt,
      disabledCols,
      disabledBySheet,
      createKey,
      keyPassword,
      keyPath
    );
  }
  if (app?.ProcessFileTo) {
    return await app.ProcessFileTo(path, outPath, salt, disabledCols, createKey, keyPassword, keyPath);
  }
  const defaultKeyPath = keyPath || `${outPath || path}.depmap`;
  return {
    success: true,
    inputPath: path,
    outputPath: outPath || `${path}.anonymized`,
    keyPath: createKey ? defaultKeyPath : undefined,
    rowsCount: 2,
    entitiesCount: 6,
    durationMs: 55,
    message: 'Файл успешно обезличен (Dev Mode)'
  };
}

export async function CancelCurrentOperation(): Promise<void> {
  const app = getApp();
  if (app?.CancelCurrentOperation) {
    await app.CancelCurrentOperation();
  }
}

export async function SelectFile(): Promise<string> {
  const app = getApp();
  if (app?.SelectFile) {
    return await app.SelectFile();
  }
  // Browser fallback when running outside desktop webview
  return new Promise<string>((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.xlsx,.xlsb,.ods,.xls,.csv,.tsv,.docx,.odt,.pdf';
    input.style.display = 'none';
    input.onchange = () => {
      const file = input.files?.[0];
      if (file) {
        const path = (file as any).path || file.name;
        resolve(path);
      } else {
        resolve('');
      }
      if (input.parentNode) {
        input.parentNode.removeChild(input);
      }
    };
    input.oncancel = () => {
      resolve('');
      if (input.parentNode) {
        input.parentNode.removeChild(input);
      }
    };
    document.body.appendChild(input);
    input.click();
  });
}

export async function SaveFileDialog(defaultName: string): Promise<string> {
  const app = getApp();
  if (app?.SaveFileDialog) {
    return await app.SaveFileDialog(defaultName);
  }
  return `/dev/mock/${defaultName}`;
}

export async function SelectKeyFile(): Promise<string> {
  const app = getApp();
  if (app?.SelectKeyFile) {
    return await app.SelectKeyFile();
  }
  return new Promise<string>((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.depmap';
    input.style.display = 'none';
    input.onchange = () => {
      const file = input.files?.[0];
      if (file) {
        const path = (file as any).path || file.name;
        resolve(path);
      } else {
        resolve('');
      }
      if (input.parentNode) {
        input.parentNode.removeChild(input);
      }
    };
    input.oncancel = () => {
      resolve('');
      if (input.parentNode) {
        input.parentNode.removeChild(input);
      }
    };
    document.body.appendChild(input);
    input.click();
  });
}

export async function SaveKeyFileDialog(defaultName: string): Promise<string> {
  const app = getApp();
  if (app?.SaveKeyFileDialog) {
    return await app.SaveKeyFileDialog(defaultName);
  }
  return `/dev/mock/${defaultName}`;
}

export async function InspectKeyFile(keyPath: string, password: string): Promise<KeyFileInfo> {
  const app = getApp();
  if (app?.InspectKeyFile) {
    return await app.InspectKeyFile(keyPath, password);
  }
  return {
    path: keyPath,
    originalFileName: 'clients.csv',
    createdAt: new Date().toISOString(),
    recordsCount: 4,
    valid: true
  };
}

export async function RestoreFileTo(
  inputPath: string,
  keyPath: string,
  password: string,
  destPath: string
): Promise<RestoreResult> {
  const app = getApp();
  if (app?.RestoreFileTo) {
    return await app.RestoreFileTo(inputPath, keyPath, password, destPath);
  }
  return {
    success: true,
    inputPath,
    keyPath,
    outputPath: destPath,
    restoredCells: 6,
    durationMs: 40
  };
}

export async function OpenContainingFolder(targetPath: string): Promise<void> {
  const app = getApp();
  if (app?.OpenContainingFolder) {
    await app.OpenContainingFolder(targetPath);
  } else {
    console.info('[Wails Dev] OpenContainingFolder:', targetPath);
  }
}

export function EventsOn(eventName: string, callback: AnyFunction): () => void {
  const runtime = getRuntime();
  if (runtime?.EventsOn) {
    runtime.EventsOn(eventName, callback);
    return () => EventsOff(eventName, callback);
  }

  // Fallback in-memory event listener
  if (!devEventListeners.has(eventName)) {
    devEventListeners.set(eventName, new Set());
  }
  devEventListeners.get(eventName)!.add(callback);

  return () => {
    EventsOff(eventName, callback);
  };
}

export function EventsOff(eventName: string, callback?: AnyFunction): void {
  const runtime = getRuntime();
  if (runtime?.EventsOff) {
    if (callback) {
      runtime.EventsOff(eventName, callback);
    } else {
      runtime.EventsOff(eventName);
    }
    return;
  }

  // Fallback in-memory event unregister
  const set = devEventListeners.get(eventName);
  if (!set) return;
  if (callback) {
    set.delete(callback);
    if (set.size === 0) {
      devEventListeners.delete(eventName);
    }
  } else {
    devEventListeners.delete(eventName);
  }
}

// Dev helper to trigger mock events in development or unit tests
export function DevEmitEvent(eventName: string, ...args: any[]): void {
  const runtime = getRuntime();
  if (runtime?.EventsEmit) {
    runtime.EventsEmit(eventName, ...args);
    return;
  }
  const set = devEventListeners.get(eventName);
  if (set) {
    for (const cb of set) {
      try {
        cb(...args);
      } catch (err) {
        console.error(`Error in event handler for ${eventName}:`, err);
      }
    }
  }
}

export function OnFileDrop(
  callback: (x: number, y: number, paths: string[]) => void
): () => void {
  const runtime = getRuntime();
  if (runtime?.OnFileDrop) {
    runtime.OnFileDrop(callback, false);
    return () => {
      if (runtime.OnFileDropOff) {
        runtime.OnFileDropOff();
      }
    };
  }
  return () => {};
}
