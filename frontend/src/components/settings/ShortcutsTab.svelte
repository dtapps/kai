<script lang="ts">
  import { onMount } from 'svelte';
  import { t } from '../../i18n';
  import {
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import {
    CheckAccessibility,
    OpenAccessibilitySettings,
    CheckScreenRecording,
    OpenScreenRecordingSettings,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts';
  import { Dialogs } from '@wailsio/runtime';
  import { isMac as detectMac } from '../../runtime/platform';

  // 单个注册类快捷键的表单形态（按键 + 启用状态），与后端 HotkeyEntry 对齐。
  type HotkeyEntry = { key: string; enabled: boolean };
  // 单个执行类快捷键的表单形态，与后端 ExecKeyEntry 对齐（执行键是独立分类，不混用 HotkeyEntry）。
  type ExecKeyEntry = { key: string; enabled: boolean; fallback: boolean };
  // 注册类快捷键表单：可编辑副本（2 项）
  type HotkeyForm = {
    input: HotkeyEntry;
    screenshot: HotkeyEntry;
  };
  // 执行类快捷键表单（独立分类）：当前仅复制键。
  type ExecKeyForm = {
    copy: ExecKeyEntry;
  };
  // 注册类快捷键默认值（与后端 DefaultSettings.Hotkeys 保持一致）。
  const defaultHotkeys: HotkeyForm = {
    input: { key: 'Alt+A', enabled: true },
    screenshot: { key: 'Alt+S', enabled: false },
  };
  // 执行类快捷键默认值（与后端 DefaultSettings.ExecKeys 保持一致）。
  const defaultExecKeys: ExecKeyForm = {
    copy: { key: 'Cmd+C', enabled: true, fallback: true },
  };

  // 快捷键表单：可编辑副本（注册类 2 项 + 复制键）
  // 初始值预填各快捷键的真实默认值，避免未加载时输入框为空。
  let hotkeyForm = $state<HotkeyForm>({
    input: { ...defaultHotkeys.input },
    screenshot: { ...defaultHotkeys.screenshot },
  });
  let execKeyForm = $state<ExecKeyForm>({
    copy: { ...defaultExecKeys.copy },
  });
  // 录制态：正在捕获按键的快捷键字段名（null 表示未录制）。注册键与执行键分两个分类。
  type RecordableKey = keyof HotkeyForm | keyof ExecKeyForm;
  let recordingKey = $state<RecordableKey | null>(null);

  // 授权卡片仅 macOS 需要（辅助功能 / 屏幕录制均为 macOS TCC 权限）。
  // Windows 复制键走 makc 调用 user32.dll、全局热键走 RegisterHotKey，均无需用户授权；
  // Linux 同样无需此类授权。故非 Mac 平台直接隐藏授权区块。
  // 使用统一的平台判断（Wails v3 多窗口下 _wails 可能丢失，需 UA 兜底）。
  const isMac = detectMac();
  console.debug(t('log.shortcutsTabIsMac'), isMac);

  // 辅助功能授权状态（macOS）：true=已授权，false=未授权，null=检测中/未知（非 darwin 始终 true）
  let accGranted = $state<boolean | null>(null);
  let accLoading = $state(false);

  async function loadAccessibility() {
    accLoading = true;
    try {
      accGranted = await CheckAccessibility();
    } catch (e) {
      console.error(t('log.shortcutCheckAccessibilityFailed'), e);
      accGranted = null;
    } finally {
      accLoading = false;
    }
  }

  async function openAccessibility() {
    try {
      await OpenAccessibilitySettings();
      // 弹窗后稍等用户操作，再刷新一次状态
      setTimeout(loadAccessibility, 800);
    } catch (e) {
      console.error(t('log.shortcutOpenAccessibilityFailed'), e);
    }
  }

  // 屏幕录制授权状态（截图翻译依赖）：true=已授权，false=未授权，null=检测中/未知（非 darwin 始终 true）
  let srGranted = $state<boolean | null>(null);
  let srLoading = $state(false);

  async function loadScreenRecording() {
    srLoading = true;
    try {
      srGranted = await CheckScreenRecording();
    } catch (e) {
      console.error(t('log.shortcutCheckScreenRecordingFailed'), e);
      srGranted = null;
    } finally {
      srLoading = false;
    }
  }

  async function openScreenRecording() {
    try {
      await OpenScreenRecordingSettings();
      // 弹窗后稍等用户操作，再刷新一次状态
      setTimeout(loadScreenRecording, 800);
    } catch (e) {
      console.error(t('log.shortcutOpenScreenRecordingFailed'), e);
    }
  }

  // 加载快捷键页所需的全部授权状态。仅拉数据，不改变展开/收起状态，
  // 以免手动刷新时把用户展开着的卡片强制收起。
  async function loadShortcutPermissions() {
    await Promise.all([loadAccessibility(), loadScreenRecording()]);
    // 仅首次加载完成后设置一次初始展开状态：都已授权则默认收起，否则展开。
    if (!permInitDone) {
      permInitDone = true;
      permExpanded = !(accGranted === true && srGranted === true);
    }
  }

  // 授权区块展开状态：默认折叠。仅在页面首次加载（首次数据返回）时按授权状态
  // 决定一次初始值；之后的刷新与手动展开/收起都由用户控制，不被重置。
  let permExpanded = $state(false);
  let permInitDone = false; // 初始展开状态是否已设置，确保只设一次

  // 授权是否都已明确授予（辅助功能 + 屏幕录制），仅用于模板折叠判断。
  let permAllGranted = $derived(accGranted === true && srGranted === true);

  // 用 e.code 取主键，避免 macOS Option(Alt) 组合键把 e.key 变成组合字符（如 Alt+S → 'ß'）
  function keyName(e: KeyboardEvent): string {
    if (e.code?.startsWith('Key')) return e.code.slice(3); // KeyS -> S
    if (e.code?.startsWith('Digit')) return e.code.slice(5); // Digit1 -> 1
    if (e.code === 'Space') return 'Space';
    const k = e.key;
    if (k === ' ' || k === 'Spacebar') return 'Space';
    if (k === 'Control' || k === 'Alt' || k === 'Shift' || k === 'Meta') return '';
    if (k.length === 1) return k.toUpperCase();
    return k;
  }

  function onHotkeyKeydown(e: KeyboardEvent) {
    if (!recordingKey) return;
    e.preventDefault();
    if (e.key === 'Escape') {
      recordingKey = null;
      return;
    }
    const mods: string[] = [];
    if (e.altKey) mods.push('Alt');
    if (e.ctrlKey) mods.push('Ctrl');
    if (e.metaKey) mods.push('Cmd');
    if (e.shiftKey) mods.push('Shift');
    const main = keyName(e);
    if (!main) return; // 仅按修饰键，等待主键
    const combo = [...mods, main].join('+');
    const k = recordingKey;
    recordingKey = null; // 先置空，防止组合键产生的重复 keydown 再次写入
    if (k) {
      if (k === 'copy') {
        execKeyForm.copy.key = combo;
      } else {
        hotkeyForm[k].key = combo;
      }
    }
  }

  function startRecord(key: keyof HotkeyForm | 'copy') {
    recordingKey = key;
  }

  onMount(() => {
    loadShortcuts();
    loadShortcutPermissions();
  });

  async function loadShortcuts() {
    try {
      const cfg = await GetConfig();
      if (cfg) {
        const hk = cfg.hotkeys ?? {};
        hotkeyForm = {
          input: { key: hk.input?.key ?? '', enabled: hk.input?.enabled ?? false },
          screenshot: { key: hk.screenshot?.key ?? '', enabled: hk.screenshot?.enabled ?? false },
        };
        const ek = cfg.execkeys ?? {};
        execKeyForm = {
          copy: {
            key: ek.copy?.key ?? '',
            enabled: ek.copy?.enabled ?? false,
            fallback: ek.copy?.fallback ?? true,
          },
        };
      }
    } catch (e) {
      console.error(t('log.shortcutLoadConfigFailed'), e);
    }
  }

  async function saveShortcuts() {
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      const next = {
        ...cfg,
        hotkeys: {
          input: hotkeyForm.input,
          screenshot: hotkeyForm.screenshot,
        },
        execkeys: { copy: execKeyForm.copy },
      };
      await SaveConfig(next as any);
      // 桌面软件用 Wails v3 原生信息对话框，与失败错误框风格统一、更醒目
      await Dialogs.Info({
        Title: t('settings.hkSavedTitle'),
        Message: t('settings.hkSaved'),
      });
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      // 桌面软件用 Wails v3 原生错误对话框，比内联文字更醒目
      await Dialogs.Error({
        Title: t('settings.hkSaveErrorTitle'),
        Message: msg,
      });
    }
  }
</script>

<svelte:window onkeydown={onHotkeyKeydown} />

<header class="mb-6">
  <h1 class="text-2xl font-semibold">{t('settings.shortcutsTitle')}</h1>
  <p class="u-muted mt-1 text-sm">{t('settings.shortcutsHint')}</p>
</header>

<!-- 快捷键所需授权状态：仅 macOS 需要（辅助功能 + 屏幕录制均为 macOS TCC 权限） -->
{#if isMac}
  <div class="u-card u-card--panel mb-5 p-5">
    {#if permAllGranted && !permExpanded}
      <!-- 折叠态：都已授权时默认收起，仅显示概览一行 -->
      <div class="flex items-center justify-between gap-4">
        <div class="flex items-center gap-2">
          <span class="text-sm font-medium">{t('settings.permTitle')}</span>
          <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <button class="u-btn u-btn--ghost px-3 py-1.5 text-sm" onclick={loadShortcutPermissions}>
            {t('settings.accRefresh')}
          </button>
          <button
            class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
            onclick={() => (permExpanded = true)}
          >
            {t('settings.permExpand')}
          </button>
        </div>
      </div>
    {:else}
      <!-- 展开态：显示明细 + 刷新/收起 -->
      <div class="mb-1 flex items-center justify-between gap-4">
        <div class="text-sm font-medium">{t('settings.permTitle')}</div>
        <div class="flex shrink-0 items-center gap-2">
          {#if permAllGranted}
            <button
              class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
              onclick={() => (permExpanded = false)}
            >
              {t('settings.permCollapse')}
            </button>
          {/if}
          <button class="u-btn u-btn--ghost px-3 py-1.5 text-sm" onclick={loadShortcutPermissions}>
            {t('settings.accRefresh')}
          </button>
        </div>
      </div>
      <p class="u-muted mb-4 text-xs">{t('settings.permHint')}</p>

      <div class="space-y-4">
        <!-- 辅助功能 -->
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-sm font-medium">{t('settings.permAccessibility')}</div>
            <p class="u-muted text-xs">{t('settings.permAccessibilityHint')}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if accGranted === null}
              <span class="u-muted text-sm">{accLoading ? t('common.loading') : '—'}</span>
            {:else if accGranted}
              <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
            {:else}
              <span class="u-text-warn text-sm font-medium">{t('settings.accDenied')}</span>
            {/if}
            <button class="u-btn u-btn--primary px-3 py-1.5 text-sm" onclick={openAccessibility}>
              {t('settings.accOpen')}
            </button>
          </div>
        </div>

        <!-- 屏幕录制：截图翻译依赖 -->
        <div class="flex items-center justify-between gap-4">
          <div class="min-w-0">
            <div class="text-sm font-medium">{t('settings.permScreenRecording')}</div>
            <p class="u-muted text-xs">{t('settings.permScreenRecordingHint')}</p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            {#if srGranted === null}
              <span class="u-muted text-sm">{srLoading ? t('common.loading') : '—'}</span>
            {:else if srGranted}
              <span class="u-text-ok text-sm font-medium">{t('settings.accGranted')}</span>
            {:else}
              <span class="u-text-warn text-sm font-medium">{t('settings.accDenied')}</span>
            {/if}
            <button class="u-btn u-btn--primary px-3 py-1.5 text-sm" onclick={openScreenRecording}>
              {t('settings.accOpen')}
            </button>
          </div>
        </div>
      </div>
    {/if}
  </div>
{/if}

<div class="u-card u-card--panel space-y-5 p-6">
  {#each [{ key: 'input', label: t('settings.hkInput') }, { key: 'screenshot', label: t('settings.hkScreenshot') }] satisfies { key: keyof HotkeyForm; label: string }[] as row}
    <div class="flex items-center justify-between gap-4">
      <label class="text-sm font-medium" for={'hk-' + row.key}>{row.label}</label>
      <div class="flex items-center gap-2">
        {#if recordingKey === row.key}
          <span class="u-field w-56 px-3 py-1.5 text-sm u-text-warn"
            >{t('settings.hkRecording')}</span
          >
        {:else}
          <input
            id={'hk-' + row.key}
            class="u-field w-56 px-3 py-1.5 text-sm"
            placeholder={defaultHotkeys[row.key].key}
            bind:value={hotkeyForm[row.key].key}
          />
        {/if}
        <button
          type="button"
          class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
          class:is-active={recordingKey === row.key}
          onclick={() => startRecord(row.key)}>{t('settings.hkRecord')}</button
        >
        <label class="u-switch" aria-label={t('settings.enabled')}>
          <input type="checkbox" bind:checked={hotkeyForm[row.key].enabled} />
          <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
        </label>
      </div>
    </div>
  {/each}
  <div class="flex items-center justify-between gap-4 border-t pt-4">
    <label class="text-sm font-medium" for="hk-copy">{t('settings.hkCopy')}</label>
    <div class="flex items-center gap-2">
      {#if recordingKey === 'copy'}
        <span class="u-field w-56 px-3 py-1.5 text-sm u-text-warn">{t('settings.hkRecording')}</span
        >
      {:else}
        <input
          id="hk-copy"
          class="u-field w-56 px-3 py-1.5 text-sm"
          placeholder={defaultExecKeys.copy.key}
          bind:value={execKeyForm.copy.key}
        />
      {/if}
      <button
        type="button"
        class="u-btn u-btn--ghost px-3 py-1.5 text-sm"
        class:is-active={recordingKey === 'copy'}
        onclick={() => startRecord('copy')}>{t('settings.hkRecord')}</button
      >
      <label class="u-switch" aria-label={t('settings.enabled')}>
        <input type="checkbox" bind:checked={execKeyForm.copy.enabled} />
        <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
      </label>
      <label class="u-switch" aria-label={t('settings.hkCopyFallback')}>
        <input type="checkbox" bind:checked={execKeyForm.copy.fallback} />
        <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
      </label>
    </div>
  </div>
  <p class="u-muted text-xs">{t('settings.hkFormatHint')}</p>
  <div class="flex justify-end pt-1">
    <button class="u-btn u-btn--primary px-4 py-1.5 text-sm" onclick={saveShortcuts}>
      {t('settings.hkSave')}
    </button>
  </div>
</div>
