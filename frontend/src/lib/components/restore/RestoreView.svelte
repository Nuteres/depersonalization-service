<script lang="ts">
  import { restoreState } from '../../stores/restoreState.svelte';
  import { appState } from '../../stores/appState.svelte';
  import { SaveFileDialog, RestoreFileTo } from '../../api/wails';

  const dataFileName = $derived(
    restoreState.dataPath.split('/').pop()?.split('\\').pop() || 'data.xlsx'
  );

  const keyFileName = $derived(
    restoreState.keyPath.split('/').pop()?.split('\\').pop() || 'key.depmap'
  );

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
        dotClass: 'w-2 h-2 rounded-full bg-emerald-600 animate-pulse',
        text: 'Готово к восстановлению',
        textClass: 'text-emerald-800 font-medium'
      };
    }
    if (!restoreState.dataPath && !restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-stone-400',
        text: 'Ожидание файла данных',
        textClass: 'text-stone-600'
      };
    }
    if (!restoreState.dataPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Ожидание файла данных',
        textClass: 'text-stone-600'
      };
    }
    if (!restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Ожидание ключа .depmap',
        textClass: 'text-stone-600'
      };
    }
    if (restoreState.keyInfo && !restoreState.keyInfo.valid) {
      if (!restoreState.password) {
        return {
          dotClass: 'w-2 h-2 rounded-full bg-amber-500',
          text: 'Требуется пароль ключа',
          textClass: 'text-stone-600'
        };
      }
      return {
        dotClass: 'w-2 h-2 rounded-full bg-rose-500',
        text: 'Неверный пароль ключа',
        textClass: 'text-rose-700 font-medium'
      };
    }
    return {
      dotClass: 'w-2 h-2 rounded-full bg-stone-400',
      text: 'Ожидание файла данных',
      textClass: 'text-stone-600'
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
      <p class="text-xs text-[#78716C] mt-1 font-sans">
        Деобезличивание документов с использованием зашифрованного ключа .depmap
      </p>
    </div>

    <!-- Live Step Status -->
    <div
      id="restore-hero-status"
      class="flex items-center space-x-2 text-xs font-sans px-3.5 py-1.5 bg-white border border-[#E8E7E3] rounded-md shadow-2xs self-start md:self-auto"
    >
      <span id="restore-status-dot" class={statusInfo.dotClass}></span>
      <span id="restore-status-text" class={statusInfo.textClass}>{statusInfo.text}</span>
    </div>
  </div>

  <!-- 3-Column Selection Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-5 items-stretch">
    <!-- Card 1: Data File -->
    <div class="bg-white border border-[#E8E7E3] rounded-lg p-5 shadow-xs flex flex-col justify-between min-h-[260px]">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 h-6 shrink-0">
          <span class="font-semibold text-[#1C1917] tracking-tight">
            Файл данных
          </span>
          {#if restoreState.dataPath}
            <span class="text-xs text-emerald-700 font-medium flex items-center gap-1.5">
              <span class="w-1.5 h-1.5 rounded-full bg-emerald-600"></span>
              Готов
            </span>
          {:else}
            <span class="text-[10px] font-mono text-[#A8A29E]">Таблица / Текст</span>
          {/if}
        </div>

        {#if restoreState.isLoadingData}
          <div class="flex flex-col items-center justify-center p-6 text-center bg-[#FAF9F5] border border-[#E8E7E3] rounded-lg gap-2.5 select-none hover-sheen">
            <div class="w-6 h-6 border-2 border-[#E8E7E3] border-t-[#1C1917] rounded-full animate-spin"></div>
            <div class="font-semibold text-xs text-[#1C1917]">Чтение структуры...</div>
            <div class="text-[11px] text-[#78716C]">Определение строк и колонок</div>
          </div>
        {:else if !restoreState.dataPath}
          <div
            id="restore-data-dropzone"
            class="flex flex-col items-center justify-center p-6 text-center border border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-stone-50 transition cursor-pointer select-none group rounded-lg hover-sheen"
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
            <svg class="w-8 h-8 text-stone-400 group-hover:text-stone-700 transition-colors mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
            </svg>
            <div class="font-semibold text-xs text-[#1C1917]">Выберите или перетащите файл</div>
            <div class="text-[11px] text-[#A8A29E] mt-0.5 mb-3.5">XLSX, ODS, CSV, DOCX, TXT</div>
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                restoreState.selectDataFile();
              }}
              class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] group-hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] rounded-md shadow-2xs pressable cursor-pointer"
            >
              Выбрать файл
            </button>
          </div>
        {:else}
          <div id="restore-data-info" class="py-7 flex items-center gap-3">
            <div class="w-9 h-9 rounded-md bg-[#FAF9F5] border border-[#E8E7E3] flex items-center justify-center shrink-0 text-emerald-700">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/>
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs text-[#1C1917] truncate" id="restore-data-name" title={dataFileName}>
                {dataFileName}
              </div>
            </div>
          </div>
        {/if}
      </div>

      {#if restoreState.dataPath}
        <div class="pt-4 border-t border-[#F0EFEB] flex items-center justify-end gap-2">
          <button
            type="button"
            onclick={() => restoreState.selectDataFile()}
            class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md shadow-2xs pressable cursor-pointer"
          >
            Сменить
          </button>
          <button
            type="button"
            id="btn-remove-data-file"
            onclick={() => restoreState.clearDataFile()}
            class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-rose-300 hover:text-rose-600 text-[#78716C] text-xs font-semibold rounded-md shadow-2xs pressable cursor-pointer"
          >
            Удалить
          </button>
        </div>
      {/if}
    </div>

    <!-- Card 2: Key File -->
    <div class="bg-white border border-[#E8E7E3] rounded-lg p-5 shadow-xs flex flex-col justify-between min-h-[260px]">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 h-6 shrink-0">
          <span class="font-semibold text-[#1C1917] tracking-tight">Ключ деобезличивания</span>
          {#if restoreState.keyPath}
            {#if restoreState.isInspectingKey}
              <span class="text-xs text-amber-600 font-medium flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-amber-500 animate-pulse"></span>
                Проверка...
              </span>
            {:else if isKeyValid}
              <span class="text-xs text-emerald-700 font-medium flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-600"></span>
                Готов
              </span>
            {:else}
              <span class="text-xs text-amber-600 font-medium flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full bg-amber-500"></span>
                Зашифрован
              </span>
            {/if}
          {:else}
            <span class="text-[10px] bg-stone-100 border border-[#E8E7E3] px-2 py-0.5 rounded whitespace-nowrap text-[#78716C] font-mono">.depmap</span>
          {/if}
        </div>

        {#if !restoreState.keyPath}
          <div
            id="restore-key-dropzone"
            class="flex flex-col items-center justify-center p-6 text-center border border-dashed border-[#D6D3D1] hover:border-[#1C1917] bg-[#FAF9F5] hover:bg-stone-50 transition cursor-pointer select-none group rounded-lg hover-sheen"
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
            <svg class="w-8 h-8 text-stone-400 group-hover:text-stone-700 transition-colors mb-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
            </svg>
            <div class="font-semibold text-xs text-[#1C1917]">Выберите ключ восстановления</div>
            <div class="text-[11px] text-[#A8A29E] mt-0.5 mb-3.5">Файл с расширением .depmap</div>
            <button
              type="button"
              onclick={(e) => {
                e.stopPropagation();
                restoreState.selectKeyFile();
              }}
              class="px-3.5 py-1.5 bg-white border border-[#E8E7E3] group-hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] rounded-md shadow-2xs hover:shadow-xs pressable transition cursor-pointer"
            >
              Выбрать ключ
            </button>
          </div>
        {:else}
          <div id="restore-key-info" class="py-7 flex items-center gap-3">
            <div class="w-9 h-9 rounded-md bg-[#FAF9F5] border border-[#E8E7E3] flex items-center justify-center shrink-0 text-emerald-700">
              <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.8" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/>
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <div class="font-bold text-xs text-[#1C1917] truncate" id="restore-key-name" title={keyFileName}>
                {keyFileName}
              </div>
            </div>
          </div>
        {/if}
      </div>

      {#if restoreState.keyPath}
        <div class="pt-4 border-t border-[#F0EFEB] flex items-center justify-end gap-2">
          <button
            type="button"
            id="btn-reselect-key"
            onclick={() => restoreState.selectKeyFile()}
            class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-[#1C1917] text-xs font-semibold rounded-md shadow-2xs pressable cursor-pointer"
          >
            Сменить
          </button>
          <button
            type="button"
            id="btn-remove-key-file"
            onclick={() => restoreState.clearKeyFile()}
            class="px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-rose-300 hover:text-rose-600 text-[#78716C] text-xs font-semibold rounded-md shadow-2xs pressable cursor-pointer"
          >
            Удалить
          </button>
        </div>
      {/if}
    </div>

    <!-- Card 3: Password & Action -->
    <div class="bg-white border border-[#E8E7E3] rounded-lg p-5 shadow-xs flex flex-col justify-between min-h-[260px]">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 h-6 shrink-0">
          <span class="font-semibold text-[#1C1917] tracking-tight">Пароль ключа</span>
          <span class="text-[10px] bg-stone-100 border border-[#E8E7E3] px-2 py-0.5 rounded whitespace-nowrap text-[#78716C] font-mono">AES-256</span>
        </div>

        <div class="space-y-2">
          <div class="relative">
            <input
              type={showPassword ? 'text' : 'password'}
              id="restore-key-password"
              placeholder="Пароль защиты ключа"
              value={restoreState.password}
              oninput={(e) => restoreState.setPassword((e.target as HTMLInputElement).value)}
              onkeydown={async (e) => {
                if (e.key === 'Enter') {
                  await restoreState.inspectKeyNow();
                  if (isReady) executeRestore();
                }
              }}
              class="w-full pl-3 pr-8 py-2 border border-[#E8E7E3] rounded-md focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5] focus:bg-white transition"
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
              <span>Неверный пароль ключа</span>
            </p>
          {:else if isKeyValid}
            <p class="text-[11px] text-emerald-700 font-sans mt-1.5 flex items-center gap-1">
              <span>Ключ готов к восстановлению</span>
            </p>
          {:else}
            <p class="text-[11px] text-[#A8A29E] mt-1.5 font-sans">
              {restoreState.keyPath ? 'Введите пароль для расшифровки' : 'Сначала укажите файл и ключ'}
            </p>
          {/if}
        </div>
      </div>

      <div class="pt-4 border-t border-[#E8E7E3]">
        <button
          id="btn-restore-start"
          type="button"
          onclick={executeRestore}
          disabled={!isReady}
          class="w-full bg-[#1C1917] disabled:bg-stone-200 disabled:text-stone-400 disabled:cursor-not-allowed hover:bg-[#292524] text-white text-xs font-semibold py-2.5 rounded-md shadow-xs pressable flex items-center justify-center gap-1.5 cursor-pointer font-sans"
        >
          <span>Восстановить данные</span>
          <span>&rarr;</span>
        </button>
      </div>
    </div>
  </div>
</div>
