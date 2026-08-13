<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../i18n';
  import { onEvent, emitEvent, Window } from '../runtime';
  import { Clipboard } from '@wailsio/runtime';
  import { EventScreenshotOCR, EventScreenshotRecapture } from '../utils/events';
  import TitleBar from './TitleBar.svelte';
  import TranslateCard from './TranslateCard.svelte';
  import type {
    ScreenshotResult,
    TranslateResult,
  } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';

  let result = $state<ScreenshotResult | null>(null);
  let imgEl: HTMLImageElement | undefined = $state();

  function recapture() {
    try {
      emitEvent(EventScreenshotRecapture);
    } catch (e) {
      console.error('[截图翻译] 重新截图失败', e);
    }
  }

  function closeWindow() {
    // 关闭前清理图片/译文等状态，下次打开是干净窗口。
    result = null;
    if (imgEl) imgEl.src = '';
    try {
      Window.Hide();
    } catch (e) {
      console.error('[截图翻译] 关闭窗口失败', e);
    }
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | null = null;
  function showToast(msg: string) {
    toast = msg;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      toast = '';
    }, 1500);
  }

  async function copyText(text: string | undefined) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('[截图翻译] 复制失败', e);
    }
  }

  const off = onEvent(EventScreenshotOCR, (data: ScreenshotResult) => {
    try {
      const incoming = Array.isArray(data.translations) ? data.translations : [];
      // 原文 + 截图整体替换（每次推送都带），保证识别到内容即展示。
      const base = {
        image: data.image,
        text: data.text ?? '',
        to: data.to,
      };
      // translations 按 engine 增量合并：已有同引擎则覆盖，否则追加。
      // 这样后端先推空（仅原文），再逐条推译文时前端逐条显示。
      const merged = new Map<string, TranslateResult>();
      for (const t of result?.translations ?? []) {
        if (t && typeof t.engine === 'string') merged.set(t.engine, t);
      }
      for (const t of incoming) {
        if (t && typeof t.engine === 'string') merged.set(t.engine, t);
      }
      result = {
        ...base,
        translations: Array.from(merged.values()),
      } as ScreenshotResult;
      console.log('[截图翻译] 收到 OCR 结果', {
        imageLen: data.image?.length ?? 0,
        imagePrefix: data.image?.slice(0, 80) ?? '',
        textLen: result.text.length,
        translations: result.translations.length,
      });
    } catch (e) {
      console.error('[截图翻译] 渲染 OCR 结果失败', e);
    }
  });

  // img 的 src 由响应式 effect 同步（result.image 变化即重设），
  // 避免 onEvent 在 result 赋值后、DOM 尚未重渲染（imgEl 尚未 bind:this 就绪）时
  // 直接 imgEl.src = ... 设到 undefined 上导致图片不显示。
  // 同时挂 onerror 诊断图片加载失败。
  $effect(() => {
    const url = result?.image;
    if (imgEl && url) {
      imgEl.onerror = () => {
        console.error('[截图翻译] 图片加载失败', {
          imageLen: url.length,
          imagePrefix: url.slice(0, 80),
        });
      };
      imgEl.src = url;
    }
  });

  onMount(() => {
    console.log('[截图翻译] 窗口已挂载');
    // 截图翻译窗口是临时浮窗，挂载后设为置顶（AlwaysOnTop），
    // 否则显示后会被其他窗口盖住、看不到。必须在挂载后设置（webview 就绪）。
    try {
      Window.SetAlwaysOnTop(true);
    } catch (e) {
      console.error('[截图翻译] 设置置顶失败', e);
    }
    return () => {
      off();
    };
  });
</script>

<div class="u-surface relative flex h-full flex-col overflow-hidden">
  <!-- 标题栏：复用统一 TitleBar 组件（含 macOS 红绿灯，与输入翻译/设置窗口一致）。
       关闭走 onClose（Window.Hide）而非默认 Close，避免销毁复用中的浮窗实例。 -->
  <TitleBar onClose={closeWindow} />

  {#if result}
    <div class="flex min-h-0 flex-1">
      <!-- 左：区域截图 -->
      <div
        class="flex min-w-0 flex-1 items-center justify-center border-r border-[var(--app-border)] p-3"
      >
        <img
          bind:this={imgEl}
          alt="screenshot"
          class="max-h-full max-w-full rounded-lg object-contain shadow-[var(--app-card-shadow)]"
        />
      </div>

      <!-- 右：原文 + 多引擎译文 -->
      <div class="flex min-w-0 flex-1 flex-col overflow-y-auto p-4">
        {#if !result.text}
          <!-- 识别中：OCR 尚未返回原文 -->
          <div
            class="flex flex-1 flex-col items-center justify-center gap-3 text-[var(--app-muted)]"
          >
            <span
              class="h-7 w-7 animate-spin rounded-full border-2 border-[var(--app-border)] border-t-[var(--app-accent)]"
            ></span>
            <span class="text-sm">{t('screenshot.recognizing')}</span>
          </div>
        {:else}
          <div class="mb-3">
            <div
              class="mb-1 flex items-center justify-between text-xs font-medium text-[var(--app-muted)]"
            >
              <span>{t('screenshot.original')}</span>
              <button
                class="u-btn u-btn--ghost p-1"
                title={t('common.copy')}
                aria-label={t('common.copy')}
                onclick={() => copyText(result?.text)}
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
            <div
              class="whitespace-pre-wrap break-words rounded-lg bg-[var(--app-card)] p-3 text-sm leading-relaxed"
            >
              {result.text}
            </div>
          </div>

          {#if result.translations && result.translations.length > 0}
            <div class="flex flex-col gap-3">
              <div class="text-xs font-medium text-[var(--app-muted)]">
                {t('screenshot.result')}
              </div>
              {#each result.translations as tr}
                <TranslateCard {tr} onCopied={() => showToast(t('common.copied'))} />
              {/each}
            </div>
          {:else}
            <!-- 翻译中：OCR 已完成、译文尚未返回 -->
            <div class="mt-4 flex flex-col items-center gap-2 text-[var(--app-muted)]">
              <span
                class="h-6 w-6 animate-spin rounded-full border-2 border-[var(--app-border)] border-t-[var(--app-accent)]"
              ></span>
              <span class="text-sm">{t('screenshot.translating')}</span>
            </div>
          {/if}
        {/if}
      </div>
    </div>
  {:else}
    <div
      class="flex flex-1 items-center justify-center px-6 text-center text-sm text-[var(--app-muted)]"
    >
      {t('screenshot.empty')}
    </div>
  {/if}

  {#if toast}
    <div
      class="pointer-events-none absolute bottom-4 left-1/2 z-50 -translate-x-1/2 rounded-lg bg-black/75 px-3 py-1.5 text-xs text-white shadow-lg"
    >
      {toast}
    </div>
  {/if}
</div>
