<script lang="ts">
  import KeyInfoCard from './KeyInfoCard.svelte';
  import { restoreState } from '../../stores/restoreState.svelte';
  import { appState } from '../../stores/appState.svelte';
  import { SaveFileDialog, RestoreFileTo } from '../../api/wails';

  const dataFileName = $derived(
    restoreState.dataPath.split('/').pop()?.split('\\').pop() || 'data.xlsx'
  );

  const dataFileExt = $derived(
    restoreState.dataPath.split('.').pop()?.toUpperCase() || 'XLSX'
  );

  function formatFileSize(bytes: number): string {
    if (!bytes || bytes <= 0) return '';
    if (bytes < 1024) return `${bytes} Б`;
    if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} КБ`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} МБ`;
  }

  const dataStatsText = $derived.by(() => {
    const info = restoreState.dataInfo;
    if (!info) {
      return restoreState.isLoadingData ? 'Загрузка информации...' : 'Готов к обработке';
    }
    const sizeStr = info.size ? formatFileSize(info.size) : '';
    const countStr = info.paragraphCount
      ? `${info.paragraphCount.toLocaleString('ru-RU')} абз.`
      : `${(info.rowCount || 0).toLocaleString('ru-RU')} строк`;
    return sizeStr ? `${countStr} ▪ ${sizeStr}` : countStr;
  });

  const isReady = $derived(
    !!(restoreState.dataPath && restoreState.keyInfo && restoreState.keyInfo.valid)
  );

  let showPassword = $state(false);

  const isPasswordError = $derived(
    !!(restoreState.keyPath && restoreState.keyInfo && !restoreState.keyInfo.valid && restoreState.password)
  );

  const isKeyValid = $derived(
    !!(restoreState.keyInfo && restoreState.keyInfo.valid)
  );

  const statusInfo = $derived.by(() => {
    if (isReady) {
      return {
        dotClass: 'w-2 h-2 rounded-none bg-emerald-600',
        text: '✔ Готово к восстановлению данных',
        textClass: 'text-[#065F46] font-semibold'
      };
    }
    if (!restoreState.dataPath && !restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-none bg-stone-400',
        text: 'Шаг 1 из 3: выберите файл данных',
        textClass: 'text-[#78716C]'
      };
    }
    if (!restoreState.dataPath) {
      return {
        dotClass: 'w-2 h-2 rounded-none bg-amber-500',
        text: 'Шаг 1: выберите файл данных',
        textClass: 'text-[#78716C]'
      };
    }
    if (!restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-none bg-amber-500',
        text: 'Шаг 2: выберите ключ (.depmap)',
        textClass: 'text-[#78716C]'
      };
    }
    if (restoreState.keyInfo && !restoreState.keyInfo.valid) {
      if (!restoreState.password) {
        return {
          dotClass: 'w-2 h-2 rounded-none bg-amber-500',
          text: 'Шаг 3: введите пароль ключа',
          textClass: 'text-[#78716C]'
        };
      }
      return {
        dotClass: 'w-2 h-2 rounded-none bg-rose-500',
        text: 'Ошибка: неверный пароль ключа',
        textClass: 'text-rose-700 font-semibold'
      };
    }
    return {
      dotClass: 'w-2 h-2 rounded-none bg-stone-400',
      text: 'Шаг 1 из 3: выберите файл данных',
      textClass: 'text-[#78716C]'
    };
  });

  async function executeRestore() {
    if (
      !restoreState.dataPath ||
      !restoreState.keyPath ||
      !restoreState.keyInfo ||
      !restoreState.keyInfo.valid
    ) {
      appState.showToast(
        'Пожалуйста, укажите файл с данными и валидный ключ восстановления (.depmap)',
        'warning'
      );
      return;
    }

    const ext = restoreState.dataPath.split('.').pop() || 'xlsx';
    let baseName = restoreState.dataPath.split('/').pop()?.split('\\').pop() || 'data';
    baseName = baseName.replace(/\.[^/.]+$/, '');
    if (baseName.endsWith('_anonymized')) {
      baseName = baseName.replace(/_anonymized$/, '');
    }
    const defaultName = `${baseName}_restored.${ext}`;

    try {
      const destPath = await SaveFileDialog(defaultName);
      if (!destPath) {
        // User cancelled dialog
        return;
      }

      let totalRows = restoreState.dataInfo?.rowCount || 0;
      if (restoreState.dataInfo?.sheets && restoreState.dataInfo.sheets.length > 1) {
        totalRows = restoreState.dataInfo.sheets.reduce(
          (acc, s) => acc + (s.rowCount || 0),
          0
        );
      }

      appState.startProgress('Восстановление данных...');
      if (totalRows > 0) {
        appState.updateProgress(0, 0, totalRows);
      }

      const res = await RestoreFileTo(
        restoreState.dataPath,
        restoreState.keyPath,
        restoreState.password,
        destPath
      );

      if (res && res.success) {
        appState.showSuccess(res);
      } else {
        const errMsg = res?.error || 'Неизвестная ошибка восстановления';
        appState.finishProgress();
        appState.showToast(`Ошибка при восстановлении данных: ${errMsg}`, 'error');
      }
    } catch (err: any) {
      console.error('Failed to restore file:', err);
      appState.finishProgress();
      const errStr = String(err);
      if (errStr.includes('context canceled') || errStr.includes('canceled') || errStr.includes('отменена')) {
        appState.showToast('Операция отменена', 'info');
      } else {
        appState.showToast(`Ошибка восстановления: ${err}`, 'error');
      }
    }
  }
</script>

<div id="restoration-view" class="flex flex-col gap-6 pb-12">
  <!-- Header Banner -->
  <div class="flex flex-col md:flex-row md:items-center justify-between gap-4">
    <div>
      <h2 class="text-2xl font-bold tracking-tight text-[#1C1917]">
        Восстановление исходных данных
      </h2>
    </div>

    <!-- Live Step Status -->
    <div
      id="restore-hero-status"
      class="flex items-center space-x-2 text-xs font-sans px-4 py-2 bg-[#FAF9F5] border border-[#E8E7E3]"
    >
      <span id="restore-status-dot" class={statusInfo.dotClass}></span>
      <span id="restore-status-text" class={statusInfo.textClass}>{statusInfo.text}</span>
    </div>
  </div>

  <!-- 3-Step Selection Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6 items-stretch">
    <!-- Step 1: Data File -->
    <div class="bg-white border border-[#E8E7E3] p-5 shadow-xs flex flex-col min-h-[250px]">
      <div class="flex items-center justify-between text-xs text-[#78716C] mb-4 h-6 shrink-0">
        <span
          class="font-bold text-[#1C1917] font-sans"
          title="Поддерживаемые форматы: XLSX, XLSB, ODS, XODS, XLS, CSV, DOCX, ODT, TXT"
        >
          ШАГ 1. ФАЙЛ ДАННЫХ
        </span>
      </div>

      <div class="flex-1 flex flex-col">
        {#if !restoreState.dataPath}
          <div
            id="restore-data-dropzone"
            class="flex-1 flex flex-col items-center justify-center p-6 text-center border border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-stone-50 transition cursor-pointer select-none group"
            role="button"
            tabindex="0"
            onclick={() => restoreState.selectDataFile()}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') restoreState.selectDataFile(); }}
            ondragover={(e) => { e.preventDefault(); if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'; }}
            ondrop={(e) => {
              e.preventDefault();
              const files = e.dataTransfer?.files;
              if (files && files.length > 0 && files[0]) {
                const path = (files[0] as any)?.path;
                if (path && (path.includes('/') || path.includes('\\'))) {
                  restoreState.loadDataFile(path);
                }
              }
            }}
          >
            <div class="text-3xl mb-1.5 select-none transition group-hover:scale-105">📄</div>
            <div class="font-semibold text-xs text-[#1C1917]">Выберите файл с данными</div>
            <div class="text-[11px] text-[#A8A29E] mt-0.5 mb-3">или перетащите сюда</div>
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                restoreState.selectDataFile();
              }}
              class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] group-hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-2xs hover:shadow-xs transition cursor-pointer"
            >
              Выбрать файл
            </button>
          </div>
        {:else}
          <div id="restore-data-info" class="flex-1 flex flex-col justify-between p-4 bg-[#FAF9F5] border border-[#E8E7E3] text-xs font-sans">
            <div>
              <div class="flex items-start justify-between gap-2 mb-2">
                <div class="flex items-center gap-2 min-w-0">
                  <span class="text-base shrink-0 select-none">📄</span>
                  <span class="font-bold text-[#1C1917] truncate font-sans" id="restore-data-name" title={dataFileName}>
                    {dataFileName}
                  </span>
                </div>
                <div class="flex items-center gap-1.5 shrink-0">
                  <span class="text-[10px] font-mono bg-emerald-50 text-emerald-700 border border-emerald-200 px-1.5 py-0.5" id="restore-data-ext">
                    {dataFileExt}
                  </span>
                  <button
                    type="button"
                    id="btn-remove-data-file"
                    onclick={() => restoreState.clearDataFile()}
                    class="text-[#A8A29E] hover:text-rose-600 hover:bg-rose-50 p-1 rounded transition cursor-pointer"
                    title="Удалить прикрепленный файл"
                    aria-label="Удалить прикрепленный файл"
                  >
                    <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
                    </svg>
                  </button>
                </div>
              </div>
              <div class="text-[11px] text-[#78716C] break-all line-clamp-2 mb-2 font-sans" id="restore-data-path" title={restoreState.dataPath}>
                {restoreState.dataPath}
              </div>
            </div>

            <div class="pt-3 border-t border-[#E8E7E3] flex items-center justify-between gap-3 font-sans">
              <span class="text-[11px] text-[#57534E] truncate min-w-0" id="restore-data-size">
                {dataStatsText}
              </span>
              <div class="flex items-center gap-2 shrink-0">
                <button
                  type="button"
                  onclick={() => restoreState.selectDataFile()}
                  class="text-[11px] text-[#1C1917] hover:underline font-semibold cursor-pointer whitespace-nowrap"
                >
                  Сменить
                </button>
                <span class="text-[#D6D3D1]">|</span>
                <button
                  type="button"
                  id="btn-clear-data-text"
                  onclick={() => restoreState.clearDataFile()}
                  class="text-[11px] text-[#78716C] hover:text-rose-600 hover:underline cursor-pointer whitespace-nowrap"
                >
                  Удалить
                </button>
              </div>
            </div>
          </div>
        {/if}
      </div>
    </div>

    <!-- Step 2: Key File -->
    <div class="bg-white border border-[#E8E7E3] p-5 shadow-xs flex flex-col min-h-[250px]">
      <div class="flex items-center justify-between text-xs text-[#78716C] mb-4 h-6 shrink-0">
        <span class="font-bold text-[#1C1917] font-sans">ШАГ 2. ФАЙЛ-КЛЮЧ</span>
        <span class="text-[10px] bg-[#FAF9F5] border border-[#E8E7E3] px-2 py-0.5 whitespace-nowrap text-[#78716C] font-mono">.DEPMAP</span>
      </div>

      <div class="flex-1 flex flex-col">
        {#if !restoreState.keyPath}
          <div
            id="restore-key-dropzone"
            class="flex-1 flex flex-col items-center justify-center p-6 text-center border border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-stone-50 transition cursor-pointer select-none group"
            role="button"
            tabindex="0"
            onclick={() => restoreState.selectKeyFile()}
            onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') restoreState.selectKeyFile(); }}
            ondragover={(e) => { e.preventDefault(); if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'; }}
            ondrop={(e) => {
              e.preventDefault();
              const files = e.dataTransfer?.files;
              if (files && files.length > 0 && files[0]) {
                const path = (files[0] as any)?.path;
                if (path && (path.includes('/') || path.includes('\\'))) {
                  restoreState.loadKeyFile(path);
                }
              }
            }}
          >
            <div class="text-3xl mb-1.5 select-none transition group-hover:scale-105">🔑</div>
            <div class="font-semibold text-xs text-[#1C1917]">Выберите файл-ключ</div>
            <div class="text-[11px] text-[#A8A29E] mt-0.5 mb-3">или перетащите сюда</div>
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                restoreState.selectKeyFile();
              }}
              class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] group-hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-2xs hover:shadow-xs transition cursor-pointer"
            >
              Выбрать ключ
            </button>
          </div>
        {:else}
          <KeyInfoCard
            keyInfo={restoreState.keyInfo}
            keyPath={restoreState.keyPath}
            isInspecting={restoreState.isInspectingKey}
            onReselect={() => restoreState.selectKeyFile()}
            onClear={() => restoreState.clearKeyFile()}
          />
        {/if}
      </div>
    </div>

    <!-- Step 3: Password & Action -->
    <div class="bg-white border border-[#E8E7E3] p-5 shadow-xs flex flex-col min-h-[250px]">
      <div class="flex items-center justify-between text-xs text-[#78716C] mb-4 h-6 shrink-0">
        <span class="font-bold text-[#1C1917] font-sans">ШАГ 3. ПАРОЛЬ КЛЮЧА</span>
        <span class="text-[10px] bg-[#FAF9F5] border border-[#E8E7E3] px-2 py-0.5 whitespace-nowrap text-[#78716C] font-mono">AES-256</span>
      </div>

      <div class="flex-1 flex flex-col justify-between p-4 bg-[#FAF9F5] border border-[#E8E7E3] text-xs font-sans">
        <div>
          <label for="restore-key-password" class="block text-xs font-semibold text-[#1C1917] mb-2 font-sans">
            Пароль для расшифровки ключа:
          </label>
          <div class="relative">
            <input
              type={showPassword ? 'text' : 'password'}
              id="restore-key-password"
              placeholder="Пароль защиты ключа..."
              value={restoreState.password}
              oninput={(e) => restoreState.setPassword((e.target as HTMLInputElement).value)}
              onkeydown={async (e) => {
                if (e.key === 'Enter') {
                  await restoreState.inspectKeyNow();
                  if (isReady) executeRestore();
                }
              }}
              class="w-full pl-3 pr-8 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-white shadow-2xs transition"
            />
            {#if restoreState.password}
              <button
                type="button"
                onclick={() => showPassword = !showPassword}
                class="absolute right-2 top-1/2 -translate-y-1/2 text-[#78716C] hover:text-[#1C1917] p-1 transition cursor-pointer"
                title={showPassword ? 'Скрыть пароль' : 'Показать пароль'}
                aria-label={showPassword ? 'Скрыть пароль' : 'Показать пароль'}
              >
                {#if showPassword}
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l18 18" />
                  </svg>
                {:else}
                  <svg class="w-3.5 h-3.5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                    <path stroke-linecap="round" stroke-linejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                  </svg>
                {/if}
              </button>
            {/if}
          </div>

          {#if isPasswordError}
            <p class="text-[11px] text-rose-600 font-sans mt-1.5 flex items-center gap-1">
              <span>⚠</span>
              <span>Неверный пароль ключа</span>
            </p>
          {:else if isKeyValid}
            <p class="text-[11px] text-emerald-700 font-sans mt-1.5 flex items-center gap-1">
              <span>✔</span>
              <span>Ключ готов к восстановлению</span>
            </p>
          {:else}
            <p class="text-[11px] text-[#A8A29E] mt-1.5 font-sans">
              {restoreState.keyPath ? 'Введите пароль для расшифровки' : 'Сначала укажите файл и ключ'}
            </p>
          {/if}
        </div>

        <div class="pt-3 border-t border-[#E8E7E3]">
          <button
            id="btn-restore-start"
            type="button"
            onclick={executeRestore}
            disabled={!isReady}
            class="w-full bg-[#1C1917] disabled:bg-[#E8E7E3] disabled:text-[#A8A29E] disabled:cursor-not-allowed hover:bg-[#292524] text-white text-xs font-semibold py-2.5 transition active:scale-[0.99] flex items-center justify-center gap-2 shadow-2xs cursor-pointer font-sans"
          >
            <span class="text-xs select-none">🔓</span>
            <span>Восстановить данные</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</div>
