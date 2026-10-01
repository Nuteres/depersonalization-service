/**
 * state.js — Глобальное состояние приложения и вспомогательные функции
 */

// Global Application Mode: 'depersonalize' | 'restore'
let currentAppMode = 'depersonalize';

// Depersonalization File & Data State
let currentFilePath = '';
let currentFileInfo = null;
let currentSalt = '152fz-secure-salt';
let currentSavedPath = '';
let currentView = 'after'; // 'after' (очищенные) | 'before' (исходные с ПДн)
let currentTextViewMode = 'after'; // 'after' | 'before' для фрагментов текста

// Multi-Sheet State (0-based index)
let currentActiveSheetIndex = 0;

// Per-sheet disabled column sets: map of sheetName -> Set<number> (1-based column indices)
let disabledColumnsBySheet = {};

// Restoration Mode State
let restoreInputPath = '';
let restoreInputInfo = null;
let restoreKeyPath = '';
let restoreKeyInfo = null;
let restorePassword = '';

// Loaded preview rows for infinite scrolling
let allLoadedRows = [];
let isLoadingMore = false;
let hasMoreRows = true;

// Track width in pixels for each column: [col_0 (#), col_1, col_2, ...]
let columnWidths = [];

/**
 * Returns the name of the currently active sheet.
 */
function getActiveSheetName() {
  if (currentFileInfo && currentFileInfo.sheets && currentFileInfo.sheets.length > 0) {
    const s = currentFileInfo.sheets[currentActiveSheetIndex];
    return s ? s.name : (currentFileInfo.activeSheet || '');
  }
  return '';
}

/**
 * Returns the Set of disabled column indices for the active sheet.
 */
function getActiveDisabledSet() {
  const sName = getActiveSheetName();
  if (!disabledColumnsBySheet[sName]) {
    disabledColumnsBySheet[sName] = new Set();
  }
  return disabledColumnsBySheet[sName];
}

/**
 * Compatibility proxy for disabledColumns operating on the active sheet.
 */
const disabledColumns = {
  has: (idx) => getActiveDisabledSet().has(idx),
  add: (idx) => getActiveDisabledSet().add(idx),
  delete: (idx) => getActiveDisabledSet().delete(idx),
  clear: () => {
    const set = getActiveDisabledSet();
    set.clear();
  },
  get size() {
    return getActiveDisabledSet().size;
  },
  [Symbol.iterator]: function* () {
    yield* getActiveDisabledSet();
  }
};

/**
 * Escapes HTML characters for safe rendering into DOM strings.
 */
function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/**
 * Measures text width in pixels using a 2D canvas context for accurate column auto-sizing.
 */
let _textMeasureCtx = null;
function measureTextPx(text, font) {
  if (!text) return 0;
  try {
    if (!_textMeasureCtx) {
      const c = document.createElement('canvas');
      _textMeasureCtx = c.getContext('2d');
    }
    if (_textMeasureCtx) {
      _textMeasureCtx.font = font || '600 12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
      return Math.ceil(_textMeasureCtx.measureText(String(text)).width);
    }
  } catch (e) {
    // Fallback for environments without canvas context
  }
  return Math.ceil(String(text).length * 8.8);
}
