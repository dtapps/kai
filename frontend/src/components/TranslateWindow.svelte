<script lang="ts">
  import { onMount, tick } from 'svelte';
  import TitleBar from './TitleBar.svelte';
  import { t, langName, engineName } from '../i18n';
  import { rootStyle } from '../stores/theme';
  import { currentLang } from '../stores/ui';
  import { persisted, pinKey } from '../stores/persisted';
  import { rootStyleToStyle } from '../utils/style';
  import { onEvent } from '../runtime';
  import { Window, Clipboard } from '@wailsio/runtime';

  // 置顶状态持久化到 localStorage，重开窗口后保留。
  const pinnedStore = persisted<boolean>(pinKey('translate'), false);
  let pinned = $derived($pinnedStore);
  async function togglePin() {
    const next = !pinned;
    pinnedStore.set(next);
    try {
      await Window.SetAlwaysOnTop(next);
    } catch (e) {
      console.error('toggle pin failed', e);
    }
  }
  import { EventTranslateResult, EventInputFill, EventWindowClosing } from '../utils/events';
  import type { TranslateResult } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
  import type {
    EngineListItem,
    NamedItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import { TRANSLATE_LANG, ALL_TRANSLATE_LANGS, type TranslateLang } from '../constants/lang';
  import { TranslateMulti } from '@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts';
  import { GetEngines } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import {
    GetLanguages,
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';

  let input = $state('');
  let engines = $state<EngineListItem[]>([]);
  let languages = $state<NamedItem[]>([]);
  let fromLang = $state<TranslateLang>(TRANSLATE_LANG.Auto);
  let toLang = $state<TranslateLang>(TRANSLATE_LANG.EN);
  // 各引擎翻译结果，按引擎名聚合（多引擎并发，逐个到达）。
  let results = $state<Record<string, TranslateResult>>({});
  let loading = $state(false);
  // 结果区自身高度范围：只缩放结果区，输入框/语言条/按钮等固定区域高度不变。
  // 高度直接测量真实渲染高度（临时放开结果区 → 读 scrollHeight → 精确设回），
  // 避免中英文/标点/长单词断行导致的估算偏差（这是之前翻译即出滚动条的根因）。
  const RESULT_MIN = 200;
  const RESULT_MAX = 720;
  // 后端 frameless 窗口设置了 InvisibleTitleBarHeight=36，SetSize 的 height 是窗口总高，
  // webview 内容区 = 窗口总高 - 该值。为保证内容全部可见，目标高度需补偿标题栏。
  const TITLE_BAR_H = 36;
  // webview 内 main 还有布局间距需补偿，否则窗口比内容矮 48px → 底部被裁出滚动条：
  // main 的 p-4（上下各 16 = 32）+ fixedEl 与 resultEl 之间的 gap-4（16）。
  const LAYOUT_EXTRA = 48;
  // 真实测量后再加一点点安全余量，防亚像素/字体加载导致的差一点点滚动条。
  const RESULT_BUF = 12;
  const DEFAULT_EXPANDED = 2; // 默认展开前 N 个引擎，其余折叠

  let fixedEl = $state<HTMLElement | null>(null);
  let resultEl = $state<HTMLElement | null>(null);
  // 结果区头部（标签行）：用于早期 return 判空；实际测量在 measureResultRealHeight 内用 resultEl 整体。
  let resultHeaderEl = $state<HTMLElement | null>(null);
  // 结果区动态高度：由引擎数量与译文长度计算，在 [RESULT_MIN, RESULT_MAX] 间。
  let resultH = $state(RESULT_MIN);
  // 缓存当前窗口宽度（只改高度，不改宽度）。
  let winW = $state(420);

  // 翻译中走马灯：动态省略号（. → .. → ... → .... 循环）
  let dotCount = $state(0);
  $effect(() => {
    if (!loading) {
      dotCount = 0;
      return;
    }
    const timer = setInterval(() => {
      dotCount = (dotCount + 1) % 4;
    }, 400);
    return () => clearInterval(timer);
  });

  const curLang = $derived(currentLang());

  const activeEngines = $derived(engines.filter((e) => e.kind === 'translate'));

  // 目标语言选项：系统翻译等引擎不支持自动检测目标语言，目标语言下拉框必须排除 auto。
  const targetLanguages = $derived(languages.filter((l) => l.value !== TRANSLATE_LANG.Auto));

  // 各引擎结果卡的展开状态：默认只展开前 DEFAULT_EXPANDED 个，其余折叠。
  let expanded = $state<Record<string, boolean>>({});
  // 引擎列表变化时，重置为默认展开前 N 个。
  $effect(() => {
    const next: Record<string, boolean> = {};
    activeEngines.slice(0, DEFAULT_EXPANDED).forEach((e) => {
      next[e.value] = true;
    });
    expanded = next;
  });
  function toggleExpand(engine: string) {
    expanded = { ...expanded, [engine]: !expanded[engine] };
    // 展开/折叠改变结果区高度，重算。
    adjustWindowHeight();
  }

  // 内容（输入/结果/loading）变化后，等下一帧布局稳定再按需调整窗口高度。
  $effect(() => {
    // 依赖：输入、结果、loading、引擎列表任意变化都触发重算。
    input;
    results;
    loading;
    activeEngines;
    if (resultEl) {
      tick().then(adjustWindowHeight);
    }
  });

  // 重入/排队防护：连续结果到达时若正在测量，标记 pending，测量结束后补一次。
  let adjusting = false;
  let pending = false;
  // 设计：窗口总高 = 固定区(语言条+输入卡片)真实高度 + 结果区高度 + 标题栏补偿 + 布局间距补偿。
  // 固定区高度恒定（输入卡片 min-h 已给足），只有翻译结果区随内容伸缩。
  //
  // 测量高度的核心难点：任何「读当前 DOM scrollHeight 再设回 resultH」的自引用都会形成正反馈
  // （flex 容器相互撑开 → scrollHeight 随 resultH 变大 → 一直加）。因此测量时必须把结果区临时
  // 设为 height:auto（脱离高度限制）+ visibility:hidden（防闪烁、不影响布局回流），读其自然内容
  // 真实高，再恢复。每次测量都从「自然内容高」起步，与历史 resultH 完全无关 → 值恒定、不累加。
  //
  // 测量方式：用「屏幕外克隆」彻底脱离当前 DOM 与 Svelte 的 style:height 绑定。
  // 直接操作真实 resultEl 的 style.height 会被 Svelte 的响应式绑定覆盖/干扰，
  // 导致读到的 scrollHeight 仍受旧 resultH 约束（偏小 → 窗口矮 → 滚动条）。
  // 克隆一个同宽、height:auto、屏幕外的副本读 scrollHeight，完全不受原元素高度限制。
  function measureResultRealHeight(): number {
    if (!resultEl) return RESULT_MIN;
    const clone = resultEl.cloneNode(true) as HTMLElement;
    clone.style.cssText = `
      position: fixed;
      top: -9999px;
      left: -9999px;
      width: ${resultEl.offsetWidth}px;
      height: auto;
      max-height: none;
      visibility: hidden;
      pointer-events: none;
      z-index: -1;
    `;
    document.body.appendChild(clone);
    const real = clone.scrollHeight;
    document.body.removeChild(clone);
    return real;
  }
  async function adjustWindowHeight() {
    if (!fixedEl || !resultEl || !resultHeaderEl) return;
    if (adjusting) {
      pending = true;
      return;
    }
    adjusting = true;
    try {
      // 等 expanded / 内容变化渲染完成后再测真实高度（此时临时 auto 测量，不依赖历史 resultH）。
      await tick();
      const realH = measureResultRealHeight();
      // 精确设回：clamp 到 [RESULT_MIN, RESULT_MAX]，加一点点安全余量。
      const ideal = Math.min(RESULT_MAX, Math.max(RESULT_MIN, realH + RESULT_BUF));
      resultH = ideal;
      // 真实 resultEl 高度由 JS 直接设置，不用 Svelte style:height 绑定，避免绑定覆盖/干扰。
      resultEl.style.height = `${ideal}px`;
      // 固定区真实高度：直接测语言条+输入卡片容器 offsetHeight（不随内容变，稳定可靠）。
      const fixedH = fixedEl.offsetHeight;
      // 窗口整体高度 = 固定区 + 结果区 + 标题栏补偿 + webview 内布局间距补偿。
      // 只让结果区参与伸缩；LAYOUT_EXTRA 补 main 的 p-4(32)+gap-4(16)，否则窗口矮 48px 出滚动条。
      const targetH = fixedH + resultH + TITLE_BAR_H + LAYOUT_EXTRA;
      console.debug(t('log.heightLogSetWindowHeight'), {
        窗口宽度: winW,
        固定区: fixedH,
        结果区: resultH,
        标题栏补偿: TITLE_BAR_H,
        布局间距: LAYOUT_EXTRA,
        目标高度: targetH,
      });
      try {
        await Window.SetSize(winW, targetH);
      } catch (e) {
        console.error(t('log.heightLogAdjustFailed'), e);
      }
    } finally {
      adjusting = false;
      if (pending) {
        pending = false;
        adjustWindowHeight();
      }
    }
  }

  onMount(() => {
    // 事件监听需先注册（await 加载期间若有翻译结果到达也不丢失）。
    const offResult = onEvent(EventTranslateResult, (payload: TranslateResult) => {
      if (payload && payload.engine) {
        results = { ...results, [payload.engine]: payload };
        loading = false;
        // 翻译结果出来：结果区内容变高，立刻重算窗口高度（动态）。
        adjustWindowHeight();
      }
    });
    const offInputFill = onEvent(EventInputFill, (text: string) => {
      if (!text) return;
      input = text;
      doTranslate();
    });
    const offClosing = onEvent(EventWindowClosing, () => {
      results = {};
      input = '';
      loading = false;
      // 清空后结果区缩回，重算高度。
      adjustWindowHeight();
    });
    // 初始化与首屏测量：必须等引擎加载完成再测高度，否则默认窗口算错。
    (async () => {
      // 恢复持久化的置顶状态 + 缓存窗口宽度（只改高度不改宽度）
      try {
        await Window.SetAlwaysOnTop($pinnedStore);
      } catch (e) {
        console.error(t('log.restorePinFailed'), e);
      }
      try {
        const size = await Window.Size();
        winW = size.width;
        console.debug(t('log.heightLogInitWidth'), winW);
      } catch (e) {
        console.error(t('log.inputLogReadSizeFailed'), e);
      }
      // 必须先等引擎/语言/默认值加载完（影响结果区占位卡片数量），否则首屏测量时
      // activeEngines 为空会走 RESULT_MIN 算出一个过矮的窗口，之后不一定能纠正。
      await Promise.all([loadDefaults(), loadEngines(), loadLanguages()]);
      // 首屏主动计算一次高度：等引擎列表渲染进结果区后，用真实测量得到准确窗口高。
      tick().then(async () => {
        await tick();
        console.debug(t('log.heightLogFirstAdjust'));
        adjustWindowHeight();
      });
      // 字体异步加载（如自定义字体）会改变文本实际高度，加载完成后再补一次，
      // 否则首屏测的偏矮 → 之后字体到位内容变高 → 滚动条。
      if (typeof document !== 'undefined' && document.fonts?.ready) {
        document.fonts.ready.then(() => adjustWindowHeight());
      }
    })();
    return () => {
      offResult();
      offInputFill();
      offClosing();
    };
  });

  // 从设置文件读取默认源/目标语言作为初始值（未配置回退 auto/zh）。
  async function loadDefaults() {
    try {
      const cfg = await GetConfig();
      if (cfg?.default_from) fromLang = cfg.default_from as TranslateLang;
      if (cfg?.default_to) toLang = cfg.default_to as TranslateLang;
    } catch (e) {
      console.error(t('log.readDefaultLangFailed'), e);
    }
  }

  // 持久化当前源/目标语言到设置文件。
  async function persistLangs() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, default_from: fromLang, default_to: toLang });
    } catch (e) {
      console.error(t('log.persistLangPrefFailed'), e);
    }
  }

  const fallbackLanguages = $derived<NamedItem[]>(
    ALL_TRANSLATE_LANGS.map((c) => ({
      value: c,
      name: langName(c),
    })),
  );

  async function loadEngines() {
    try {
      const list = await GetEngines();
      engines = list ?? [];
    } catch (e) {
      console.error(t('log.loadEngineListFailed'), e);
      engines = [];
    }
  }

  async function loadLanguages() {
    try {
      const list = await GetLanguages(curLang);
      languages = list?.length ? list : fallbackLanguages;
    } catch (e) {
      console.error(t('log.loadLangListFailed'), e);
      languages = fallbackLanguages;
    }
  }

  function swap() {
    // 复制键/源语言为「自动检测」时无法直接作为目标语言（目标下拉框无 auto 选项）。
    // 此时把源语言落到一个具体语言（zh）再交换，保证交换永远有可见效果，且 toLang 不会落到 auto。
    const from = fromLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : fromLang;
    const to = toLang === TRANSLATE_LANG.Auto ? TRANSLATE_LANG.ZH : toLang;
    fromLang = to;
    toLang = from;
    persistLangs();
  }

  async function doTranslate() {
    if (!input.trim() || activeEngines.length === 0) return;
    loading = true;
    results = {};
    // 开始翻译：结果区显示 loading 占位，立刻重算高度（动态结果区）。
    adjustWindowHeight();
    try {
      // 多引擎并发由后端按已开启引擎并行，不依赖单个 engine；
      // bindings 生成的 TranslateRequest.engine 为必填，传空串以满足类型（后端忽略）。
      await TranslateMulti({
        text: input,
        from: fromLang as TranslateLang,
        to: toLang as TranslateLang,
        engine: '',
      });
      // 结果通过 EventTranslateResult 逐个异步到达，loading 在收到首个结果时由模板判断解除。
    } catch (e) {
      console.error(t('log.translateRequestFailed'), e);
    } finally {
      // 兜底：若所有引擎都失败/无响应（后端不发送结果事件），最长 15s 后强制解除 loading。
      setTimeout(() => {
        if (Object.keys(results).length === 0) loading = false;
      }, 15000);
    }
  }

  let toast = $state('');
  let toastTimer: ReturnType<typeof setTimeout> | undefined;
  function showToast(msg: string) {
    toast = msg;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = ''), 1600);
  }

  async function copy(text: string) {
    if (!text) return;
    try {
      await Clipboard.SetText(text);
      showToast(t('common.copied'));
    } catch (e) {
      console.error('copy failed', e);
    }
  }

  function clearInput() {
    input = '';
    results = {};
    // 清空翻译：结果区缩回（无结果卡片），立刻重算窗口高度（动态结果区）。
    adjustWindowHeight();
  }
</script>

<div class="u-surface flex flex-col" style={rootStyleToStyle($rootStyle)}>
  <TitleBar />

  <main class="flex flex-col gap-4 overflow-hidden p-4">
    <!-- 固定区：语言条 + 输入卡片，高度恒定不变，只有翻译结果区随内容伸缩 -->
    <div bind:this={fixedEl} class="flex flex-col gap-4">
      <!-- 语言控制条 -->
      <div class="flex items-center justify-center gap-2">
        <select
          class="u-field u-select u-lang-select px-3 py-2 text-sm"
          bind:value={fromLang}
          onchange={persistLangs}
          aria-label={t('translate.from')}
        >
          {#each languages as l}
            <option value={l.value}>{langName(l.value)}</option>
          {/each}
        </select>

        <button
          class="u-icon-btn u-no-drag"
          onclick={swap}
          aria-label={t('translate.swap')}
          title={t('translate.swap')}
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
          bind:value={toLang}
          onchange={persistLangs}
          aria-label={t('translate.to')}
        >
          {#each targetLanguages as l}
            <option value={l.value}>{langName(l.value)}</option>
          {/each}
        </select>
      </div>

      <!-- 输入区 -->
      <section class="u-card u-card--panel flex flex-col overflow-hidden">
        <div class="u-border-b flex items-center justify-between px-3 py-2">
          <span class="u-label">{t('translate.from')}</span>
          <div class="flex items-center gap-2">
            {#if activeEngines.length === 0}
              <span class="u-muted text-xs">{t('translate.noActiveEngine')}</span>
            {:else}
              <span class="u-muted text-xs"
                >{activeEngines.length} · {t('translate.multiEngineHint')}</span
              >
            {/if}
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
          </div>
        </div>
        <textarea
          class="min-h-[120px] resize-none bg-transparent p-4 text-base leading-relaxed outline-none"
          bind:value={input}
          placeholder={t('translate.placeholder')}></textarea>
        <div class="u-border-t flex items-center justify-between px-3 py-2">
          <button class="u-btn u-btn--ghost u-no-drag px-3 py-1.5 text-sm" onclick={clearInput}>
            {t('translate.clearInput')}
          </button>
          <div class="flex items-center gap-2">
            <button
              class="u-icon-btn u-no-drag"
              onclick={() => copy(input)}
              aria-label={t('common.copy')}
              title={t('common.copy')}
              disabled={!input}
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
                <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
              </svg>
            </button>
            <button
              class="u-btn u-btn--primary u-no-drag px-5 py-1.5 text-sm"
              onclick={doTranslate}
              disabled={loading || !input.trim()}
            >
              {loading ? t('common.loading') : t('translate.button')}
            </button>
          </div>
        </div>
      </section>
    </div>
    <!-- 固定区结束 -->

    <!-- 结果区：每个已开启翻译引擎一张卡，并发到达；区域自身在 [RESULT_MIN, RESULT_MAX] 间自适应高度，超出内部滚动，不影响输入框等固定区 -->
    <section class="u-card u-card--panel flex flex-col overflow-hidden" bind:this={resultEl}>
      <div
        bind:this={resultHeaderEl}
        class="u-border-b flex items-center justify-between px-3 py-2"
      >
        <span class="u-label">{t('translate.result')}</span>
        {#if Object.keys(results).length > 0}
          <button
            class="u-icon-btn u-no-drag"
            onclick={() =>
              copy(
                Object.values(results)
                  .map((r) => r.result)
                  .join('\n\n'),
              )}
            aria-label={t('translate.copy')}
            title={t('translate.copy')}
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
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
            </svg>
          </button>
        {/if}
      </div>
      <div class="min-h-0 flex-1 overflow-y-auto p-4">
        {#if activeEngines.length === 0}
          <div class="flex h-full flex-col items-center justify-center gap-2 text-center">
            <svg
              class="u-muted"
              width="40"
              height="40"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M12 20h9" />
              <path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z" />
            </svg>
            <span class="u-muted text-sm">{t('translate.noActiveEngine')}</span>
          </div>
        {:else}
          <!-- 默认展示已开启翻译引擎卡片：未翻译时显示待翻译提示，翻译中显示 loading。
               默认展开前 DEFAULT_EXPANDED 个，其余折叠（仅显示引擎名，点击展开）。 -->
          <div class="flex flex-col gap-4">
            {#each activeEngines as e}
              {@const r = results[e.value]}
              {@const isOpen = !!expanded[e.value]}
              <div class="u-result-card">
                <div class="mb-2 flex items-center gap-2">
                  <span
                    class="rounded-full bg-[var(--app-accent)] px-2 py-0.5 text-xs font-medium text-[var(--app-accent-fg)]"
                  >
                    {engineName(e.value)}
                  </span>
                  {#if isOpen}
                    {#if r?.phonetic}
                      <span class="u-muted text-xs">{r.phonetic}</span>
                    {/if}
                    <span class="ml-auto flex items-center gap-2">
                      {#if r?.result}
                        <button
                          class="u-icon-btn u-icon-btn--sm u-no-drag"
                          onclick={(ev) => {
                            ev.stopPropagation();
                            copy(r.result);
                          }}
                          aria-label={t('translate.copy')}
                          title={t('translate.copy')}
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
                            <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                            <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                          </svg>
                        </button>
                      {/if}
                      <button
                        class="u-icon-btn u-icon-btn--sm u-no-drag"
                        onclick={(ev) => {
                          ev.stopPropagation();
                          toggleExpand(e.value);
                        }}
                        aria-label={t('translate.collapse')}
                        title={t('translate.collapse')}
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
                          <polyline points="18 15 12 9 6 15" />
                        </svg>
                      </button>
                    </span>
                  {:else}
                    <button
                      class="u-icon-btn u-icon-btn--sm u-no-drag ml-auto"
                      onclick={(ev) => {
                        ev.stopPropagation();
                        toggleExpand(e.value);
                      }}
                      aria-label={t('translate.expand')}
                      title={t('translate.expand')}
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
                        <polyline points="6 9 12 15 18 9" />
                      </svg>
                    </button>
                  {/if}
                </div>
                {#if isOpen}
                  {#if r}
                    <p class="whitespace-pre-wrap text-base leading-relaxed">{r.result}</p>
                  {:else if loading}
                    <div class="flex flex-col gap-2">
                      <p class="u-muted text-base leading-relaxed">
                        {t('common.loading')}<span class="kai-dots">{'.'.repeat(dotCount)}</span>
                      </p>
                      <div class="kai-loading-bar" aria-hidden="true"></div>
                    </div>
                  {/if}
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      </div>
    </section>
  </main>

  {#if toast}
    <div class="u-toast">{toast}</div>
  {/if}
</div>
