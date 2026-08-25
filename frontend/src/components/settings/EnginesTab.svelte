<script lang="ts">
  import { onMount } from 'svelte';
  import { t, langName, engineName } from '../../i18n';
  import {
    GetAllEngines,
    GetKnownEngines,
    GetEngineSchema,
    GetEngineConfig,
    AddEngine,
    UpdateEngineConfig,
    ToggleEngineEnabled,
    RemoveEngine,
    CheckTesseract,
    GetOcrLangs,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts';
  import type {
    AllEngineItem,
    EngineListItem,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/models.ts';
  import type {
    EngineSchema,
    EngineFieldSchema,
    EngineConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/engine/models.ts';
  import { SystemLanguages } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { Dialogs } from '@wailsio/runtime';
  import { emitEvent } from '../../runtime';
  import { EventEnginesChanged } from '../../utils/events';
  let engines = $state<AllEngineItem[]>([]);

  // 按 Kind 分组展示：翻译引擎 / OCR 引擎（TTS 的 apple 引擎归入翻译类）
  type EngineGroup = { kind: string; title: string; items: AllEngineItem[] };
  const engineGroups = $derived.by<EngineGroup[]>(() => {
    const order = ['translate', 'ocr'];
    const map = new Map<string, EngineGroup>();
    for (const k of order) {
      map.set(k, {
        kind: k,
        title: t(k === 'ocr' ? 'settings.engineGroupOCR' : 'settings.engineGroupTranslate'),
        items: [],
      });
    }
    for (const e of engines) {
      const g = map.get(e.kind) ?? map.get('translate')!;
      g.items.push(e);
    }
    return order.map((k) => map.get(k)!).filter((g) => g.items.length > 0);
  });
  let selectedId = $state<number | null>(null);
  let schema = $state<EngineSchema | null>(null);
  let configValues = $state<Record<string, string>>({});

  // OCR 引擎（vision / tesseract）统一存放于 Extra(JSON) 的专属参数：
  //   - ocrLangs:     语言码多选（"+" 拼接，如 "chi_sim+eng"）
  //   - ocrCorrect:   是否开启语言校正（仅 vision 语义，tesseract 忽略）
  //   - ocrTimeoutSec: OCR 超时秒数（默认 60）
  //   - ocrRetry:     Vision OCR 失败兜底重试次数（仅 vision 语义，tesseract 忽略，默认 2）
  let ocrLangs = $state<string[]>([]);
  let ocrCorrect = $state(true);
  let ocrTimeoutSec = $state(60);
  let ocrRetry = $state(2);
  // OCR 语言码候选项（来自后端 GetOcrLangs，与 Extra(JSON) 解耦）
  let ocrLangOptions = $state<string[]>([]);

  function toggleOcrLang(code: string) {
    const set = new Set(ocrLangs);
    if (set.has(code)) set.delete(code);
    else set.add(code);
    ocrLangs = [...set];
  }
  // 解析引擎 extra(JSON) 中的 OCR 参数，回填到本地状态（vision / tesseract 共用）。
  // 兼容旧数据：extra 为纯字符串语言码（非 JSON）时，整串作为 langs 兜底。
  function loadOcrOpts(extra: string | undefined) {
    ocrLangs = [];
    ocrCorrect = true;
    ocrTimeoutSec = 60;
    ocrRetry = 2;
    if (!extra) return;
    // 先试 JSON（统一方案）
    try {
      const o = JSON.parse(extra);
      if (typeof o.langs === 'string' && o.langs) {
        ocrLangs = o.langs
          .split('+')
          .map((s: string) => s.trim())
          .filter(Boolean);
      }
      if (typeof o.correct_text === 'boolean') ocrCorrect = o.correct_text;
      if (typeof o.timeout_sec === 'number' && o.timeout_sec > 0) ocrTimeoutSec = o.timeout_sec;
      if (typeof o.retry_count === 'number' && o.retry_count > 0) ocrRetry = o.retry_count;
      return;
    } catch {
      /* 不是 JSON，落下面兼容旧纯字符串语言码 */
    }
    ocrLangs = extra
      .split('+')
      .map((s) => s.trim())
      .filter(Boolean);
  }
  // 将 OCR 参数合并进 extra(JSON)。langs 仅 tesseract 写入（vision 走系统 Vision 框架，无需语言码）；
  // correct 仅在 vision（isVision=true）时写入。
  function buildExtraWithOcr(extra: string | undefined, isVision: boolean): string {
    let o: Record<string, unknown> = {};
    if (extra) {
      try {
        o = JSON.parse(extra);
      } catch {
        o = {};
      }
    }
    if (!isVision) o.langs = ocrLangs.join('+');
    o.timeout_sec = ocrTimeoutSec;
    if (isVision) {
      o.correct_text = ocrCorrect;
      o.retry_count = ocrRetry;
    }
    return JSON.stringify(o);
  }
  // system 引擎支持的语言列表（只读展示，由后端从 Translation.framework 读取）
  let systemLangs = $state<string[]>([]);
  let systemLangsLoading = $state(false);
  // tesseract 安装探测结果（选中 tesseract 时刷新，供右侧展示「已安装/未安装」+ 路径/版本）
  let tesseract = $state<{ installed: boolean; path: string; version: string; os: string } | null>(
    null,
  );

  // 新增引擎弹层
  let showAdd = $state(false);
  let knownEngines = $state<EngineListItem[]>([]);
  let addName = $state('');
  let addSchema = $state<EngineFieldSchema[]>([]);
  let addValues = $state<Record<string, string>>({});

  // secret 字段明文/掩码切换状态（按字段 key 记录），便于查看已保存的密钥值。
  let revealed = $state<Record<string, boolean>>({});
  function toggleReveal(key: string) {
    revealed = { ...revealed, [key]: !revealed[key] };
  }

  onMount(() => {
    loadEngines();
  });

  async function loadOcrLangs() {
    try {
      ocrLangOptions = (await GetOcrLangs()) ?? [];
    } catch (e) {
      console.error(t('log.engineLoadOcrLangsFailed'), e);
      ocrLangOptions = [];
    }
  }

  async function loadEngines() {
    try {
      engines = (await GetAllEngines()) ?? [];
      if (engines.length && selectedId === null) selectEngine(engines[0].id);
    } catch (e) {
      console.error(t('log.engineLoadListFailed'), e);
      engines = [];
    }
  }

  async function selectEngine(id: number) {
    selectedId = id;
    configValues = {};
    const eng = engines.find((e) => e.id === id);
    if (!eng) return;
    // 拉取已持久化的完整配置，回填表单（服务地址 / API Key 等不再为空）
    let saved: EngineConfig | null = null;
    try {
      const s: EngineSchema = await GetEngineSchema(eng.value);
      schema = s;
      try {
        saved = await GetEngineConfig(id);
      } catch (e) {
        console.error(t('log.engineReadConfigFailed'), e);
        saved = null;
      }
      for (const f of schema.fields ?? []) {
        configValues[f.field] = (saved?.[f.field as keyof typeof saved] as string) ?? '';
      }
    } catch (e) {
      console.error(t('log.engineLoadFieldsFailed'), e);
      schema = null;
    }
    // 选中系统翻译引擎时，拉取并显示其支持的语言列表（只读）
    if (eng.value === 'apple' && eng.supported) {
      loadSystemLangs();
    } else {
      systemLangs = [];
    }
    // 选中 tesseract 时探测本机是否安装，供右侧展示安装状态
    if (eng.value === 'tesseract') {
      checkTesseract();
    } else {
      tesseract = null;
    }
    // 选中任意 OCR 引擎（vision / tesseract）时，回填其 extra(JSON) 中的 OCR 专属参数
    if (eng.kind === 'ocr') {
      await loadOcrLangs();
      loadOcrOpts(saved?.extra);
    }
  }

  // checkTesseract 调用后端探测本机 tesseract 安装情况
  async function checkTesseract() {
    try {
      tesseract = await CheckTesseract();
    } catch (e) {
      console.error(t('log.engineProbeTesseractFailed'), e);
      tesseract = { installed: false, path: '', version: '', os: '' };
    }
  }

  // loadSystemLangs 从后端读取系统翻译支持的语言（Translation.framework 已安装语言包）。
  async function loadSystemLangs() {
    systemLangsLoading = true;
    systemLangs = [];
    try {
      const langs = await SystemLanguages();
      systemLangs = Array.isArray(langs) ? langs : [];
    } catch (e) {
      console.error(t('log.engineReadSystemLangsFailed'), e);
      systemLangs = [];
    } finally {
      systemLangsLoading = false;
    }
  }

  // parseErr 把后端抛出的错误转换为可展示文案。
  // 后端必填校验错误形如「缺少必填项：settings.engine_field.xxx」，其中 key 走 i18n。
  function parseErr(e: unknown): string {
    const msg = e instanceof Error ? e.message : String(e);
    const prefix = t('settings.engineMissing');
    if (msg.startsWith(prefix)) {
      const key = msg.slice(prefix.length);
      return prefix + t(key);
    }
    return msg;
  }

  async function saveConfig() {
    const eng = engines.find((e) => e.id === selectedId);
    if (!eng) return;
    // OCR 引擎（vision / tesseract）：把 OCR 专属参数写回 extra(JSON) 再提交
    let extra = configValues['extra'] || undefined;
    if (eng.kind === 'ocr') {
      extra = buildExtraWithOcr(configValues['extra'], eng.value === 'vision');
    }
    try {
      await UpdateEngineConfig({
        id: eng.id,
        engine: eng.value,
        enabled: eng.enabled,
        api_key: configValues['api_key'] || undefined,
        secret: configValues['secret'] || undefined,
        endpoint: configValues['endpoint'] || undefined,
        extra,
      });
      await Dialogs.Info({
        Title: t('settings.engineSavedTitle'),
        Message: t('settings.engineSaved'),
      });
      // 保存可能改变启用态/配置，重新拉取并广播变更给翻译窗口等
      await loadEngines();
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  async function toggleEngine(id: number, enabled: boolean, el?: HTMLInputElement) {
    const eng = engines.find((x) => x.id === id);
    // 系统内置引擎（如 vision 系统 OCR / apple 系统翻译）可切换启用，但不可删除；
    // OCR 内置项切换时由后端保证 OCR 单选（自动禁用其它 OCR）。
    // 不支持当前平台的引擎（如 apple 仅 macOS）禁止开关。
    if (eng && !eng.supported) {
      if (el) el.checked = eng.enabled;
      engines = engines.map((x) => (x.id === id ? { ...x, enabled: eng.enabled } : x));
      return;
    }
    try {
      await ToggleEngineEnabled(id, enabled);
      // 成功：重新拉取以与后端保持一致
      await loadEngines();
      // 广播引擎变更，通知翻译窗口等重新拉取引擎列表
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
      // 回滚本地状态：校验失败时 checkbox 已被用户点动，Svelte 的 keyed each
      // 复用同一 DOM 节点不会主动撤销浏览器已翻动过的 checked，导致视觉卡住。
      // 因此直接用 DOM 把勾选态同步回滚，并同步 engines 数据源。
      const prev = !enabled;
      if (el) el.checked = prev;
      engines = engines.map((x) => (x.id === id ? { ...x, enabled: prev } : x));
    }
  }

  async function removeEngine(id: number) {
    try {
      await RemoveEngine(id);
      selectedId = null;
      await loadEngines();
      // 广播引擎变更，通知翻译窗口等重新拉取引擎列表
      emitEvent(EventEnginesChanged);
      await Dialogs.Info({
        Title: t('settings.engineSavedTitle'),
        Message: t('settings.engineRemoved'),
      });
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  // 打开新增引擎弹层（下拉来自 GetKnownEngines，列表只渲染数据库已有项）
  async function openAdd() {
    showAdd = true;
    addName = '';
    addSchema = [];
    addValues = {};
    try {
      knownEngines = (await GetKnownEngines()) ?? [];
    } catch (e) {
      console.error(t('log.engineLogOptionalListFailed'), e);
      knownEngines = [];
    }
  }

  function closeAdd() {
    showAdd = false;
  }

  function onAddOverlayKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeAdd();
    }
  }

  // 选择引擎类型后动态加载该引擎的字段 schema
  async function onAddNameChange(name: string) {
    addName = name;
    addValues = {};
    ocrLangs = [];
    ocrCorrect = true;
    ocrTimeoutSec = 60;
    ocrRetry = 2;
    if (!name) {
      addSchema = [];
      return;
    }
    try {
      const s: EngineSchema = await GetEngineSchema(name);
      addSchema = s.fields ?? [];
      for (const f of addSchema) addValues[f.field] = f.default ?? '';
      if (s.kind === 'ocr') {
        await loadOcrLangs();
      }
    } catch (e) {
      console.error(t('log.engineLoadAddFieldsFailed'), e);
      addSchema = [];
    }
  }

  async function submitAdd() {
    if (!addName) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: t('settings.engineAddSelect'),
      });
      return;
    }
    // OCR 引擎：把 OCR 专属参数（langs/timeout）拼进 extra(JSON) 再提交
    let extra = addValues['extra'] || undefined;
    const addSchemaKind = addSchema.length ? addSchema[0] : null;
    if (addSchemaKind && (await addEngineIsOcr(addName))) {
      extra = buildExtraWithOcr(addValues['extra'], addName === 'vision');
    }
    try {
      await AddEngine({
        id: 0,
        engine: addName,
        enabled: false,
        api_key: addValues['api_key'] || undefined,
        secret: addValues['secret'] || undefined,
        endpoint: addValues['endpoint'] || undefined,
        extra,
      });
      showAdd = false;
      await loadEngines();
      // 广播引擎变更，通知翻译窗口等重新拉取引擎列表
      emitEvent(EventEnginesChanged);
    } catch (e) {
      await Dialogs.Error({
        Title: t('settings.engineOpErrorTitle'),
        Message: parseErr(e),
      });
    }
  }

  // addEngineIsOcr 判断新增的引擎类型是否为 OCR（按后端 KnownEngines 的 kind）。
  async function addEngineIsOcr(name: string): Promise<boolean> {
    try {
      const s: EngineSchema = await GetEngineSchema(name);
      return s.kind === 'ocr';
    } catch {
      return false;
    }
  }
</script>

<header class="mb-6">
  <h1 class="text-2xl font-semibold">{t('settings.enginesTitle')}</h1>
</header>

<div class="flex min-h-[360px] gap-5">
  <!-- Engine list -->
  <div class="u-card u-card--panel flex w-56 flex-col p-4">
    <div class="mb-3 flex items-center justify-between px-1">
      <span class="u-label">{t('settings.engineList')}</span>
      <button
        class="u-btn u-btn--ghost px-2 py-0.5 text-xs"
        onclick={openAdd}
        aria-label={t('settings.engineAdd')}>＋</button
      >
    </div>
    <ul class="flex-1 space-y-3 overflow-y-auto">
      {#each engineGroups as group (group.kind)}
        <li class="space-y-1">
          <div class="u-label px-1 pb-0.5 text-[11px] opacity-70">{group.title}</div>
          {#each group.items as e (e.id)}
            <div class="u-list-item" class:is-active={selectedId === e.id}>
              <button
                class="flex-1 bg-transparent text-left text-sm font-medium"
                onclick={() => selectEngine(e.id)}
              >
                {engineName(e.value)}
              </button>
              {#if !e.supported}
                <span class="u-muted text-[10px] leading-tight text-right max-w-[3.5rem]"
                  >{t('settings.engineUnsupported')}</span
                >
              {:else}
                <label class="u-switch" aria-label={t('settings.engineEnabled')}>
                  <input
                    type="checkbox"
                    checked={e.enabled}
                    onchange={(ev) =>
                      toggleEngine(
                        e.id,
                        (ev.target as HTMLInputElement).checked,
                        ev.target as HTMLInputElement,
                      )}
                  />
                  <span class="u-switch__track">
                    <span class="u-switch__thumb"></span>
                  </span>
                </label>
              {/if}
            </div>
          {/each}
        </li>
      {/each}
    </ul>
    {#if engines.length === 0}
      <p class="u-muted px-1 pt-2 text-xs">{t('settings.engineListEmpty')}</p>
    {/if}
  </div>

  <!-- Engine config -->
  <div class="u-card u-card--panel flex flex-1 flex-col p-5">
    <div class="u-label mb-4">
      {t('settings.engineConfig')}
    </div>
    {#if schema?.builtin}
      <div class="flex items-start gap-2 rounded-md border u-border-ok px-3 py-2 text-xs">
        <span class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full u-bg-ok"></span>
        <div>
          <p class="font-medium">
            {#if schema?.kind === 'ocr'}
              {t('settings.engine_tip.vision_builtin')}
            {:else}
              {t('settings.engine_tip.apple_builtin')}
            {/if}
          </p>
          <p class="u-muted">
            {#if schema?.kind === 'ocr'}
              {t('settings.engine_tip.vision_builtin_desc')}
            {:else}
              {t('settings.engine_tip.apple_builtin_desc')}
            {/if}
          </p>
        </div>
      </div>
    {/if}
    {#if (schema?.fields?.length ?? 0) === 0}
      <div class="flex flex-1 flex-col items-center justify-center text-center">
        <p class="u-muted text-sm">{t('settings.engineNoSchema')}</p>
      </div>
    {:else}
      <div class="u-card mt-4 space-y-4 px-4 py-3">
        {#if schema?.kind === 'translate' && schema?.builtin}
          <div>
            <p class="u-label mb-2">{t('settings.engineSystemLangs')}</p>
            {#if systemLangsLoading}
              <p class="u-muted text-xs">{t('common.loading')}</p>
            {:else if systemLangs.length}
              <div class="flex flex-wrap gap-1.5">
                {#each systemLangs as code}
                  <span class="u-tag">{langName(code)}</span>
                {/each}
              </div>
            {:else}
              <p class="u-muted text-xs">{t('settings.engineSystemLangsEmpty')}</p>
            {/if}
          </div>
        {/if}
        {#each schema?.fields ?? [] as f}
          {#if f.widget === 'ocr_status'}
            <!-- tesseract 安装状态探测卡（含可编辑自定义二进制路径 endpoint），仅 tesseract schema 声明 -->
            {#if tesseract}
              <div
                class="flex flex-col gap-2 rounded-md border px-3 py-2 text-xs"
                class:u-border-danger={!tesseract.installed}
                class:u-border-ok={tesseract.installed}
              >
                <div class="flex items-start gap-2">
                  <span
                    class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full"
                    class:u-bg-danger={!tesseract.installed}
                    class:u-bg-ok={tesseract.installed}
                  ></span>
                  <div>
                    {#if tesseract.installed}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_installed')}</p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_path')}{tesseract.path}
                      </p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_version')}{tesseract.version || '-'}
                      </p>
                    {:else}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_missing')}</p>
                      {#if tesseract.os === 'darwin'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_mac')}
                        </p>
                      {:else if tesseract.os === 'windows'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_windows')}
                        </p>
                      {:else}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_linux')}
                        </p>
                      {/if}
                    {/if}
                  </div>
                </div>
                <div class="mt-1">
                  <label class="mb-1 block text-xs font-medium" for={'ef-' + f.field}>
                    {f.label_key ? t(f.label_key as any) : f.field}
                  </label>
                  <input
                    id={'ef-' + f.field}
                    type="text"
                    class="u-field w-full px-3 py-1.5 text-xs"
                    placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                    bind:value={configValues[f.field]}
                  />
                </div>
              </div>
            {/if}
          {:else if f.widget === 'ocr_langs'}
            <!-- OCR 识别语言多选（仅 tesseract），候选项取 ocrLangOptions -->
            <div>
              <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
              {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              <div class="mt-2 flex flex-wrap gap-1.5">
                {#each ocrLangOptions as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={ocrLangs.includes(code)}
                    onclick={() => toggleOcrLang(code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
            </div>
          {:else if f.widget === 'ocr_timeout'}
            <!-- OCR 超时（秒） -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="5"
                max="300"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrTimeoutSec}
              />
            </div>
          {:else if f.widget === 'ocr_retry'}
            <!-- OCR 失败重试次数（仅 vision） -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="0"
                max="10"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrRetry}
              />
            </div>
          {:else if f.widget === 'ocr_correct'}
            <!-- OCR 语言校正开关（仅 vision） -->
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <label class="u-switch" aria-label={f.label_key ? t(f.label_key as any) : f.field}>
                <input type="checkbox" bind:checked={ocrCorrect} />
                <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
              </label>
            </div>
          {:else if f.type === 'secret'}
            <!-- 密钥类字段：支持明文/掩码切换，便于查看已保存的值 -->
            {@const revealKey = 'ef-reveal-' + f.field}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <div class="relative">
                <input
                  id={'ef-' + f.field}
                  type={revealed[revealKey] ? 'text' : 'password'}
                  class="u-field w-full px-3 py-2 pr-10 text-sm"
                  placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                  bind:value={configValues[f.field]}
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 u-muted px-1 text-xs"
                  aria-label={revealed[revealKey] ? t('settings.hideSecret') : t('settings.showSecret')}
                  onclick={() => toggleReveal(revealKey)}
                >
                  {revealed[revealKey] ? '🙈' : '👁'}
                </button>
              </div>
            </div>
          {:else}
            <!-- 普通文本字段 -->
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={configValues[f.field]}
              />
            </div>
          {/if}
        {/each}
        <div class="u-border-t pt-4">
          <button class="u-btn u-btn--primary px-5 py-2 text-sm" onclick={saveConfig}>
            {t('settings.engineSave')}
          </button>
        </div>
      </div>
    {/if}
    {#if selectedId !== null}
      {@const sel = engines.find((x) => x.id === selectedId)}
      <div class="mt-auto pt-4">
        {#if sel?.builtin}
          <span class="u-muted text-xs">{t('settings.engineBuiltinHint')}</span>
        {:else}
          <button
            class="u-btn u-btn--link-danger text-sm"
            onclick={() => {
              if (selectedId !== null) removeEngine(selectedId);
            }}
          >
            {t('settings.engineRemove')}
          </button>
        {/if}
      </div>
    {/if}
  </div>
</div>

{#if showAdd}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
    role="button"
    tabindex="0"
    aria-label={t('settings.engineAddClose')}
    onclick={(e) => {
      if (e.target === e.currentTarget) closeAdd();
    }}
    onkeydown={onAddOverlayKeydown}
  >
    <div
      class="u-card u-card--panel w-[420px] max-w-[90vw] p-5"
      role="dialog"
      aria-modal="true"
      aria-label={t('settings.engineAddTitle')}
    >
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-base font-semibold">{t('settings.engineAddTitle')}</h2>
        <button class="text-sm" onclick={() => (showAdd = false)} aria-label={t('common.close')}
          >✕</button
        >
      </div>
      <div class="space-y-4">
        <div>
          <label class="mb-1.5 block text-sm font-medium" for="add-engine-type"
            >{t('settings.engineAddType')}</label
          >
          <select
            id="add-engine-type"
            class="u-field u-select w-full px-3 py-2 text-sm"
            value={addName}
            onchange={(e) => onAddNameChange((e.target as HTMLSelectElement).value)}
          >
            <option value="">{t('settings.engineAddSelectPlaceholder')}</option>
            {#each knownEngines as k}
              <option value={k.value}>{engineName(k.value)}</option>
            {/each}
          </select>
        </div>
        {#each addSchema as f}
          {#if f.widget === 'ocr_status'}
            {#if tesseract}
              <div
                class="flex flex-col gap-2 rounded-md border px-3 py-2 text-xs"
                class:u-border-danger={!tesseract.installed}
                class:u-border-ok={tesseract.installed}
              >
                <div class="flex items-start gap-2">
                  <span
                    class="mt-0.5 inline-block h-2 w-2 shrink-0 rounded-full"
                    class:u-bg-danger={!tesseract.installed}
                    class:u-bg-ok={tesseract.installed}
                  ></span>
                  <div>
                    {#if tesseract.installed}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_installed')}</p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_path')}{tesseract.path}
                      </p>
                      <p class="u-muted break-all">
                        {t('settings.engine_tip.tesseract_version')}{tesseract.version || '-'}
                      </p>
                    {:else}
                      <p class="font-medium">{t('settings.engine_tip.tesseract_missing')}</p>
                      {#if tesseract.os === 'darwin'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_mac')}
                        </p>
                      {:else if tesseract.os === 'windows'}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_windows')}
                        </p>
                      {:else}
                        <p class="u-muted break-all">
                          {t('settings.engine_tip.tesseract_install_linux')}
                        </p>
                      {/if}
                    {/if}
                  </div>
                </div>
                <div class="mt-1">
                  <label class="mb-1 block text-xs font-medium" for={'add-ef-' + f.field}>
                    {f.label_key ? t(f.label_key as any) : f.field}
                  </label>
                  <input
                    id={'add-ef-' + f.field}
                    type="text"
                    class="u-field w-full px-3 py-1.5 text-xs"
                    placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                    bind:value={addValues[f.field]}
                  />
                </div>
              </div>
            {/if}
          {:else if f.widget === 'ocr_langs'}
            <div>
              <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
              {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              <div class="mt-2 flex flex-wrap gap-1.5">
                {#each ocrLangOptions as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={ocrLangs.includes(code)}
                    onclick={() => toggleOcrLang(code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
            </div>
          {:else if f.widget === 'ocr_timeout'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="5"
                max="300"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrTimeoutSec}
              />
            </div>
          {:else if f.widget === 'ocr_retry'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <input
                type="number"
                min="0"
                max="10"
                class="u-field w-28 px-3 py-1.5 text-sm"
                bind:value={ocrRetry}
              />
            </div>
          {:else if f.widget === 'ocr_correct'}
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm font-medium">{f.label_key ? t(f.label_key as any) : f.field}</p>
                {#if f.hint_key}<p class="u-muted text-xs">{t(f.hint_key as any)}</p>{/if}
              </div>
              <label class="u-switch" aria-label={f.label_key ? t(f.label_key as any) : f.field}>
                <input type="checkbox" bind:checked={ocrCorrect} />
                <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
              </label>
            </div>
          {:else if f.type === 'secret'}
            <!-- 密钥类字段：支持明文/掩码切换，便于查看已保存的值 -->
            {@const revealKey = 'add-ef-reveal-' + f.field}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <div class="relative">
                <input
                  id={'add-ef-' + f.field}
                  type={revealed[revealKey] ? 'text' : 'password'}
                  class="u-field w-full px-3 py-2 pr-10 text-sm"
                  placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                  bind:value={addValues[f.field]}
                />
                <button
                  type="button"
                  class="absolute right-2 top-1/2 -translate-y-1/2 u-muted px-1 text-xs"
                  aria-label={revealed[revealKey] ? t('settings.hideSecret') : t('settings.showSecret')}
                  onclick={() => toggleReveal(revealKey)}
                >
                  {revealed[revealKey] ? '🙈' : '👁'}
                </button>
              </div>
            </div>
          {:else}
            <div>
              <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
                {f.label_key ? t(f.label_key as any) : f.field}
              </label>
              <input
                id={'add-ef-' + f.field}
                type="text"
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={addValues[f.field]}
              />
            </div>
          {/if}
        {/each}
        <div class="flex justify-end gap-2 pt-1">
          <button class="u-btn u-btn--ghost px-4 py-1.5 text-sm" onclick={() => (showAdd = false)}
            >{t('common.cancel')}</button
          >
          <button class="u-btn u-btn--primary px-4 py-1.5 text-sm" onclick={submitAdd}
            >{t('settings.engineAddConfirm')}</button
          >
        </div>
      </div>
    </div>
  </div>
{/if}
