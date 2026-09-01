<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../i18n';
  import { onEvent, emitEvent, Window } from '../runtime';
  import { Clipboard } from '@wailsio/runtime';
  import {
    EventScreenshotOCR,
    EventScreenshotRecapture,
    EventScreenshotRetranslate,
    EventWindowClosing,
    ScreenshotSessionScreenshot,
  } from '../utils/events';
  import { WindowScreenshot } from '../constants/window';
  import TranslateCard from './TranslateCard.svelte';
  import {
    TRANSLATE_LANG,
    ALL_TRANSLATE_LANGS,
    TARGET_TRANSLATE_LANGS,
    type TranslateLang,
  } from '../constants/lang';
  import type {
    ScreenshotResult,
    TranslateResult,
  } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type { ScreenshotRetranslatePayload } from '../utils/events';
  import { GetConfig } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { persisted, pinKey } from '../stores/persisted';

  // 置顶状态持久化到 localStorage，与输入翻译窗口（pinKey('translate')）相互独立的记忆。
  const pinnedStore = persisted<boolean>(pinKey('screenshot'), false);
  let pinned = $derived($pinnedStore);
  async function togglePin() {
    const next = !pinned;
    pinnedStore.set(next);
    // 仅在用户主动 pin 时调用 SetAlwaysOnTop(true)。注意：不可在 next=false 时调用
    // SetAlwaysOnTop(false)——那会把创建时的 floating 层级压回 NSNormal，重新引入遮挡。
    // 窗口层级（floating）由 main.go 的 AlwaysOnTop:true 保证，pin 仅记忆用户偏好。
    if (next) {
      try {
        await Window.SetAlwaysOnTop(true);
      } catch (e) {
        console.error(t('log.screenshotSetPinFailed'), e);
      }
    }
  }

  let result = $state<ScreenshotResult | null>(null);
  let imgEl: HTMLImageElement | undefined = $state();

  // 截图翻译的语言条：展示源/目标语言，用户可直接选择。
  // 改语言后前端带防抖 emit EventScreenshotRetranslate，后端复用上次 OCR 原文直接重翻（跳过截图/OCR）。
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);

  // 从设置读取默认源/目标语言，作为语言条初始展示值。
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // retranslateTimer 防抖：改语言后 300ms 再触发重翻，避免连选时频繁请求。
  let retranslateTimer: ReturnType<typeof setTimeout> | null = null;
  let langReady = false; // 首次渲染（loadDefaults）期间不触发重翻，仅用户改动后才发。
  // 语言条变化即自动重翻（防抖）。监听两侧语言，任一侧变化都触发。
  $effect(() => {
    const f = fromLang;
    const t = toLang;
    if (!langReady) return; // 跳过初始值设定
    if (retranslateTimer) clearTimeout(retranslateTimer);
    retranslateTimer = setTimeout(() => {
      try {
        emitEvent(EventScreenshotRetranslate, {
          session: ScreenshotSessionScreenshot,
          from: f,
          to: t,
        } as ScreenshotRetranslatePayload);
        console.debug(t('log.screenshot_retranslate_emit'), { from: f, to: t });
      } catch (e) {
        console.error(t('log.screenshot_retranslate_failed'), e);
      }
    }, 300);
  });
  // 默认仅展开前两个「翻译成功」的卡片，其余（含失败的）全部收起。
  const successEngines = $derived(
    (result?.translations ?? []).filter((t) => t?.result).map((t) => t.engine),
  );
  const expandedEngines = $derived(new Set(successEngines.slice(0, 2)));

  function recapture() {
    try {
      emitEvent(EventScreenshotRecapture);
    } catch (e) {
      console.error(t('log.screenshotRecaptureFailed'), e);
    }
  }

  // 交换源/目标语言（Auto 不参与交换，落到 to 侧则视为无效，保持 Auto）。
  function swapLangs() {
    if (fromLang === TRANSLATE_LANG.Auto) return;
    const tmp = toLang;
    toLang = fromLang;
    fromLang = tmp === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.Auto : tmp;
  }

  function closeWindow() {
    // 关闭前清理图片/译文等状态，下次打开是干净窗口。
    // 注意：关闭走 Window.Close() → 后端 WindowClosing hook（Cancel + Hide），
    // 与输入翻译窗口一致。不要用 Window.Hide() 直接隐藏——那样会绕过后端
    // hook，导致窗口隐藏状态异常、下次 Show 时 Focus 不生效（表现为被遮挡）。
    result = null;
    if (imgEl) imgEl.src = '';
    try {
      Window.Close();
    } catch (e) {
      console.error(t('log.screenshotCloseFailed'), e);
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
      console.error(t('log.screenshotCopyFailed'), e);
    }
  }

  const off = onEvent(EventScreenshotOCR, (data: ScreenshotResult) => {
    try {
      // 收到后端推送的原始事件（后端→前端），第一手证据：判断"后端到底推了什么"。
      console.debug(t('log.screenshotLogOcrEvent'), {
        hasImage: !!data.image,
        imagePrefix: (data.image ?? '').slice(0, 80),
        textLen: (data.text ?? '').length,
        error: data.error ?? '',
        translations: (data.translations ?? []).length,
      });
      const incoming = Array.isArray(data.translations) ? data.translations : [];
      // 原文 + 截图整体替换（每次推送都带），保证识别到内容即展示。
      const base = {
        image: data.image,
        text: data.text ?? '',
        to: data.to,
        error: data.error ?? '',
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
      if (result.error) {
        console.warn(t('log.screenshotRenderError'), result.error);
      } else {
        console.debug(t('log.screenshotLogRenderResult'), {
          imageLen: (result.image ?? '').length,
          textLen: result.text.length,
          translations: result.translations.length,
          engines: result.translations.map((t) => t.engine),
        });
      }
    } catch (e) {
      console.error(t('log.screenshotRenderOcrFailed'), e);
    }
  });

  // img 的 src 由响应式 effect 同步（result.image 变化即重设），
  // 避免 onEvent 在 result 赋值后、DOM 尚未重渲染（imgEl 尚未 bind:this 就绪）时
  // 直接 imgEl.src = ... 设到 undefined 上导致图片不显示。
  // 同时挂 onerror 诊断图片加载失败。
  $effect(() => {
    const url = result?.image;
    if (imgEl && url) {
      // 挂载一次性的 onerror 诊断（只在首次挂载时设，避免每次 effect 重跑重复绑定）。
      // 注意：error 态（OCR 超时/失败）下 result 被整体重设为带 error 的新对象，
      // 本 effect 会重跑但 src 不变（下方守卫），不应触发真正的加载失败——
      // 故 onerror 仅记录、降为 warn，且带 error 态不报，避免超时路径下的误报。
      if (!imgEl.dataset.ocrErrBound) {
        imgEl.onerror = () => {
          if (result?.error) return; // 错误态下图片可能被卸载重挂，onerror 属副作用，非真加载失败
          console.warn(t('log.screenshotImageLoadFailed'), {
            imageLen: url.length,
            imagePrefix: url.slice(0, 80),
          });
        };
        imgEl.dataset.ocrErrBound = '1';
      }
      // 关键：仅在 src 真正变化时重设，避免每次事件 new 对象导致的大 base64 图被反复重设 src 而偶发 onerror。
      if (imgEl.getAttribute('src') !== url) {
        imgEl.src = url;
      }
    }
  });

  // 监听本窗口（screenshot）关闭事件：原生红 X → 后端 WindowClosing hook 广播
  // EventWindowClosing，这里按窗口名过滤后清空截图与译文，下次打开是干净窗口。
  const offClosing = onEvent(EventWindowClosing, (name: string) => {
    if (name !== WindowScreenshot) return;
    result = null;
    if (imgEl) imgEl.src = '';
  });

  onMount(async () => {
    console.debug(t('log.screenshotLogMounted'));
    loadDefaults().then(() => {
      // loadDefaults 设定初始值后，下一拍再允许语言变更触发重翻，避免初始设定误触发。
      setTimeout(() => {
        langReady = true;
      }, 0);
    });
    // 截图翻译窗口是临时浮窗。窗口层级已在 main.go 设 MacWindowLevelModalPanel，
    // 默认即浮在普通窗口之上（无需 AlwaysOnTop）。这里仅在用户主动 pin 时
    // 调用 SetAlwaysOnTop(true) 永久钉住；pin=false 时不调用，避免把 modalPanel
    // 层级压回普通（否则会丢失浮起能力）。
    if ($pinnedStore) {
      try {
        await Window.SetAlwaysOnTop(true);
      } catch (e) {
        console.error(t('log.screenshotSetPinFailed'), e);
      }
    }
    return () => {
      off();
      offClosing();
    };
  });
</script>

<div class="u-surface relative flex h-full flex-col overflow-hidden">
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
        <!-- 常驻工具栏：仅出现在右侧内容区顶部，置顶（与输入翻译窗口各自记忆）+ 重新截图。 -->
        <div class="mb-3 flex items-center justify-end gap-2">
          <button
            class="u-icon-btn u-icon-btn--sm u-no-drag"
            class:u-icon-btn--active={pinned}
            onclick={togglePin}
            aria-label={pinned ? t('translate.unpin') : t('translate.pin')}
            title={pinned ? t('translate.unpin') : t('translate.pin')}
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M12 17v5" />
              <path
                d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"
              />
            </svg>
          </button>
          <button
            class="u-btn u-btn--ghost u-no-drag px-2.5 py-1 text-xs"
            onclick={recapture}
            title={t('screenshot.recapture')}
          >
            {t('screenshot.recapture')}
          </button>
        </div>

        {#if result.error}
          <!-- 识别/翻译失败：停止转圈并展示错误（含超时），不再无限"识别中" -->
          <div
            class="flex flex-1 flex-col items-center justify-center gap-3 text-[var(--app-muted)]"
          >
            <span class="text-lg font-medium text-[var(--app-fg-strong)]"
              >{t('screenshot.ocrError')}</span
            >
            <span class="max-w-[90%] break-words text-center text-xs opacity-80"
              >{result.error}</span
            >
            <span class="text-xs">{t('screenshot.retryHint')}</span>
          </div>
        {:else if !result.text}
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
          <!-- 语言控制条：样式与输入翻译窗口一致，用户可直接改语言触发重翻。 -->
          <div class="mb-3 flex items-center justify-center gap-2">
            <select
              class="u-field u-select u-lang-select px-3 py-2 text-sm"
              value={fromLang}
              aria-label={t('translate.from')}
              onchange={(e) => (fromLang = e.currentTarget.value as TranslateLang)}
            >
              {#each ALL_TRANSLATE_LANGS as l}
                <option value={l}>{langName(l)}</option>
              {/each}
            </select>

            <button
              class="u-icon-btn u-no-drag"
              aria-label={t('translate.swap')}
              title={t('translate.swap')}
              onclick={swapLangs}
            >
              <svg
                width="16"
                height="16"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M7 10h14l-4-4" />
                <path d="M17 14H3l4 4" />
              </svg>
            </button>

            <select
              class="u-field u-select u-lang-select px-3 py-2 text-sm"
              value={toLang}
              aria-label={t('translate.to')}
              onchange={(e) => (toLang = e.currentTarget.value as TranslateLang)}
            >
              {#each TARGET_TRANSLATE_LANGS as l}
                <option value={l}>{langName(l)}</option>
              {/each}
            </select>
          </div>

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
                <TranslateCard
                  {tr}
                  expanded={expandedEngines.has(tr.engine)}
                  onCopied={() => showToast(t('common.copied'))}
                />
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
