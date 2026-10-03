export interface EntityInfo {
  columnIndex: number;
  columnName: string;
  type: string;
  description: string;
  action: string;
}

export interface PreviewRow {
  index: number;
  before: string[];
  after: string[];
}

export interface SheetInfo {
  name: string;
  index: number;
  rowCount: number;
  headers: string[];
  entities: EntityInfo[];
  previewRows: PreviewRow[];
}

export interface DocxTextSnippet {
  original: string;
  anonymized: string;
  type: string;
}

export interface TextParagraphSegment {
  text: string;
  isEntity: boolean;
  type?: string;
  original?: string;
  anonymized?: string;
}

export interface TextParagraphPreview {
  index: number;
  segments: TextParagraphSegment[];
}

export interface FileInfo {
  path: string;
  name: string;
  extension: string;
  fileType: 'table' | 'text' | 'pdf' | string;
  format: string;
  supportedExportFormats?: string[];
  size: number;
  rowCount: number;
  paragraphCount?: number;
  pageCount: number;
  entities: EntityInfo[];
  headers: string[];
  previewRows: PreviewRow[];
  sheets?: SheetInfo[];
  activeSheet?: string;
  textSnippets?: DocxTextSnippet[];
  paragraphPreviews?: TextParagraphPreview[];
  pdfTextSnippet?: string;
  pdfRedactedSnippet?: string;
}

export interface ProcessOptions {
  inputPath: string;
  outputPath?: string;
  salt?: string;
  disabledCols?: number[];
  disabledBySheet?: Record<string, number[]>;
  createKey?: boolean;
  keyPassword?: string;
  keyPath?: string;
}

export interface ProcessResult {
  success: boolean;
  inputPath: string;
  outputPath: string;
  keyPath?: string;
  rowsCount: number;
  entitiesCount: number;
  durationMs: number;
  message: string;
  error?: string;
}

export interface KeyFileInfo {
  path: string;
  originalFileName: string;
  createdAt: string;
  recordsCount: number;
  valid: boolean;
  error?: string;
}

export interface RestoreOptions {
  inputPath: string;
  keyPath: string;
  password?: string;
  outputPath: string;
}

export interface RestoreResult {
  success: boolean;
  inputPath: string;
  keyPath: string;
  outputPath: string;
  restoredCells: number;
  durationMs: number;
  error?: string;
}

export interface AppInfo {
  version: string;
  os: string;
  arch: string;
}

export interface ProgressEventData {
  processed: number;
  total: number;
  percent: number;
}
