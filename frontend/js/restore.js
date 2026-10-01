/**
 * restore.js — Логика обратной персонализации (деобезличивания) через ключ .depmap
 */

async function handleSelectRestoreInput() {
  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectFile) {
      const selected = await window.go.main.App.SelectFile();
      if (selected) {
        await loadRestoreDataFile(selected);
      }
    } else {
      // Demo fallback input
      const input = document.createElement('input');
      input.type = 'file';
      input.accept = '.csv,.tsv,.xlsx,.xlsm,.xlsb,.xls,.ods,.docx,.odt';
      input.onchange = (e) => {
        const f = e.target.files[0];
        if (f) {
          loadRestoreDataFile(f.name);
        }
      };
      input.click();
    }
  } catch (err) {
    alert("Ошибка при выборе файла данных: " + err);
  }
}

async function loadRestoreDataFile(path) {
  if (!path) return;
  const ext = path.split('.').pop().toLowerCase();
  const validExts = ['xlsx', 'csv', 'tsv', 'xlsm', 'xlsb', 'xls', 'ods', 'docx', 'odt'];
  if (!validExts.includes(ext)) {
    alert("Пожалуйста, выберите поддерживаемый файл данных (.xlsx, .ods, .csv, .docx, .odt)");
    return;
  }

  restoreInputPath = path;
  const dropzone = document.getElementById('restore-data-dropzone');
  const infoCard = document.getElementById('restore-data-info');
  const nameEl = document.getElementById('restore-data-name');
  const extEl = document.getElementById('restore-data-ext');
  const pathEl = document.getElementById('restore-data-path');
  const sizeEl = document.getElementById('restore-data-size');

  if (dropzone) dropzone.classList.add('hidden');
  if (infoCard) infoCard.classList.remove('hidden');

  const baseName = path.split('/').pop().split('\\').pop();
  if (nameEl) nameEl.textContent = baseName;
  if (extEl) extEl.textContent = ext.toUpperCase();
  if (pathEl) pathEl.textContent = path;

  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.InspectFile) {
      const info = await window.go.main.App.InspectFile(path);
      if (info) {
        restoreInputInfo = info;
        const sizeKb = Math.round((info.size || 0) / 1024);
        const countStr = info.paragraphCount ? `${info.paragraphCount.toLocaleString('ru-RU')} абз.` : `${info.rowCount.toLocaleString('ru-RU')} строк`;
        if (sizeEl) sizeEl.textContent = `${countStr} • ${sizeKb} КБ`;
      }
    }
  } catch (e) {
    console.warn("Could not inspect restore data file:", e);
  }

  updateRestoreButtonState();
}

async function handleSelectRestoreKey() {
  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.SelectKeyFile) {
      const selected = await window.go.main.App.SelectKeyFile();
      if (selected) {
        await loadRestoreKeyFile(selected);
      }
    } else {
      // Demo fallback input
      const input = document.createElement('input');
      input.type = 'file';
      input.accept = '.depmap';
      input.onchange = (e) => {
        const f = e.target.files[0];
        if (f) {
          loadRestoreKeyFile(f.name);
        }
      };
      input.click();
    }
  } catch (err) {
    alert("Ошибка при выборе файла ключа: " + err);
  }
}

async function loadRestoreKeyFile(path) {
  if (!path) return;
  restoreKeyPath = path;
  await inspectRestoreKey(path);
}

async function inspectRestoreKey(path) {
  if (!path) return;
  const dropzone = document.getElementById('restore-key-dropzone');
  const infoCard = document.getElementById('restore-key-info');
  const nameEl = document.getElementById('restore-key-name');
  const badgeEl = document.getElementById('restore-key-status-badge');
  const pathEl = document.getElementById('restore-key-path');
  const metaEl = document.getElementById('restore-key-meta');
  const errorEl = document.getElementById('restore-key-error');

  if (dropzone) dropzone.classList.add('hidden');
  if (infoCard) infoCard.classList.remove('hidden');

  const baseName = path.split('/').pop().split('\\').pop();
  if (nameEl) nameEl.textContent = baseName;
  if (pathEl) pathEl.textContent = path;

  const pwdInput = document.getElementById('restore-key-password');
  restorePassword = pwdInput ? pwdInput.value : '';

  try {
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.InspectKeyFile) {
      const kInfo = await window.go.main.App.InspectKeyFile(path, restorePassword);
      restoreKeyInfo = kInfo;

      if (kInfo && kInfo.valid) {
        if (badgeEl) {
          badgeEl.textContent = '✔ Валиден';
          badgeEl.className = 'text-[10px] px-1.5 py-0.5 font-medium bg-emerald-50 text-emerald-700 border border-emerald-200';
        }
        if (metaEl) {
          metaEl.textContent = `Исходный: ${kInfo.originalFileName || 'данные'} • Записей: ${kInfo.recordsCount.toLocaleString('ru-RU')} • ${kInfo.createdAt}`;
          metaEl.classList.remove('hidden');
        }
        if (errorEl) errorEl.classList.add('hidden');
      } else {
        if (badgeEl) {
          badgeEl.textContent = '⚠ Ошибка / Пароль';
          badgeEl.className = 'text-[10px] px-1.5 py-0.5 font-medium bg-rose-50 text-rose-700 border border-rose-200';
        }
        if (metaEl) metaEl.classList.add('hidden');
        if (errorEl) {
          errorEl.textContent = kInfo && kInfo.error ? kInfo.error : 'Неверный формат ключа или требуется пароль расшифровки';
          errorEl.classList.remove('hidden');
        }
      }
    } else {
      // Browser demo fallback
      restoreKeyInfo = { valid: true, recordsCount: 120, originalFileName: 'data.xlsx', createdAt: '2026-09-30 12:00:00' };
      if (badgeEl) {
        badgeEl.textContent = '✔ Валиден (демо)';
        badgeEl.className = 'text-[10px] px-1.5 py-0.5 font-medium bg-emerald-50 text-emerald-700 border border-emerald-200';
      }
      if (metaEl) metaEl.textContent = `Исходный: data.xlsx • Записей: 120 • 2026-09-30 12:00:00`;
      if (errorEl) errorEl.classList.add('hidden');
    }
  } catch (err) {
    restoreKeyInfo = { valid: false, error: String(err) };
    if (badgeEl) {
      badgeEl.textContent = '⚠ Ошибка';
      badgeEl.className = 'text-[10px] px-1.5 py-0.5 font-medium bg-rose-50 text-rose-700 border border-rose-200';
    }
    if (metaEl) metaEl.classList.add('hidden');
    if (errorEl) {
      errorEl.textContent = String(err);
      errorEl.classList.remove('hidden');
    }
  }

  updateRestoreButtonState();
}

let restorePwdDebounce = null;
function handleRestorePasswordChange() {
  clearTimeout(restorePwdDebounce);
  restorePwdDebounce = setTimeout(() => {
    if (restoreKeyPath) {
      inspectRestoreKey(restoreKeyPath);
    }
  }, 300);
}

function updateRestoreButtonState() {
  const isReady = !!(restoreInputPath && restoreKeyInfo && restoreKeyInfo.valid);
  const btnMain = document.getElementById('btn-restore-start');
  if (btnMain) btnMain.disabled = !isReady;

  const statusEl = document.getElementById('restore-status-text');
  const statusDot = document.getElementById('restore-status-dot');
  if (statusEl && statusDot) {
    if (isReady) {
      statusDot.className = 'w-2 h-2 rounded-full bg-emerald-600';
      statusEl.textContent = '✔ Готово к восстановлению данных';
      statusEl.className = 'text-[#065F46] font-semibold';
    } else if (!restoreInputPath && !restoreKeyPath) {
      statusDot.className = 'w-2 h-2 rounded-full bg-stone-400';
      statusEl.textContent = 'Шаг 1 из 3: выберите файл данных';
      statusEl.className = 'text-[#78716C]';
    } else if (!restoreInputPath) {
      statusDot.className = 'w-2 h-2 rounded-full bg-amber-500';
      statusEl.textContent = 'Шаг 1: выберите файл данных';
      statusEl.className = 'text-[#78716C]';
    } else if (!restoreKeyPath) {
      statusDot.className = 'w-2 h-2 rounded-full bg-amber-500';
      statusEl.textContent = 'Шаг 2: выберите ключ (.depmap)';
      statusEl.className = 'text-[#78716C]';
    } else if (restoreKeyInfo && !restoreKeyInfo.valid) {
      statusDot.className = 'w-2 h-2 rounded-full bg-rose-500';
      statusEl.textContent = 'Ошибка: неверный пароль ключа';
      statusEl.className = 'text-rose-700 font-semibold';
    }
  }
}

async function executeRestoreFile() {
  if (!restoreInputPath || !restoreKeyPath || !restoreKeyInfo || !restoreKeyInfo.valid) {
    alert("Пожалуйста, укажите файл с данными и валидный ключ восстановления (.depmap)");
    return;
  }

  try {
    const ext = restoreInputPath.split('.').pop() || "xlsx";
    let baseName = restoreInputPath.split('/').pop().split('\\').pop() || "data";
    baseName = baseName.replace(/\.[^/.]+$/, "");
    if (baseName.endsWith("_anonymized")) {
      baseName = baseName.replace(/_anonymized$/, "");
    }
    const defaultName = `${baseName}_restored.${ext}`;

    let destPath = "";
    if (window.go && window.go.main && window.go.main.App && window.go.main.App.SaveFileDialog) {
      destPath = await window.go.main.App.SaveFileDialog(defaultName);
      if (!destPath) return; // User cancelled save dialog
    }

    if (typeof openProgressModal === 'function') {
      openProgressModal(0, 'restore');
    }

    if (window.go && window.go.main && window.go.main.App && window.go.main.App.RestoreFileTo) {
      const res = await window.go.main.App.RestoreFileTo(restoreInputPath, restoreKeyPath, restorePassword, destPath);
      if (res && res.success) {
        if (typeof onProcessingCompleted === 'function') {
          onProcessingCompleted(res, 'restore');
        }
      } else {
        alert("Ошибка при восстановлении данных: " + (res && res.error ? res.error : "неизвестная ошибка"));
        if (typeof closeProgressModal === 'function') closeProgressModal();
      }
    } else {
      // Demo fallback
      setTimeout(() => {
        if (typeof onProcessingCompleted === 'function') {
          onProcessingCompleted({
            success: true,
            outputPath: destPath || defaultName,
            restoredCells: 450,
            durationMs: 180
          }, 'restore');
        }
      }, 700);
    }
  } catch (err) {
    alert("Ошибка восстановления: " + err);
    if (typeof closeProgressModal === 'function') closeProgressModal();
  }
}
