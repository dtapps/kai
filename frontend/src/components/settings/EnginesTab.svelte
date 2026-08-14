<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
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
  let schema = $state<EngineFieldSchema[]>([]);
  let configValues = $state<Record<string, string>>({});

  // 多选字段（如 tesseract 语言码）的值解析/切换：值以 "+" 拼接，与后端 extra 兼容
  function parseOpts(val: string | undefined): Set<string> {
    return new Set(
      (val ?? '')
        .split('+')
        .map((s) => s.trim())
        .filter(Boolean),
    );
  }
  function toggleOpt(target: Record<string, string>, field: string, code: string) {
    const set = parseOpts(target[field]);
    if (set.has(code)) set.delete(code);
    else set.add(code);
    target[field] = [...set].join('+');
  }
  // system 引擎支持的语言列表（只读展示，由后端从 Translation.framework 读取）
  let systemLangs = $state<string[]>([]);
  let systemLangsLoading = $state(false);

  // 新增引擎弹层
  let showAdd = $state(false);
  let knownEngines = $state<EngineListItem[]>([]);
  let addName = $state('');
  let addSchema = $state<EngineFieldSchema[]>([]);
  let addValues = $state<Record<string, string>>({});
  let addError = $state('');
  // 右侧配置面板的校验错误提示（保存/启用失败原因）
  let formError = $state('');
  // 保存成功的轻量反馈（几秒后自动消失）
  let formOk = $state('');
  let formOkTimer: ReturnType<typeof setTimeout> | null = null;
  function flashOk(msg: string) {
    formOk = msg;
    if (formOkTimer) clearTimeout(formOkTimer);
    formOkTimer = setTimeout(() => (formOk = ''), 2500);
  }
  onDestroy(() => {
    if (formOkTimer) clearTimeout(formOkTimer);
  });

  onMount(() => {
    loadEngines();
  });

  async function loadEngines() {
    try {
      engines = (await GetAllEngines()) ?? [];
      if (engines.length && selectedId === null) selectEngine(engines[0].id);
    } catch (e) {
      console.error('[引擎] 加载引擎列表失败', e);
      engines = [];
    }
  }

  async function selectEngine(id: number) {
    selectedId = id;
    configValues = {};
    const eng = engines.find((e) => e.id === id);
    if (!eng) return;
    try {
      const s: EngineSchema = await GetEngineSchema(eng.value);
      schema = s.fields ?? [];
      // 拉取已持久化的完整配置，回填表单（服务地址 / API Key 等不再为空）
      let saved: EngineConfig | null = null;
      try {
        saved = await GetEngineConfig(id);
      } catch (e) {
        console.error('[引擎] 读取引擎配置失败', e);
        saved = null;
      }
      for (const f of schema) {
        configValues[f.field] = (saved?.[f.field as keyof typeof saved] as string) ?? '';
      }
    } catch (e) {
      console.error('[引擎] 加载引擎字段失败', e);
      schema = [];
    }
    // 选中系统翻译引擎时，拉取并显示其支持的语言列表（只读）
    if (eng.value === 'apple' && eng.supported) {
      loadSystemLangs();
    } else {
      systemLangs = [];
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
      console.error('[引擎] 读取系统语言失败', e);
      systemLangs = [];
    } finally {
      systemLangsLoading = false;
    }
  }

  // parseErr 把后端抛出的错误转换为可展示文案。
  // 后端必填校验错误形如「缺少必填项：settings.engine_field.xxx」，其中 key 走 i18n。
  function parseErr(e: unknown): string {
    const msg = e instanceof Error ? e.message : String(e);
    const prefix = '缺少必填项：';
    if (msg.startsWith(prefix)) {
      const key = msg.slice(prefix.length);
      return t('settings.engineMissing') + t(key);
    }
    return msg;
  }

  async function saveConfig() {
    const eng = engines.find((e) => e.id === selectedId);
    if (!eng) return;
    formError = '';
    try {
      await UpdateEngineConfig({
        id: eng.id,
        engine: eng.value,
        enabled: eng.enabled,
        api_key: configValues['api_key'] || undefined,
        secret: configValues['secret'] || undefined,
        endpoint: configValues['endpoint'] || undefined,
        extra: configValues['extra'] || undefined,
      });
      flashOk(t('settings.engineSaved'));
    } catch (e) {
      formError = parseErr(e);
    }
  }

  async function toggleEngine(id: number, enabled: boolean, el?: HTMLInputElement) {
    formError = '';
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
    } catch (e) {
      formError = parseErr(e);
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
    } catch (e) {
      console.error('[引擎] 删除引擎失败', e);
    }
  }

  // 打开新增引擎弹层（下拉来自 GetKnownEngines，列表只渲染数据库已有项）
  async function openAdd() {
    addError = '';
    showAdd = true;
    addName = '';
    addSchema = [];
    addValues = {};
    try {
      knownEngines = (await GetKnownEngines()) ?? [];
    } catch (e) {
      console.error('[引擎] 加载可选引擎列表失败', e);
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
    if (!name) {
      addSchema = [];
      return;
    }
    try {
      const s: EngineSchema = await GetEngineSchema(name);
      addSchema = s.fields ?? [];
      for (const f of addSchema) addValues[f.field] = f.default ?? '';
    } catch (e) {
      console.error('[引擎] 加载新增引擎字段失败', e);
      addSchema = [];
    }
  }

  async function submitAdd() {
    if (!addName) {
      addError = t('settings.engineAddSelect');
      return;
    }
    try {
      await AddEngine({
        id: 0,
        engine: addName,
        enabled: true,
        api_key: addValues['api_key'] || undefined,
        secret: addValues['secret'] || undefined,
        endpoint: addValues['endpoint'] || undefined,
        extra: addValues['extra'] || undefined,
      });
      showAdd = false;
      await loadEngines();
    } catch (e) {
      addError = String(e);
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
    {#if engines.find((e) => e.id === selectedId)?.value === 'apple'}
      <div
        class="mb-4 rounded-lg border border-[var(--app-border)] bg-[var(--app-surface-2)] px-4 py-3"
      >
        <p class="mb-2 text-xs font-medium text-[var(--app-text-muted)]">
          {t('settings.engineSystemLangs')}
        </p>
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
    {#if schema.length === 0}
      <div class="flex flex-1 flex-col items-center justify-center text-center">
        <p class="u-muted text-sm">{t('settings.engineNoSchema')}</p>
      </div>
    {:else}
      <div class="space-y-4">
        {#if formError}
          <p class="u-text-danger text-xs">{formError}</p>
        {/if}
        {#if formOk}
          <p class="u-text-ok text-xs">{formOk}</p>
        {/if}
        {#each schema as f}
          <div>
            <label class="mb-1.5 block text-sm font-medium" for={'ef-' + f.field}>
              {f.label_key ? t(f.label_key as any) : f.field}
            </label>
            {#if f.options && f.options.length}
              <div class="flex flex-wrap gap-1.5">
                {#each f.options as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={parseOpts(configValues[f.field]).has(code)}
                    onclick={() => toggleOpt(configValues, f.field, code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
              <p class="u-muted mt-1.5 text-xs">{t('settings.engineLangsHint')}</p>
            {:else}
              <input
                id={'ef-' + f.field}
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={configValues[f.field]}
              />
            {/if}
          </div>
        {/each}
        <div class="pt-2">
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
          <div>
            <label class="mb-1.5 block text-sm font-medium" for={'add-ef-' + f.field}>
              {f.label_key ? t(f.label_key as any) : f.field}
            </label>
            {#if f.options && f.options.length}
              <div class="flex flex-wrap gap-1.5">
                {#each f.options as code}
                  <button
                    type="button"
                    class="u-chip"
                    class:is-on={parseOpts(addValues[f.field]).has(code)}
                    onclick={() => toggleOpt(addValues, f.field, code)}
                  >
                    {langName(code)}
                  </button>
                {/each}
              </div>
            {:else}
              <input
                id={'add-ef-' + f.field}
                class="u-field w-full px-3 py-2 text-sm"
                placeholder={f.placeholder_key ? t(f.placeholder_key as any) : ''}
                bind:value={addValues[f.field]}
              />
            {/if}
          </div>
        {/each}
        {#if addError}
          <p class="u-text-danger text-xs">{addError}</p>
        {/if}
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
