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

  const dataStatsText = $derived.by(() => {
    const info = restoreState.dataInfo;
    if (!info) {
      return restoreState.isLoadingData ? 'Загрузка информации...' : 'Готов к обработке';
    }
    const sizeKb = Math.round((info.size || 0) / 1024);
    const countStr = info.paragraphCount
      ? `${info.paragraphCount.toLocaleString('ru-RU')} абз.`
      : `${(info.rowCount || 0).toLocaleString('ru-RU')} строк`;
    return `${countStr} ▪ ${sizeKb} КБ`;
  });

  const isReady = $derived(
    !!(restoreState.dataPath && restoreState.keyInfo && restoreState.keyInfo.valid)
  );

  const statusInfo = $derived.by(() => {
    if (isReady) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-emerald-600',
        text: '✔ Готово к восстановлению данных',
        textClass: 'text-[#065F46] font-semibold'
      };
    }
    if (!restoreState.dataPath && !restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-stone-400',
        text: 'Шаг 1 из 3: выберите файл данных',
        textClass: 'text-[#78716C]'
      };
    }
    if (!restoreState.dataPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Шаг 1: выберите файл данных',
        textClass: 'text-[#78716C]'
      };
    }
    if (!restoreState.keyPath) {
      return {
        dotClass: 'w-2 h-2 rounded-full bg-amber-500',
        text: 'Шаг 2: выберите ключ (.depmap)',
        textClass: 'text-[#78716C]'
      };
    }
    if (restoreState.keyInfo && !restoreState.keyInfo.valid) {
      if (!restoreState.password) {
        return {
          dotClass: 'w-2 h-2 rounded-full bg-amber-500',
          text: 'Шаг 3: введите пароль ключа',
          textClass: 'text-[#78716C]'
        };
      }
      return {
        dotClass: 'w-2 h-2 rounded-full bg-rose-500',
        text: 'Ошибка: неверный пароль ключа',
        textClass: 'text-rose-700 font-semibold'
      };
    }
    return {
      dotClass: 'w-2 h-2 rounded-full bg-stone-400',
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

      appState.startProgress('Восстановление данных...');

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

<div id="restoration-view" class="flex-1 flex flex-col gap-6 pb-12">
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
      class="flex items-center space-x-2 text-xs font-mono px-4 py-2 bg-[#FAF9F5] border border-[#E8E7E3]"
    >
      <span id="restore-status-dot" class={statusInfo.dotClass}></span>
      <span id="restore-status-text" class={statusInfo.textClass}>{statusInfo.text}</span>
    </div>
  </div>

  <!-- 3-Step Selection Grid -->
  <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
    <!-- Step 1: Data File -->
    <div class="bg-white border border-[#E8E7E3] p-5 shadow-xs">
      <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 font-mono">
        <span class="font-bold text-[#1C1917]">ШАГ 1. ФАЙЛ С ДАННЫМИ</span>
        <span class="text-[10px] bg-[#FAF9F5] border border-[#E8E7E3] px-1.5 py-0.5">XLSX / ODS / DOCX / ODT / CSV</span>
      </div>

      {#if !restoreState.dataPath}
        <div
          id="restore-data-dropzone"
          class="border border-dashed border-[#D6D3D1] hover:border-[#1C1917] p-5 text-center transition cursor-pointer bg-[#FAF9F5]"
          role="button"
          tabindex="0"
          onclick={() => restoreState.selectDataFile()}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') restoreState.selectDataFile(); }}
          ondragover={(e) => e.preventDefault()}
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
          <div class="text-2xl mb-1">📄</div>
          <div class="font-semibold text-xs text-[#1C1917]">Выберите файл с данными</div>
          <button
            type="button"
            class="mt-3 px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs cursor-pointer"
          >
            Выбрать файл
          </button>
        </div>
      {:else}
        <div id="restore-data-info" class="p-3 bg-[#FAF9F5] border border-[#E8E7E3] text-xs font-mono space-y-1">
          <div class="flex items-center justify-between">
            <span class="font-bold text-[#1C1917] truncate max-w-[180px]" id="restore-data-name" title={dataFileName}>
              {dataFileName}
            </span>
            <span class="text-[10px] bg-emerald-50 text-emerald-700 border border-emerald-200 px-1 py-0.5" id="restore-data-ext">
              {dataFileExt}
            </span>
          </div>
          <div class="text-[11px] text-[#78716C] truncate" id="restore-data-path" title={restoreState.dataPath}>
            {restoreState.dataPath}
          </div>
          <div class="flex items-center justify-between pt-1">
            <span class="text-[11px] text-[#57534E]" id="restore-data-size">
              {dataStatsText}
            </span>
            <button
              type="button"
              onclick={() => restoreState.selectDataFile()}
              class="text-[11px] text-[#1C1917] hover:underline font-semibold cursor-pointer"
            >
              Сменить файл
            </button>
          </div>
        </div>
      {/if}
    </div>

    <!-- Step 2: Key File -->
    <div class="bg-white border border-[#E8E7E3] p-5 shadow-xs">
      <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 font-mono">
        <span class="font-bold text-[#1C1917]">ШАГ 2. ФАЙЛ-КЛЮЧ</span>
        <span class="text-[10px] bg-[#FAF9F5] border border-[#E8E7E3] px-1.5 py-0.5">.DEPMAP</span>
      </div>

      {#if !restoreState.keyPath}
        <div
          id="restore-key-dropzone"
          class="border border-dashed border-[#D6D3D1] hover:border-[#1C1917] p-5 text-center transition cursor-pointer bg-[#FAF9F5]"
          role="button"
          tabindex="0"
          onclick={() => restoreState.selectKeyFile()}
          onkeydown={(e) => { if (e.key === 'Enter' || e.key === ' ') restoreState.selectKeyFile(); }}
          ondragover={(e) => e.preventDefault()}
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
          <div class="text-2xl mb-1">🔑</div>
          <div class="font-semibold text-xs text-[#1C1917]">Выберите файл-ключ</div>
          <button
            type="button"
            class="mt-3 px-3 py-1.5 bg-white border border-[#E8E7E3] hover:border-[#1C1917] text-xs font-semibold text-[#1C1917] shadow-xs cursor-pointer"
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
        />
      {/if}
    </div>

    <!-- Step 3: Password & Summary -->
    <div class="bg-white border border-[#E8E7E3] p-5 flex flex-col justify-between shadow-xs">
      <div>
        <div class="flex items-center justify-between text-xs text-[#78716C] mb-3 font-mono">
          <span class="font-bold text-[#1C1917]">ШАГ 3. ПАРОЛЬ КЛЮЧА</span>
        </div>

        <div class="space-y-3">
          <p class="text-xs text-[#78716C]">
            Введите пароль для расшифровки ключа:
          </p>
          <div>
            <input
              type="password"
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
              class="w-full px-3 py-2 border border-[#E8E7E3] focus:border-[#1C1917] focus:outline-none text-xs font-mono bg-[#FAF9F5]"
            />
          </div>
        </div>
      </div>

      <div class="mt-6 pt-4 border-t border-[#F0EFEB]">
        <button
          id="btn-restore-start"
          type="button"
          onclick={executeRestore}
          disabled={!isReady}
          class="w-full bg-[#1C1917] disabled:bg-stone-300 disabled:cursor-not-allowed hover:bg-[#292524] text-white text-xs font-semibold py-3 transition active:scale-[0.99] flex items-center justify-center gap-2 shadow-sm cursor-pointer"
        >
          <span class="text-xs leading-none shrink-0 inline-flex items-center justify-center select-none">🔓</span>
          <span>Восстановить данные</span>
        </button>
      </div>
    </div>
  </div>
</div>
