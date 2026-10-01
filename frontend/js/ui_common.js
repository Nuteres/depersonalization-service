/**
 * ui_common.js — Общие элементы управления интерфейсом, модальные окна и события Wails
 */

function setAppMode(mode) {
  currentAppMode = mode;
  const depView = document.getElementById('depersonalization-view');
  const resView = document.getElementById('restoration-view');
  const tabDep = document.getElementById('mode-tab-depersonalize');
  const tabRes = document.getElementById('mode-tab-restore');
  const dotDep = document.getElementById('dot-depersonalize');
  const dotRes = document.getElementById('dot-restore');

  const activeTabCls = 'px-3.5 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition flex items-center space-x-1.5 cursor-pointer';
  const inactiveTabCls = 'px-3.5 py-1.5 text-[#78716C] hover:text-[#1C1917] transition flex items-center space-x-1.5 cursor-pointer';

  if (mode === 'depersonalize') {
    if (depView) depView.classList.remove('hidden');
    if (resView) resView.classList.add('hidden');
    if (tabDep) tabDep.className = activeTabCls;
    if (tabRes) tabRes.className = inactiveTabCls;
    if (dotDep) dotDep.className = 'w-2 h-2 rounded-full bg-emerald-600';
    if (dotRes) dotRes.className = 'w-2 h-2 rounded-full bg-stone-400';
  } else {
    if (depView) depView.classList.add('hidden');
    if (resView) resView.classList.remove('hidden');
    if (tabRes) tabRes.className = activeTabCls;
    if (tabDep) tabDep.className = inactiveTabCls;
    if (dotRes) dotRes.className = 'w-2 h-2 rounded-full bg-emerald-600';
    if (dotDep) dotDep.className = 'w-2 h-2 rounded-full bg-stone-400';
    if (typeof updateRestoreButtonState === 'function') {
      updateRestoreButtonState();
    }
  }
}

function setDocMode(mode) {
  const tabTable = document.getElementById('tab-table');
  const tabPdf = document.getElementById('tab-pdf');
  const contentTable = document.getElementById('mode-table-content');
  const contentPdf = document.getElementById('mode-pdf-content');

  if (mode === 'table') {
    if (contentTable) contentTable.classList.remove('hidden');
    if (contentPdf) contentPdf.classList.add('hidden');
    if (tabTable) tabTable.className = 'px-4 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition cursor-pointer';
    if (tabPdf) tabPdf.className = 'px-4 py-1.5 text-[#78716C] hover:text-[#1C1917] transition cursor-pointer';
  } else {
    if (contentTable) contentTable.classList.add('hidden');
    if (contentPdf) contentPdf.classList.remove('hidden');
    if (tabPdf) tabPdf.className = 'px-4 py-1.5 bg-white text-[#1C1917] shadow-sm font-semibold transition cursor-pointer';
    if (tabTable) tabTable.className = 'px-4 py-1.5 text-[#78716C] hover:text-[#1C1917] transition cursor-pointer';
  }
}

function handleDroppedFilePath(filePath) {
  if (!filePath) return;
  const lower = filePath.toLowerCase();
  if (lower.endsWith('.depmap')) {
    setAppMode('restore');
    if (typeof loadRestoreKeyFile === 'function') {
      loadRestoreKeyFile(filePath);
    }
  } else if (currentAppMode === 'restore') {
    if (lower.endsWith('.xlsx') || lower.endsWith('.csv') || lower.endsWith('.tsv') || lower.endsWith('.xlsm')) {
      if (typeof loadRestoreDataFile === 'function') {
        loadRestoreDataFile(filePath);
      }
    } else {
      setAppMode('depersonalize');
      if (typeof loadFile === 'function') {
        loadFile(filePath);
      }
    }
  } else {
    if (typeof loadFile === 'function') {
      loadFile(filePath);
    }
  }
}

// 1. Initialize System Environment Badge & Runtime Event Subscriptions
async function initAppEnv() {
  let osName = 'Система';
  let version = 'v1.0';

  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetAppInfo) {
      const info = await window.go.main.App.GetAppInfo();
      if (info) {
        if (info.os) osName = info.os;
        if (info.version) version = info.version;
      }
    } else {
      // Browser fallback detection
      const ua = navigator.userAgent || '';
      if (ua.includes('Mac') || ua.includes('Darwin')) osName = 'macOS';
      else if (ua.includes('Win')) osName = 'Windows';
      else if (ua.includes('Linux')) osName = 'Linux';
    }
  } catch (err) {
    console.warn("Could not detect OS info:", err);
  }

  const badge = document.getElementById('app-env-badge');
  if (badge) {
    badge.textContent = `${version} • ${osName}`;
  }

  // Subscribe to Wails progress and backend drop events
  if (window.runtime && window.runtime.EventsOn) {
    window.runtime.EventsOn('process-progress', (data) => {
      onProgressUpdate(data);
    });
    window.runtime.EventsOn('restore-progress', (data) => {
      onProgressUpdate(data);
    });
    window.runtime.EventsOn('app:file-dropped', (filePath) => {
      if (filePath) {
        const overlay = document.getElementById('drag-drop-overlay');
        if (overlay) overlay.classList.add('hidden');
        dragCounter = 0;
        handleDroppedFilePath(filePath);
      }
    });
  }
}

// ==================== PROGRESS MODAL ====================

function openProgressModal(totalCount, mode) {
  const modal = document.getElementById('progress-modal');
  const fill = document.getElementById('progress-bar-fill');
  const pctBadge = document.getElementById('progress-pct-badge');
  const titleEl = document.getElementById('progress-modal-title');
  const statusText = document.getElementById('progress-status-text');
  const countText = document.getElementById('progress-count-text');
  const successBox = document.getElementById('progress-success-box');
  const revealBtn = document.getElementById('btn-reveal-folder');
  const keyPathEl = document.getElementById('progress-key-path');
  const indicator = document.getElementById('progress-indicator');

  if (modal) modal.classList.remove('hidden');
  if (fill) fill.style.width = '0%';
  if (pctBadge) pctBadge.textContent = '0%';
  if (keyPathEl) keyPathEl.classList.add('hidden');
  if (successBox) successBox.classList.add('hidden');
  if (revealBtn) revealBtn.classList.add('hidden');
  if (indicator) {
    indicator.className = 'w-2.5 h-2.5 rounded-full bg-emerald-600 animate-pulse';
  }

  if (mode === 'restore') {
    if (titleEl) titleEl.textContent = 'Восстановление исходных данных';
    if (statusText) statusText.textContent = 'Расшифровка ключа и обратная персонализация...';
    if (countText) countText.textContent = '0%';
  } else {
    if (titleEl) titleEl.textContent = 'Деперсонализация ПДн';
    if (statusText) statusText.textContent = 'Потоковая обработка строк...';
    if (countText) countText.textContent = `0 / ${totalCount.toLocaleString('ru-RU')} строк`;
  }
}

function onProgressUpdate(data) {
  if (!data) return;
  const fill = document.getElementById('progress-bar-fill');
  const pctBadge = document.getElementById('progress-pct-badge');
  const countText = document.getElementById('progress-count-text');

  const pct = Math.min(100, Math.max(0, data.percent || 0));
  if (fill) fill.style.width = `${pct}%`;
  if (pctBadge) pctBadge.textContent = `${pct}%`;
  if (countText && data.total) {
    countText.textContent = `${(data.processed || 0).toLocaleString('ru-RU')} / ${data.total.toLocaleString('ru-RU')} строк`;
  }
}

function onProcessingCompleted(res, mode) {
  currentSavedPath = res.outputPath || '';
  const fill = document.getElementById('progress-bar-fill');
  const pctBadge = document.getElementById('progress-pct-badge');
  const statusText = document.getElementById('progress-status-text');
  const successBox = document.getElementById('progress-success-box');
  const successTitle = document.getElementById('progress-success-title');
  const pathEl = document.getElementById('progress-saved-path');
  const keyPathEl = document.getElementById('progress-key-path');
  const statsEl = document.getElementById('progress-stats-text');
  const revealBtn = document.getElementById('btn-reveal-folder');
  const indicator = document.getElementById('progress-indicator');

  if (fill) fill.style.width = '100%';
  if (pctBadge) pctBadge.textContent = '100%';
  if (indicator) {
    indicator.className = 'w-2.5 h-2.5 rounded-full bg-emerald-600';
  }
  if (successBox) successBox.classList.remove('hidden');
  if (pathEl) {
    pathEl.textContent = res.outputPath;
    pathEl.title = res.outputPath || '';
  }

  if (mode === 'restore') {
    if (statusText) statusText.textContent = 'Восстановление успешно завершено!';
    if (successTitle) {
      successTitle.innerHTML = '<span>✔</span><span>Исходные персональные данные успешно восстановлены</span>';
    }
    if (statsEl) {
      const duration = res.durationMs || 0;
      const cells = res.restoredCells || 0;
      statsEl.textContent = `Восстановлено ячеек с ПДн: ${cells.toLocaleString('ru-RU')} • Время: ${duration} мс`;
    }
  } else {
    if (statusText) statusText.textContent = 'Деперсонализация успешно завершена!';
    if (successTitle) {
      successTitle.innerHTML = '<span>✔</span><span>Файл успешно сохранен без персональных данных</span>';
    }
    if (statsEl) {
      const duration = res.durationMs || 0;
      const count = res.rowsCount || 0;
      statsEl.textContent = `Обработано строк: ${count.toLocaleString('ru-RU')} • Время: ${duration} мс`;
    }

    if (res.keyPath && keyPathEl) {
      const escapedPath = escapeHtml(res.keyPath);
      keyPathEl.innerHTML = `
        <div class="flex items-center justify-between gap-2">
          <span class="font-bold flex items-center space-x-1">
            <span>🔑</span><span>Ключ для обратного восстановления:</span>
          </span>
          <button type="button" onclick="navigator.clipboard.writeText('${escapedPath}'); alert('Путь к файлу-ключу скопирован в буфер!');" class="text-[10px] text-emerald-800 underline font-sans hover:text-emerald-950 cursor-pointer">
            📋 Копировать путь
          </button>
        </div>
        <div class="text-[10px] font-mono opacity-90 truncate mt-0.5" title="${escapedPath}">${escapedPath}</div>
        <div class="text-[10px] text-emerald-700 font-sans mt-0.5">Сохраните этот файл в безопасном месте. Без него обратная персонализация невозможна.</div>
      `;
      keyPathEl.classList.remove('hidden');
    }
  }

  if (revealBtn) {
    revealBtn.classList.remove('hidden');
  }
}

function closeProgressModal() {
  const modal = document.getElementById('progress-modal');
  if (modal) modal.classList.add('hidden');
}

async function handleRevealFolder() {
  if (!currentSavedPath) return;
  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.OpenContainingFolder) {
      await window.go.main.App.OpenContainingFolder(currentSavedPath);
    }
  } catch (e) {
    alert("Не удалось открыть папку: " + e);
  }
}

// ==================== DRAG & DROP & KEYBOARD ====================

// Close modal on Escape
window.addEventListener('keydown', (e) => {
  if (e.key === 'Escape') {
    if (typeof closeExpandedPreviewModal === 'function') {
      closeExpandedPreviewModal();
    }
    if (typeof closeSaveOptionsModal === 'function') {
      closeSaveOptionsModal();
    }
  }
});

let dragCounter = 0;
window.addEventListener('dragenter', (e) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounter++;
  const overlay = document.getElementById('drag-drop-overlay');
  if (overlay) overlay.classList.remove('hidden');
});

window.addEventListener('dragleave', (e) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounter--;
  if (dragCounter <= 0) {
    dragCounter = 0;
    const overlay = document.getElementById('drag-drop-overlay');
    if (overlay) overlay.classList.add('hidden');
  }
});

window.addEventListener('dragover', (e) => {
  e.preventDefault();
  e.stopPropagation();
});

window.addEventListener('drop', (e) => {
  e.preventDefault();
  e.stopPropagation();
  dragCounter = 0;
  const overlay = document.getElementById('drag-drop-overlay');
  if (overlay) overlay.classList.add('hidden');

  if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
    const file = e.dataTransfer.files[0];
    if (file.path) {
      handleDroppedFilePath(file.path);
    }
  }
});

function initDropListeners() {
  if (window.runtime && window.runtime.OnFileDrop) {
    try {
      window.runtime.OnFileDrop((x, y, paths) => {
        const overlay = document.getElementById('drag-drop-overlay');
        if (overlay) overlay.classList.add('hidden');
        dragCounter = 0;
        if (paths && paths.length > 0) {
          handleDroppedFilePath(paths[0]);
        }
      }, false);
    } catch (e) {
      console.warn("OnFileDrop registration error:", e);
    }
  }
}

function initModalWheelProtection() {
  const modalScrollBox = document.getElementById('modal-table-scroll-container');
  if (modalScrollBox) {
    modalScrollBox.addEventListener('wheel', (e) => {
      e.stopPropagation();

      if (e.deltaY > 0 && hasMoreRows && !isLoadingMore) {
        const isNearBottom = modalScrollBox.scrollTop + modalScrollBox.clientHeight >= modalScrollBox.scrollHeight - 250;
        const isNotScrollable = modalScrollBox.scrollHeight <= modalScrollBox.clientHeight + 40;
        if ((isNearBottom || isNotScrollable) && typeof loadNextRowBatch === 'function') {
          loadNextRowBatch();
        }
      }
    }, { passive: true });
  }

  const modalBackdrop = document.getElementById('expanded-preview-modal');
  if (modalBackdrop) {
    modalBackdrop.addEventListener('wheel', (e) => {
      e.stopPropagation();
    }, { passive: true });
  }
}

// Bootstrap initialization
initAppEnv();
initDropListeners();
initModalWheelProtection();
window.addEventListener('DOMContentLoaded', () => {
  initAppEnv();
  initDropListeners();
  initModalWheelProtection();
});

let wailsInitTries = 0;
const wailsInitInterval = setInterval(() => {
  wailsInitTries++;
  if (window.go && window.go.main && window.go.main.App) {
    clearInterval(wailsInitInterval);
    initAppEnv();
    initDropListeners();
    initModalWheelProtection();
  } else if (wailsInitTries > 30) {
    clearInterval(wailsInitInterval);
  }
}, 100);
