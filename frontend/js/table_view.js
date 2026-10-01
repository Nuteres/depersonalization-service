/**
 * table_view.js — Рендеринг таблицы, расчет ширины колонок, ресайзинг и динамическая подгрузка строк
 */

function applyTableWidth() {
  if (columnWidths.length === 0) return;
  const total = columnWidths.reduce((a, b) => a + b, 0);
  const table = document.getElementById('preview-table');
  if (table) table.style.width = `${total}px`;
  const modalTable = document.getElementById('modal-preview-table');
  if (modalTable) modalTable.style.width = `${total}px`;
}

function calculateOptimalColumnWidth(h, col1Based) {
  const ent = currentFileInfo && currentFileInfo.entities ? currentFileInfo.entities.find(e => e.columnIndex === col1Based) : null;
  const isPiiCol = !!ent;

  // 1. Measure header text width (bold 12px font)
  const headerTextW = measureTextPx(h, '600 12px -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif');

  // 2. Badge "[ ПДн: Замена ]" takes ~88px + 6px margin
  const badgeW = isPiiCol ? 96 : 0;

  // 3. Spacing: th px-3 (24px) + gap-1.5 (6px) + col-resizer handle (10px) + safety buffer (16px) = 56px
  let maxNeeded = headerTextW + badgeW + 56;

  // 4. Sample preview rows (up to 15 rows) so cell data also displays comfortably
  if (currentFileInfo && currentFileInfo.previewRows && currentFileInfo.previewRows.length > 0) {
    const cIdx = col1Based - 1;
    const sampleRows = currentFileInfo.previewRows.slice(0, 15);
    for (const row of sampleRows) {
      const vBefore = (row.before && row.before[cIdx] != null) ? String(row.before[cIdx]) : '';
      const vAfter = (row.after && row.after[cIdx] != null) ? String(row.after[cIdx]) : '';
      
      if (vBefore) {
        const font = isPiiCol ? '400 12px "JetBrains Mono", monospace' : '400 13px -apple-system, BlinkMacSystemFont, sans-serif';
        const wBefore = measureTextPx(vBefore, font) + 36;
        if (wBefore > maxNeeded) maxNeeded = wBefore;
      }
      if (vAfter) {
        const font = isPiiCol ? '500 12px "JetBrains Mono", monospace' : '400 13px -apple-system, BlinkMacSystemFont, sans-serif';
        const wAfter = measureTextPx(vAfter, font) + 36;
        if (wAfter > maxNeeded) maxNeeded = wAfter;
      }
    }
  }

  // Minimum 130px, maximum default 520px
  return Math.min(520, Math.max(130, Math.ceil(maxNeeded)));
}

function renderTableHeaders() {
  const headerRow = document.getElementById('table-header-row');
  if (!headerRow || !currentFileInfo || !currentFileInfo.headers) return;

  // Initialize column widths if not already populated for current header count
  if (columnWidths.length !== currentFileInfo.headers.length + 1) {
    columnWidths = [50]; // Column 0: '#'
    currentFileInfo.headers.forEach((h, idx) => {
      const col1Based = idx + 1;
      const defaultW = calculateOptimalColumnWidth(h, col1Based);
      columnWidths.push(defaultW);
    });
  }

  applyTableWidth();

  let ths = `<th style="width: ${columnWidths[0]}px;" class="py-3 px-3 font-normal text-center text-[#A8A29E] bg-[#FAF9F5] border-r border-[#F0EFEB] select-none">#</th>`;
  const disabledSet = getActiveDisabledSet();

  currentFileInfo.headers.forEach((h, idx) => {
    const col1Based = idx + 1;
    const ent = currentFileInfo.entities ? currentFileInfo.entities.find(e => e.columnIndex === col1Based) : null;
    const isPiiCol = !!ent;
    const isDisabled = disabledSet.has(col1Based);
    const colW = columnWidths[col1Based] || 180;

    let badge = '';
    if (isPiiCol) {
      if (isDisabled) {
        badge = '<span class="ml-1.5 shrink-0 text-[10px] text-stone-500 bg-stone-100 border border-stone-200 px-1 py-0.5 rounded font-mono line-through" title="Деперсонализация отключена">ПДн</span>';
      } else {
        badge = '<span class="ml-1.5 shrink-0 text-[10px] text-emerald-700 bg-emerald-50 border border-emerald-200 px-1 py-0.5 rounded font-mono font-medium" title="Категория: ' + escapeHtml(ent.type) + '">ПДн: Замена</span>';
      }
    }

    ths += `
      <th style="width: ${colW}px;" class="py-3 px-3 font-semibold text-[#1C1917] select-none border-r border-[#F0EFEB]">
        <div class="flex items-center justify-between gap-1.5 w-full min-w-0 overflow-hidden pr-1">
          <span class="truncate min-w-0 flex-1 whitespace-nowrap" title="${escapeHtml(h)}">${escapeHtml(h)}</span>
          ${badge}
        </div>
        <div class="col-resizer" onmousedown="initResize(event, ${col1Based})"></div>
      </th>
    `;
  });

  if (headerRow) headerRow.innerHTML = ths;
  const modalHeaderRow = document.getElementById('modal-table-header-row');
  if (modalHeaderRow) modalHeaderRow.innerHTML = ths;
}

// Resizable Column Logic with free shrinking below content length
function initResize(e, colIndex) {
  e.preventDefault();
  e.stopPropagation();

  const startX = e.pageX;
  const startWidth = columnWidths[colIndex] || 180;
  const resizer = e.target;
  resizer.classList.add('resizing');
  document.body.style.cursor = 'col-resize';
  document.body.style.userSelect = 'none';

  function onMouseMove(moveEvent) {
    const diff = moveEvent.pageX - startX;
    // User can shrink down to 25px freely; text will be clipped with ellipsis
    const newWidth = Math.max(25, Math.round(startWidth + diff));
    columnWidths[colIndex] = newWidth;

    // Apply width to both main and modal tables
    const mainTh = document.querySelector(`#table-header-row th:nth-child(${colIndex + 1})`);
    if (mainTh) mainTh.style.width = `${newWidth}px`;
    const modalTh = document.querySelector(`#modal-table-header-row th:nth-child(${colIndex + 1})`);
    if (modalTh) modalTh.style.width = `${newWidth}px`;

    applyTableWidth();
  }

  function onMouseUp() {
    resizer.classList.remove('resizing');
    document.body.style.cursor = '';
    document.body.style.userSelect = '';
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
  }

  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
}

function renderRowHtml(row, i) {
  const vals = row.before || [];
  const afterVals = row.after || [];
  const disabledSet = getActiveDisabledSet();

  let html = `<td class="py-3 px-3 font-mono text-center text-[#A8A29E] text-xs bg-[#FAF9F5] border-r border-[#F0EFEB]">${String(row.index || i + 1).padStart(2, '0')}</td>`;

  vals.forEach((v, cIdx) => {
    const col1Based = cIdx + 1;
    const isPiiCol = currentFileInfo.entities && currentFileInfo.entities.some(e => e.columnIndex === col1Based);
    const isDisabled = disabledSet.has(col1Based);

    let displayVal = '';
    let cls = "py-3 px-4 text-xs font-mono border-r border-[#F0EFEB] truncate";

    if (currentView === 'after') {
      if (isDisabled) {
        displayVal = v;
        cls += ' text-[#78716C] bg-stone-50/70 italic';
      } else if (isPiiCol) {
        displayVal = afterVals[cIdx] !== undefined ? afterVals[cIdx] : v;
        cls += ' font-medium text-[#1C1917] bg-emerald-50/40';
      } else {
        displayVal = v;
        cls += ' text-[#57534E]';
      }
    } else {
      // before view
      displayVal = v;
      if (isPiiCol) {
        cls += isDisabled ? ' text-[#78716C] bg-stone-50/50' : ' font-medium text-rose-700 bg-rose-50/50';
      } else {
        cls += ' text-[#57534E]';
      }
    }

    html += `<td class="${cls}" title="${escapeHtml(displayVal)}">${escapeHtml(displayVal)}</td>`;
  });

  return html;
}

// Render fixed 10 rows on main view
function renderTableBody() {
  const tbody = document.getElementById('table-rows');
  if (!tbody) return;

  if (!currentFileInfo || allLoadedRows.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="${(currentFileInfo && currentFileInfo.headers ? currentFileInfo.headers.length + 1 : 2)}" class="py-8 text-center text-xs text-[#78716C] font-mono">
          В таблице нет строк данных для отображения
        </td>
      </tr>
    `;
    return;
  }

  tbody.innerHTML = '';
  const mainRows = allLoadedRows.slice(0, 10);
  mainRows.forEach((row, i) => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-[#FAF9F5] transition-colors whitespace-nowrap';
    tr.innerHTML = renderRowHtml(row, i);
    tbody.appendChild(tr);
  });

  updatePreviewCountLabel();
}

function updatePreviewCountLabel() {
  const previewLabel = document.getElementById('preview-count-label');
  const mainSummary = document.getElementById('main-preview-summary');
  if (currentFileInfo) {
    const rowCount = currentFileInfo.rowCount || 0;
    const shownCount = Math.min(10, allLoadedRows.length);
    const sheetName = getActiveSheetName();
    const prefix = (currentFileInfo.sheets && currentFileInfo.sheets.length > 1) ? `[Лист: ${sheetName}] ` : '';
    if (previewLabel) {
      previewLabel.textContent = `${prefix}(первые ${shownCount} строк)`;
    }
    if (mainSummary) {
      mainSummary.textContent = `${prefix}Показано ${shownCount} из ${rowCount.toLocaleString('ru-RU')} строк`;
    }
  }
}

function setViewMode(mode) {
  currentView = mode;
  const btnAfter = document.getElementById('btn-show-after');
  const btnBefore = document.getElementById('btn-show-before');
  const modalBtnAfter = document.getElementById('modal-btn-show-after');
  const modalBtnBefore = document.getElementById('modal-btn-show-before');

  const activeCls = 'px-3 py-1 bg-[#1C1917] text-white font-semibold transition cursor-pointer';
  const inactiveCls = 'px-3 py-1 text-[#78716C] hover:text-[#1C1917] transition cursor-pointer';

  if (mode === 'after') {
    if (btnAfter) btnAfter.className = activeCls;
    if (btnBefore) btnBefore.className = inactiveCls;
    if (modalBtnAfter) modalBtnAfter.className = activeCls;
    if (modalBtnBefore) modalBtnBefore.className = inactiveCls;
  } else {
    if (btnBefore) btnBefore.className = activeCls;
    if (btnAfter) btnAfter.className = inactiveCls;
    if (modalBtnBefore) modalBtnBefore.className = activeCls;
    if (modalBtnAfter) modalBtnAfter.className = inactiveCls;
  }

  renderTableBody();
  renderModalTableBody();
}

// ==================== EXPANDED PREVIEW MODAL ====================

async function openExpandedPreviewModal() {
  if (!currentFileInfo || !currentFileInfo.headers) {
    alert("Пожалуйста, сначала откройте файл для предпросмотра");
    return;
  }
  const modal = document.getElementById('expanded-preview-modal');
  if (!modal) return;
  modal.classList.remove('hidden');
  document.body.classList.add('modal-open');
  document.body.style.overflow = 'hidden';

  const titleEl = document.getElementById('modal-file-title');
  if (titleEl) titleEl.textContent = currentFileInfo.name || '';
  const colsSummary = document.getElementById('modal-columns-summary');
  if (colsSummary && currentFileInfo) {
    const sheetName = getActiveSheetName();
    const sheetText = (currentFileInfo.sheets && currentFileInfo.sheets.length > 1) ? `Лист: ${sheetName} • ` : '';
    colsSummary.textContent = `${sheetText}Колонок: ${currentFileInfo.headers ? currentFileInfo.headers.length : 0} • ПДн: ${currentFileInfo.entities ? currentFileInfo.entities.length : 0}`;
  }

  if (typeof renderSheetTabs === 'function') {
    renderSheetTabs();
  }
  renderTableHeaders();
  renderModalTableBody();
  updateModalPreviewCountLabel();

  // Pre-fetch enough rows to fill the viewport
  if (hasMoreRows && allLoadedRows.length < 30) {
    await loadNextRowBatch();
    if (hasMoreRows && allLoadedRows.length < 50) {
      await loadNextRowBatch();
    }
  }
}

function closeExpandedPreviewModal() {
  const modal = document.getElementById('expanded-preview-modal');
  if (modal) modal.classList.add('hidden');
  document.body.classList.remove('modal-open');
  document.body.style.overflow = '';
}

function updateModalPreviewCountLabel() {
  const modalCount = document.getElementById('modal-preview-count-label');
  if (modalCount && currentFileInfo) {
    const sheetName = getActiveSheetName();
    const prefix = (currentFileInfo.sheets && currentFileInfo.sheets.length > 1) ? `[Лист: ${sheetName}] ` : '';
    modalCount.textContent = `${prefix}(показано ${allLoadedRows.length.toLocaleString('ru-RU')} из ${currentFileInfo.rowCount.toLocaleString('ru-RU')} строк)`;
  }
}

function renderModalTableBody() {
  const tbody = document.getElementById('modal-table-rows');
  if (!tbody) return;

  if (!currentFileInfo || allLoadedRows.length === 0) {
    tbody.innerHTML = `
      <tr>
        <td colspan="${(currentFileInfo && currentFileInfo.headers ? currentFileInfo.headers.length + 1 : 2)}" class="py-8 text-center text-xs text-[#78716C] font-mono">
          В таблице нет строк данных для отображения
        </td>
      </tr>
    `;
    return;
  }

  tbody.innerHTML = '';
  allLoadedRows.forEach((row, i) => {
    const tr = document.createElement('tr');
    tr.className = 'hover:bg-[#FAF9F5] transition-colors whitespace-nowrap';
    tr.innerHTML = renderRowHtml(row, i);
    tbody.appendChild(tr);
  });

  updateModalPreviewCountLabel();
}

async function handleModalTableScroll() {
  const container = document.getElementById('modal-table-scroll-container');
  if (!container || isLoadingMore || !hasMoreRows || !currentFilePath) return;

  if (container.scrollTop + container.clientHeight >= container.scrollHeight - 250) {
    await loadNextRowBatch();
  }
}

async function loadNextRowBatch() {
  if (isLoadingMore || !hasMoreRows || !currentFilePath) return;
  isLoadingMore = true;

  const tbody = document.getElementById('modal-table-rows');
  let loadingTr = null;
  if (tbody) {
    loadingTr = document.createElement('tr');
    loadingTr.id = 'modal-table-loading-row';
    const colSpan = currentFileInfo && currentFileInfo.headers ? currentFileInfo.headers.length + 1 : 2;
    loadingTr.innerHTML = `
      <td colspan="${colSpan}" class="py-3 text-center text-xs text-[#78716C] font-mono bg-[#FAF9F5]">
        <span class="inline-block animate-spin mr-2">⏳</span> Подгрузка следующих строк...
      </td>
    `;
    tbody.appendChild(loadingTr);
  }

  try {
    const offset = allLoadedRows.length;
    const limit = 30;
    const disabledArr = Array.from(getActiveDisabledSet());
    const sheetName = getActiveSheetName();

    if (window.go && window.go.main && window.go.main.App) {
      let newRows = null;
      if (window.go.main.App.GetSheetRows) {
        newRows = await window.go.main.App.GetSheetRows(currentFilePath, sheetName, offset, limit, currentSalt, disabledArr);
      } else if (window.go.main.App.GetTableRows) {
        newRows = await window.go.main.App.GetTableRows(currentFilePath, offset, limit, currentSalt, disabledArr);
      }

      if (newRows && newRows.length > 0) {
        allLoadedRows.push(...newRows);
        
        if (tbody) {
          newRows.forEach((row, idx) => {
            const tr = document.createElement('tr');
            tr.className = 'hover:bg-[#FAF9F5] transition-colors whitespace-nowrap';
            tr.innerHTML = renderRowHtml(row, offset + idx);
            if (loadingTr && loadingTr.parentElement === tbody) {
              tbody.insertBefore(tr, loadingTr);
            } else {
              tbody.appendChild(tr);
            }
          });
        }

        if (allLoadedRows.length >= currentFileInfo.rowCount || newRows.length < limit) {
          hasMoreRows = false;
        }
      } else {
        hasMoreRows = false;
      }
    } else {
      hasMoreRows = false;
    }
  } catch (err) {
    console.error("Error loading table rows batch:", err);
    hasMoreRows = false;
  } finally {
    if (loadingTr && loadingTr.parentElement) {
      loadingTr.parentElement.removeChild(loadingTr);
    }
    isLoadingMore = false;
    updateModalPreviewCountLabel();
    updatePreviewCountLabel();
  }
}
