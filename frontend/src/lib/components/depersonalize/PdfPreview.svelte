<script lang="ts">
  import { appState } from '../../stores/appState.svelte';

  interface RedactedPart {
    text: string;
    isRedacted: boolean;
  }

  const renderedParts = $derived.by<RedactedPart[]>(() => {
    const raw =
      appState.currentFileInfo?.pdfRedactedSnippet ||
      appState.currentFileInfo?.pdfTextSnippet ||
      '';
    if (!raw) return [];
    const parts: RedactedPart[] = [];
    const regex = /\[\s*ПДн\s*СКРЫТО\s*\]/gi;
    let lastIndex = 0;
    let match: RegExpExecArray | null;

    while ((match = regex.exec(raw)) !== null) {
      if (match.index > lastIndex) {
        parts.push({ text: raw.slice(lastIndex, match.index), isRedacted: false });
      }
      parts.push({ text: '[ ПДн СКРЫТО ]', isRedacted: true });
      lastIndex = match.index + match[0].length;
    }
    if (lastIndex < raw.length) {
      parts.push({ text: raw.slice(lastIndex), isRedacted: false });
    }
    return parts;
  });
</script>

{#if appState.currentFileInfo && appState.currentFileInfo.fileType === 'pdf'}
  <div class="flex flex-col gap-4">
    <!-- Clean Paper Sheet with Blackout Preview -->
    <div
      class="bg-[#E8E7E3] p-6 sm:p-12 flex justify-center items-center border border-[#D6D3D1]"
    >
      <div
        class="bg-white p-8 sm:p-14 max-w-2xl w-full text-xs font-serif leading-relaxed text-[#1C1917] shadow-lg border border-[#E8E7E3]"
      >
        <div class="text-center font-bold text-sm tracking-wide mb-6">
          ПРЕДПРОСМОТР PDF (Blackout Redaction)
        </div>

        {#if renderedParts.length > 0}
          <div class="mb-4 text-justify whitespace-pre-wrap font-mono text-xs leading-6">
            {#each renderedParts as part}
              {#if part.isRedacted}
                <span class="redaction-bar">[ ПДн СКРЫТО ]</span>
              {:else}
                <span>{part.text}</span>
              {/if}
            {/each}
          </div>
        {:else}
          <p class="mb-4 text-justify text-[#78716C] font-sans">
            Откройте PDF файл для просмотра защищенного текста со скрытыми персональными данными.
          </p>
        {/if}

        <div
          class="mt-8 pt-4 border-t border-[#E8E7E3] flex flex-col sm:flex-row sm:items-center justify-between gap-2 text-[11px] font-sans text-[#78716C]"
        >
          <span>Физическое стирание символов и закрашивание плашками</span>
          <span class="redaction-bar">[ ПДн СКРЫТО ]</span>
        </div>
      </div>
    </div>
  </div>
{/if}
