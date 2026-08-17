<script lang="ts">
  import { t, langName, engineName } from '../i18n';
  import { Clipboard } from '@wailsio/runtime';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';

  let { tr, onCopied }: { tr: TranslateResult; onCopied?: (text: string) => void } = $props();

  async function copyText(text: string | undefined) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      onCopied?.(text ?? '');
    } catch (e) {
      console.error(t('log.translateCardCopyFailed'), e);
    }
  }
</script>

<div class="u-card p-3">
  <div class="mb-1 flex items-center justify-between">
    <span class="text-xs font-semibold text-[var(--app-accent)]"
      >{engineName(tr?.engine ?? '')}</span
    >
    <div class="flex items-center gap-2">
      <span class="text-[11px] text-[var(--app-muted)]">
        {t('screenshot.source')}: {langName(tr?.from ?? '')} → {langName(tr?.to ?? '')}
      </span>
      <button
        class="u-btn u-btn--ghost p-1"
        title={t('common.copy')}
        aria-label={t('common.copy')}
        onclick={() => copyText(tr?.result)}
      >
        <svg
          class="h-3.5 w-3.5"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
        </svg>
      </button>
    </div>
  </div>
  <div class="whitespace-pre-wrap break-words text-sm leading-relaxed">
    {#if tr?.result}
      {tr.result}
    {:else}
      <span class="text-[var(--app-danger)]">{t('screenshot.translateFailed')}</span>
    {/if}
  </div>
</div>
