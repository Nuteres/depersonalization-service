/**
 * depersonalize.js — Управление деперсонализацией, листами Excel, карточками ПДн и сохранением
 */

// Render Multi-Sheet Tabs Bar (Main view and Expanded Modal)
function renderSheetTabs() {
  const bar = document.getElementById('excel-sheets-bar');
  const list = document.getElementById('excel-sheets-list');
  const modalBar = document.getElementById('modal-excel-sheets-bar');
  const modalList = document.getElementById('modal-excel-sheets-list');

  if (!currentFileInfo || !currentFileInfo.sheets || currentFileInfo.sheets.length <= 1) {
    if (bar) { bar.classList.add('hidden'); bar.classList.remove('flex'); }
    if (modalBar) { modalBar.classList.add('hidden'); modalBar.classList.remove('flex'); }
    return;
  }

  if (bar) { bar.classList.remove('hidden'); bar.classList.add('flex'); }
  if (modalBar) { modalBar.classList.remove('hidden'); modalBar.classList.add('flex'); }

  let tabsHtml = '';
  let modalTabsHtml = '';

  currentFileInfo.sheets.forEach((sheet, idx) => {
    const isActive = idx === currentActiveSheetIndex;
    const disabledSet = disabledColumnsBySheet[sheet.name] || new Set();
    const disabledCount = disabledSet.size;
    const rowCountStr = (sheet.rowCount || 0).toLocaleString('ru-RU');

    let badgeHtml = '';
    if (disabledCount > 0) {
      badgeHtml = `<span class="text-[10px] font-mono text-stone-500 bg-stone-200/80 px-1 py-0.2 rounded" title="Отключено колонок: ${disabledCount}">-${disabledCount}</span>`;
    }

    if (isActive) {
      tabsHtml += `
        <button type="button" onclick="switchActiveSheet(${idx})" class="px-3 py-1.5 bg-white border border-[#1C1917] text-[#1C1917] font-semibold text-xs shadow-xs transition flex items-center space-x-1.5 cursor-pointer">
          <span class="w-2 h-2 rounded-full bg-emerald-600"></span>
          <span class="truncate max-w-[180px]">${escapeHtml(sheet.name)}</span>
          <span class="text-[10px] font-mono text-[#78716C] bg-[#F0EFEB] px-1.5 py-0.2 rounded">(${rowCountStr})</span>
          ${badgeHtml}
        </button>
      `;
      modalTabsHtml += `
        <button type="button" onclick="switchActiveSheet(${idx})" class="px-3 py-1 bg-white border border-[#1C1917] text-[#1C1917] font-semibold text-xs shadow-xs transition flex items-center space-x-1.5 cursor-pointer">
          <span class="w-2 h-2 rounded-full bg-emerald-600"></span>
          <span class="truncate max-w-[180px]">${escapeHtml(sheet.name)}</span>
          <span class="text-[10px] font-mono text-[#78716C] bg-[#F0EFEB] px-1.5 py-0.2 rounded">(${rowCountStr})</span>
          ${badgeHtml}
        </button>
      `;
    } else {
      tabsHtml += `
        <button type="button" onclick="switchActiveSheet(${idx})" class="px-3 py-1.5 bg-transparent border border-transparent hover:border-[#D6D3D1] hover:bg-white text-[#78716C] hover:text-[#1C1917] text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer">
          <span class="truncate max-w-[180px]">${escapeHtml(sheet.name)}</span>
          <span class="text-[10px] font-mono text-[#A8A29E]">(${rowCountStr})</span>
          ${badgeHtml}
        </button>
      `;
      modalTabsHtml += `
        <button type="button" onclick="switchActiveSheet(${idx})" class="px-3 py-1 bg-transparent border border-transparent hover:border-[#D6D3D1] hover:bg-white text-[#78716C] hover:text-[#1C1917] text-xs font-medium transition flex items-center space-x-1.5 cursor-pointer">
          <span class="truncate max-w-[180px]">${escapeHtml(sheet.name)}</span>
          <span class="text-[10px] font-mono text-[#A8A29E]">(${rowCountStr})</span>
          ${badgeHtml}
        </button>
      `;
    }
  });

  if (list) list.innerHTML = tabsHtml;
  if (modalList) modalList.innerHTML = modalTabsHtml;
}

// Switch currently active preview sheet in multi-sheet workbook
async function switchActiveSheet(idx) {
  if (!currentFileInfo || !currentFileInfo.sheets || !currentFileInfo.sheets[idx]) return;
  if (currentActiveSheetIndex === idx) return;

  currentActiveSheetIndex = idx;
  const sheet = currentFileInfo.sheets[idx];

  // Update current active sheet state
  currentFileInfo.activeSheet = sheet.name;
  currentFileInfo.headers = sheet.headers || [];
  currentFileInfo.entities = sheet.entities || [];
  currentFileInfo.rowCount = sheet.rowCount || 0;
  currentFileInfo.previewRows = sheet.previewRows || [];

  allLoadedRows = sheet.previewRows ? [...sheet.previewRows] : [];
  hasMoreRows = allLoadedRows.length < sheet.rowCount;
  isLoadingMore = false;
  columnWidths = []; // recalculate widths for this sheet

  renderSheetTabs();
  renderEntityCards();
  if (typeof renderTableHeaders === 'function') renderTableHeaders();
  if (typeof renderTableBody === 'function') renderTableBody();
  if (typeof renderModalTableBody === 'function') renderModalTableBody();
  if (typeof updatePreviewCountLabel === 'function') updatePreviewCountLabel();
  if (typeof updateModalPreviewCountLabel === 'function') updateModalPreviewCountLabel();

  // Scroll tables back to top-left
  const scrollBox = document.getElementById('table-scroll-container');
  if (scrollBox) { scrollBox.scrollTop = 0; scrollBox.scrollLeft = 0; }
  const modalScrollBox = document.getElementById('modal-table-scroll-container');
  if (modalScrollBox) { modalScrollBox.scrollTop = 0; modalScrollBox.scrollLeft = 0; }

  // If in expanded modal and few rows loaded, pre-fetch to fill screen
  const modal = document.getElementById('expanded-preview-modal');
  if (modal && !modal.classList.contains('hidden')) {
    if (hasMoreRows && allLoadedRows.length < 30 && typeof loadNextRowBatch === 'function') {
      await loadNextRowBatch();
    }
  }
}

// Toggle Column Modification
function toggleColumn(colIndex, event) {
  if (event) {
    event.stopPropagation();
  }
  const dSet = getActiveDisabledSet();
  if (dSet.has(colIndex)) {
    dSet.delete(colIndex);
  } else {
    dSet.add(colIndex);
  }

  renderSheetTabs();
  renderEntityCards();
  if (typeof renderTableHeaders === 'function') renderTableHeaders();
  if (typeof renderTableBody === 'function') renderTableBody();
  if (typeof renderModalTableBody === 'function') renderModalTableBody();
}

function renderEntityCards() {
  const cardContainer = document.getElementById('entity-cards-container');
  const countBadge = document.getElementById('entity-count-badge');
  const cardsHeader = document.getElementById('entity-cards-header');
  if (!cardContainer) return;

  if (!currentFileInfo) {
    if (cardsHeader) {
      cardsHeader.classList.add('hidden');
      cardsHeader.classList.remove('flex');
    }
    cardContainer.innerHTML = `
      <div id="empty-state-dropzone" class="col-span-full border-2 border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-white p-8 sm:p-10 text-center transition cursor-pointer flex flex-col items-center justify-center gap-2.5 shadow-2xs" onclick="handleSelectFile()">
        <span class="text-3xl">📁</span>
        <div class="font-bold text-sm text-[#1C1917]">Перетащите файл сюда или нажмите для выбора</div>
        <div class="text-xs text-[#78716C] max-w-md">Поддерживаются таблицы Excel (.xlsx), CSV (.csv) и документы Adobe PDF (.pdf). Все данные обрабатываются локально без выхода в сеть.</div>
        <button type="button" class="mt-2 px-4 py-2 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs cursor-pointer">
          Выбрать файл на диске...
        </button>
      </div>
    `;
    if (countBadge) countBadge.textContent = '0 категорий';
    return;
  }

  if (cardsHeader) {
    cardsHeader.classList.remove('hidden');
    cardsHeader.classList.add('flex');
  }

  if (!currentFileInfo.entities || currentFileInfo.entities.length === 0) {
    cardContainer.innerHTML = `
      <div class="col-span-full bg-white border border-[#E8E7E3] p-5 text-xs text-[#78716C] flex items-center space-x-3">
        <span class="text-base text-emerald-600 font-bold">✔</span>
        <div>
          <div class="font-semibold text-[#1C1917] text-sm">В файле не обнаружено персональных данных</div>
          <div class="text-xs text-[#78716C] mt-0.5">В таблице нет колонок ФИО, СНИЛС, ИНН, Паспортов, Телефонов, Почты или Дат. Все исходные колонки будут сохранены без изменений.</div>
        </div>
      </div>
    `;
    if (countBadge) countBadge.textContent = '0 категорий';
    return;
  }

  const disabledSet = getActiveDisabledSet();
  const totalCount = currentFileInfo.entities.length;
  const activeCount = totalCount - disabledSet.size;
  if (countBadge) {
    countBadge.textContent = `${activeCount} из ${totalCount} активных`;
  }

  cardContainer.innerHTML = '';
  currentFileInfo.entities.forEach((ent) => {
    const isDisabled = disabledSet.has(ent.columnIndex);
    const card = document.createElement('div');
    
    card.className = `group relative bg-white border ${isDisabled ? 'border-dashed border-stone-300 opacity-80' : 'border-[#E8E7E3] hover:border-[#1C1917]'} p-4 flex flex-col justify-between shadow-xs transition-all cursor-pointer`;
    card.onclick = () => toggleColumn(ent.columnIndex);

    card.innerHTML = `
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-2 font-mono">
          <span>${currentFileInfo && currentFileInfo.fileType === 'text' ? 'КАТЕГОРИЯ ПДн' : 'КОЛОНКА'} #${ent.columnIndex}</span>
          <button type="button" onclick="toggleColumn(${ent.columnIndex}, event)" class="focus:outline-none flex items-center space-x-1.5 px-2 py-0.5 rounded text-[11px] font-mono font-medium transition cursor-pointer ${isDisabled ? 'bg-stone-100 text-stone-500 hover:bg-stone-200' : 'bg-emerald-50 text-emerald-700 border border-emerald-200 hover:bg-emerald-100'}">
            <span class="w-2 h-2 rounded-full ${isDisabled ? 'bg-stone-400' : 'bg-emerald-500'}"></span>
            <span>${isDisabled ? 'Отключено' : '● Заменено'}</span>
          </button>
        </div>
        <div class="flex items-start justify-between gap-2">
          <div class="font-bold text-sm text-[#1C1917] truncate" title="${escapeHtml(ent.columnName)}">
            ${escapeHtml(ent.columnName)}
          </div>
          <!-- Notion-like Toggle Switch -->
          <button type="button" onclick="toggleColumn(${ent.columnIndex}, event)" title="${isDisabled ? 'Включить деперсонализацию' : 'Отключить деперсонализацию'}" class="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${isDisabled ? 'bg-stone-300' : 'bg-emerald-600'}">
            <span class="pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow-sm ring-0 transition duration-200 ease-in-out ${isDisabled ? 'translate-x-0' : 'translate-x-4'}"></span>
          </button>
        </div>
        <div class="text-xs text-[#78716C] mt-1">${escapeHtml(ent.description)}</div>
      </div>
      <div class="mt-4 pt-3 border-t border-[#F0EFEB] text-xs font-mono -mx-4 -mb-4 p-2.5 transition-colors ${isDisabled ? 'text-[#78716C] bg-[#F5F5F4]' : 'text-[#065F46] bg-[#ECFDF5]'}">
        ${isDisabled ? '○ Исходные данные без изменений' : `➜ ${escapeHtml(ent.action)}`}
      </div>
    `;
    cardContainer.appendChild(card);
  });
}

async function handleSelectFile() {
  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectFile) {
      const selected = await window.go.main.App.SelectFile();
      if (selected) {
        await loadFile(selected);
      }
    } else {
      // Demo browser fallback input
      const input = document.createElement('input');
      input.type = 'file';
      input.accept = '.csv,.tsv,.xlsx,.xlsm,.xlsb,.xls,.ods,.docx,.odt,.pdf';
      input.onchange = (e) => {
        const f = e.target.files[0];
        if (f) {
          alert("В браузере выбран файл: " + f.name + ". В десктопном приложении файл обрабатывается локально через движок Go.");
        }
      };
      input.click();
    }
  } catch (err) {
    alert("Ошибка при выборе файла: " + err);
  }
}

async function loadFile(path) {
  try {
    if (!window.go || !window.go.main || !window.go.main.App || !window.go.main.App.InspectFile) {
      return;
    }
    const info = await window.go.main.App.InspectFile(path);
    if (!info) return;

    currentFilePath = path;
    currentFileInfo = info;
    currentActiveSheetIndex = 0;
    disabledColumnsBySheet = {};
    if (info.sheets && info.sheets.length > 0) {
      info.sheets.forEach(s => {
        disabledColumnsBySheet[s.name] = new Set();
      });
    } else {
      disabledColumnsBySheet[''] = new Set();
    }
    columnWidths = [];

    allLoadedRows = info.previewRows ? [...info.previewRows] : [];
    hasMoreRows = info.fileType === 'table' && allLoadedRows.length < info.rowCount;
    isLoadingMore = false;

    // Update file titles
    const titleEl = document.getElementById('file-name-title');
    if (titleEl) titleEl.textContent = info.name;
    const pdfTitleEl = document.getElementById('pdf-file-name-title');
    if (pdfTitleEl) pdfTitleEl.textContent = info.name;

    // Update meta bar
    const metaEl = document.getElementById('file-meta-bar');
    if (metaEl) {
      let units = '';
      if (info.fileType === 'pdf') {
        units = `${info.pageCount} стр.`;
      } else if (info.fileType === 'text') {
        units = `${info.paragraphCount || info.rowCount} абз.`;
      } else if (info.sheets && info.sheets.length > 1) {
        const totalR = info.sheets.reduce((acc, s) => acc + (s.rowCount || 0), 0);
        units = `${info.sheets.length} листов (${totalR.toLocaleString('ru-RU')} строк)`;
      } else {
        units = `${info.rowCount.toLocaleString('ru-RU')} строк`;
      }
      metaEl.innerHTML = `<span>Файл загружен</span><span>•</span><span>${units}</span><span>•</span><span>${info.extension.toUpperCase()}</span>`;
    }

    // Update description
    const entityCount = info.entities ? info.entities.length : 0;
    const descEl = document.getElementById('file-desc-text');
    if (descEl) {
      if (info.fileType === 'text') {
        descEl.textContent = `Документ содержит ${info.paragraphCount || info.rowCount} абзацев. Найдено ${entityCount} категорий ПДн. Форматирование, шрифты и стили документа сохраняются без изменений.`;
      } else if (info.sheets && info.sheets.length > 1) {
        const totalEntities = info.sheets.reduce((acc, s) => acc + (s.entities ? s.entities.length : 0), 0);
        descEl.textContent = `Книга Excel содержит ${info.sheets.length} листа. Всего найдено ${totalEntities} категорий ПДн. Переключайтесь между вкладками листов для настройки колонок каждого листа.`;
      } else {
        descEl.textContent = `Обнаружено категорий ПДн: ${entityCount}. Чувствительные поля подготовлены к безопасной деперсонализации по 152-ФЗ.`;
      }
    }

    // Update action buttons in hero
    const btnOpen = document.getElementById('btn-open-file-hero');
    const btnChange = document.getElementById('btn-change-file-hero');
    const btnSave = document.getElementById('btn-save');
    if (btnOpen) btnOpen.classList.add('hidden');
    if (btnChange) btnChange.classList.remove('hidden');
    if (btnSave) btnSave.classList.remove('hidden');

    // Update button badge
    const btnBadge = document.querySelector('#btn-save .font-mono');
    if (btnBadge && info.extension) {
      btnBadge.textContent = `(.${info.extension.toLowerCase()})`;
    }

    // Render sheet tabs if multi-sheet
    renderSheetTabs();

    // Render Entity Cards
    renderEntityCards();

    // Render Table, Text or PDF
    if (info.fileType === 'table') {
      if (typeof renderTableHeaders === 'function') renderTableHeaders();
      if (typeof renderTableBody === 'function') renderTableBody();
      if (typeof renderModalTableBody === 'function') renderModalTableBody();
      if (typeof setDocMode === 'function') setDocMode('table');
    } else if (info.fileType === 'text') {
      renderTextSnippets();
      if (typeof setDocMode === 'function') setDocMode('text');
    } else if (info.fileType === 'pdf') {
      const pdfSnippetEl = document.getElementById('pdf-snippet-view');
      if (pdfSnippetEl && info.pdfRedactedSnippet) {
        pdfSnippetEl.innerHTML = escapeHtml(info.pdfRedactedSnippet).replace(/\[\s*ПДн СКРЫТО\s*\]/g, '<span class="redaction-bar">[ ПДн СКРЫТО ]</span>');
      }
      if (typeof setDocMode === 'function') setDocMode('pdf');
    }
  } catch (err) {
    alert("Ошибка инспекции файла: " + err);
  }
}

function setTextViewMode(mode) {
  currentTextViewMode = mode;
  const btnAfter = document.getElementById('btn-text-show-after');
  const btnBefore = document.getElementById('btn-text-show-before');

  if (mode === 'after') {
    if (btnAfter) btnAfter.className = 'px-3 py-1 bg-[#1C1917] text-white font-semibold transition cursor-pointer';
    if (btnBefore) btnBefore.className = 'px-3 py-1 text-[#78716C] hover:text-[#1C1917] transition cursor-pointer';
  } else {
    if (btnBefore) btnBefore.className = 'px-3 py-1 bg-[#1C1917] text-white font-semibold transition cursor-pointer';
    if (btnAfter) btnAfter.className = 'px-3 py-1 text-[#78716C] hover:text-[#1C1917] transition cursor-pointer';
  }
  renderTextSnippets();
}

function renderTextSnippets() {
  const container = document.getElementById('text-snippets-container');
  const countLabel = document.getElementById('text-snippet-count-label');
  if (!container) return;

  const snippets = (currentFileInfo && currentFileInfo.textSnippets) ? currentFileInfo.textSnippets : [];
  if (countLabel) {
    countLabel.textContent = `(${snippets.length} фрагментов)`;
  }

  if (snippets.length === 0) {
    container.innerHTML = `
      <div class="text-center py-8 text-xs text-[#78716C] font-mono">
        В документе не обнаружено фрагментов персональных данных для замены
      </div>
    `;
    return;
  }

  const typeLabels = {
    'FIO': 'ФИО',
    'SNILS': 'СНИЛС',
    'PASSPORT': 'Паспорт РФ',
    'PHONE': 'Телефон',
    'INN': 'ИНН',
    'BANK_CARD': 'Банковская карта',
    'DATE': 'Дата',
    'EMAIL': 'Email'
  };

  let html = '';
  snippets.forEach((snip, idx) => {
    const typeName = typeLabels[snip.type] || snip.type || 'ПДн';
    const isAfter = currentTextViewMode === 'after';

    html += `
      <div class="pt-3.5 first:pt-0">
        <div class="flex items-center justify-between mb-1.5 font-mono text-[11px]">
          <div class="flex items-center space-x-2">
            <span class="px-2 py-0.5 bg-[#FAF9F5] border border-[#E8E7E3] text-[#1C1917] font-semibold text-[10px] uppercase">
              ${escapeHtml(typeName)}
            </span>
            <span class="text-[#78716C]">Фрагмент #${idx + 1}</span>
          </div>
          <span class="text-[10px] text-[#A8A29E]">${isAfter ? 'Обезличенный вид' : 'Исходный вид'}</span>
        </div>

        <div class="bg-white border border-[#E8E7E3] p-3 text-xs leading-relaxed text-[#1C1917] font-sans">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
            <div class="space-y-1 flex-1">
              <div class="text-[11px] text-[#78716C] font-mono">До деперсонализации:</div>
              <div class="p-2 bg-rose-50/50 border border-rose-200/60 text-rose-900 rounded-none font-mono text-[11px]">
                ${escapeHtml(snip.original)}
              </div>
            </div>
            <div class="text-center text-[#78716C] font-bold text-xs shrink-0 self-center px-1">➔</div>
            <div class="space-y-1 flex-1">
              <div class="text-[11px] text-[#78716C] font-mono">После деперсонализации:</div>
              <div class="p-2 bg-emerald-50/60 border border-emerald-200 text-emerald-950 rounded-none font-mono text-[11px]">
                ${escapeHtml(snip.anonymized)}
              </div>
            </div>
          </div>
        </div>
      </div>
    `;
  });

  container.innerHTML = html;
}

function getFormatDisplayName(fmt) {
  switch ((fmt || '').toLowerCase()) {
    case 'xlsx': return 'Таблица Microsoft Excel (.xlsx)';
    case 'ods': return 'Таблица OpenDocument Spreadsheet (.ods)';
    case 'csv': return 'Текстовая таблица CSV (.csv)';
    case 'docx': return 'Документ Microsoft Word (.docx)';
    case 'odt': return 'Документ OpenDocument Text (.odt)';
    case 'pdf': return 'Документ PDF (.pdf)';
    default: return (fmt || '').toUpperCase();
  }
}

function updateFormatHint() {
  const select = document.getElementById('exportFormatSelect');
  const hint = document.getElementById('save-format-hint');
  if (!select || !hint || !currentFileInfo) return;

  const val = (select.value || '').toLowerCase();
  const origExt = (currentFileInfo.extension || '').toLowerCase();

  if (origExt === 'xls' || origExt === 'xlsb') {
    hint.textContent = 'Бинарный формат Excel конвертируется в современный открытый стандарт .xlsx без потери данных.';
  } else if (val === origExt) {
    hint.textContent = 'Формат файла сохраняется. Все стили, структура и шрифты остаются исходными.';
  } else {
    hint.textContent = `Данные будут сохранены и преобразованы в формат .${val}.`;
  }
}

function onExportFormatChange() {
  updateFormatHint();
}

// ==================== SAVE OPTIONS & PROCESSING ====================

function toggleSaveKeyPasswordVisibility() {
  const chk = document.getElementById('chk-create-key');
  const wrap = document.getElementById('save-key-password-wrap');
  if (wrap) {
    if (chk && chk.checked) {
      wrap.classList.remove('hidden');
    } else {
      wrap.classList.add('hidden');
    }
  }
}

function closeSaveOptionsModal() {
  const modal = document.getElementById('save-options-modal');
  if (modal) modal.classList.add('hidden');
}

async function handleSaveProcessedFile() {
  if (!currentFilePath) {
    alert("Пожалуйста, сначала откройте файл для обработки («Открыть файл...»)");
    return;
  }

  // If PDF, save directly without .depmap option (PDF uses irreversible blackout redaction)
  if (currentFileInfo && currentFileInfo.fileType === 'pdf') {
    await executeSaveFileDirect(false, "");
    return;
  }

  // Populate format selector
  const formatSelect = document.getElementById('exportFormatSelect');
  if (formatSelect && currentFileInfo) {
    formatSelect.innerHTML = '';
    const supported = (currentFileInfo.supportedExportFormats && currentFileInfo.supportedExportFormats.length > 0)
      ? currentFileInfo.supportedExportFormats
      : [currentFileInfo.extension || 'xlsx'];
    supported.forEach(fmt => {
      const opt = document.createElement('option');
      opt.value = fmt;
      opt.textContent = getFormatDisplayName(fmt);
      formatSelect.appendChild(opt);
    });
    formatSelect.value = supported[0];
    updateFormatHint();
  }

  // For tables and text docs, show Save Options Modal (format export, key generation & password)
  const modal = document.getElementById('save-options-modal');
  const fileNameEl = document.getElementById('save-modal-file-name');
  const fileMetaEl = document.getElementById('save-modal-file-meta');
  if (fileNameEl && currentFileInfo) fileNameEl.textContent = currentFileInfo.name || 'data';
  if (fileMetaEl && currentFileInfo) {
    if (currentFileInfo.sheets && currentFileInfo.sheets.length > 1) {
      const totalR = currentFileInfo.sheets.reduce((acc, s) => acc + (s.rowCount || 0), 0);
      fileMetaEl.textContent = `${currentFileInfo.sheets.length} листов • ${totalR.toLocaleString('ru-RU')} строк`;
    } else if (currentFileInfo.fileType === 'text') {
      fileMetaEl.textContent = `${(currentFileInfo.paragraphCount || currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} абзацев`;
    } else {
      fileMetaEl.textContent = `${(currentFileInfo.rowCount || 0).toLocaleString('ru-RU')} строк`;
    }
  }
  if (modal) modal.classList.remove('hidden');
}

async function confirmAndSaveProcessedFile() {
  const chk = document.getElementById('chk-create-key');
  const pwdInput = document.getElementById('save-key-password');
  const createKey = chk ? chk.checked : true;
  const keyPassword = pwdInput ? pwdInput.value : '';

  closeSaveOptionsModal();
  await executeSaveFileDirect(createKey, keyPassword);
}

async function executeSaveFileDirect(createKey, keyPassword) {
  try {
    let destPath = "";
    const formatSelect = document.getElementById('exportFormatSelect');
    let ext = currentFileInfo ? currentFileInfo.extension : "xlsx";
    if (formatSelect && formatSelect.value) {
      ext = formatSelect.value;
    } else if (ext === "xls" || ext === "xlsb") {
      ext = "xlsx";
    }
    const baseName = currentFileInfo ? currentFileInfo.name.replace(/\.[^/.]+$/, "") : "data";
    const defaultName = `${baseName}_anonymized.${ext}`;

    if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveFileDialog) {
      destPath = await window.go.main.App.SaveFileDialog(defaultName);
      if (!destPath) {
        // User cancelled save dialog
        return;
      }
    }

    // Open progress modal with true total rows across all sheets
    let totalRowsToProcess = currentFileInfo ? currentFileInfo.rowCount : 0;
    if (currentFileInfo && currentFileInfo.sheets && currentFileInfo.sheets.length > 1) {
      totalRowsToProcess = currentFileInfo.sheets.reduce((acc, s) => acc + (s.rowCount || 0), 0);
    }
    if (typeof openProgressModal === 'function') {
      openProgressModal(totalRowsToProcess, 'depersonalize');
    }

    // Build per-sheet disabled column map
    const disabledBySheetMap = {};
    for (const [sName, colSet] of Object.entries(disabledColumnsBySheet)) {
      if (colSet && colSet.size > 0) {
        disabledBySheetMap[sName] = Array.from(colSet);
      }
    }
    const globalDisabledArr = Array.from(getActiveDisabledSet());

    if (window.go && window.go.main && window.go.main.App) {
      let res = null;
      if (window.go.main.App.ProcessFileWithSheets) {
        res = await window.go.main.App.ProcessFileWithSheets(currentFilePath, destPath, currentSalt, globalDisabledArr, disabledBySheetMap, createKey, keyPassword, "");
      } else if (window.go.main.App.ProcessFileTo) {
        res = await window.go.main.App.ProcessFileTo(currentFilePath, destPath, currentSalt, globalDisabledArr, createKey, keyPassword, "");
      }

      if (res && res.success) {
        if (typeof onProcessingCompleted === 'function') {
          onProcessingCompleted(res, 'depersonalize');
        }
      } else {
        alert("Ошибка при обработке файла: " + (res && res.error ? res.error : "неизвестная ошибка"));
        if (typeof closeProgressModal === 'function') closeProgressModal();
      }
    } else {
      // Demo fallback
      setTimeout(() => {
        if (typeof onProcessingCompleted === 'function') {
          onProcessingCompleted({
            success: true,
            outputPath: destPath || defaultName,
            keyPath: createKey ? (destPath || defaultName) + ".depmap" : "",
            rowsCount: currentFileInfo ? currentFileInfo.rowCount : 100,
            durationMs: 350
          }, 'depersonalize');
        }
      }, 800);
    }
  } catch (err) {
    alert("Ошибка деперсонализации: " + err);
    if (typeof closeProgressModal === 'function') closeProgressModal();
  }
}
