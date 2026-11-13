<script lang="ts">
  import { onMount } from 'svelte';
  import { t, locale, resolveLang } from '../../i18n';
  import { userLang } from '../../stores/ui';
  import { themeMode, setTheme } from '../../stores/theme';
  import {
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { Lang, type LangCode } from '../../constants/lang';
  import { THEME, type ThemeMode } from '../../constants/theme';
  import { track } from '../../utils/analytics';

  let { curLang = $bindable<LangCode>(Lang.ZHCN) }: { curLang: LangCode } = $props();

  // 匿名统计开关：初始值从配置读取，变更经 SaveConfig 落盘（Go 侧 analytics 据此决定是否上报）。
  let analyticsEnabled = $state(false);
  // httplog.db 请求日志保留天数：0 表示不清理；变更后 Go 侧立即清理并同步定时任务。
  let httpLogRetention = $state(7);
  // 更新设置：预发布通道 / 自动检查间隔（小时，0 = 关闭）/ 更新完自动重启。
  // 均由 Go 侧在运行时实时读取（每次检测/每轮调度），保存后立即生效，无需重启应用。
  type UpdaterField = 'prerelease' | 'auto_restart';
  let updaterPrerelease = $state(true);
  let updaterAutoRestart = $state(true);
  let updaterInterval = $state(6);

  const themeOptions = $derived.by<{ mode: ThemeMode; label: string }[]>(() => [
    { mode: THEME.Auto, label: t('settings.themeAuto') },
    { mode: THEME.Light, label: t('settings.themeLight') },
    { mode: THEME.Dark, label: t('settings.themeDark') },
  ]);

  onMount(async () => {
    try {
      const cfg = await GetConfig();
      if (cfg) {
        analyticsEnabled = cfg.analytics_enabled ?? false;
        httpLogRetention = cfg.http_log?.retention_days ?? 7;
        const u = cfg.updater;
        if (u) {
          updaterPrerelease = u.prerelease ?? true;
          updaterAutoRestart = u.auto_restart ?? true;
          updaterInterval = u.check_interval_hours ?? 6;
        }
      }
    } catch {
      /* 忽略读取失败，回退默认关 */
    }
  });

  async function changeLang(l: LangCode) {
    curLang = l; // 下拉高亮保留原始 mode（auto/zh-CN/en-US）
    userLang.set(l); // 同步用户 mode，刷新后下拉框仍选中该项
    locale.set(resolveLang(l)); // i18n 实际生效语言立即解析，避免 auto 时显示 key
    try {
      const cfg = (await GetConfig()) ?? ({ language: l } as any);
      await SaveConfig({ ...cfg, language: l });
    } catch (e) {
      console.error(t('log.generalSaveLangFailed'), e);
    }
  }

  async function changeTheme(m: ThemeMode) {
    await setTheme(m);
  }

  async function toggleAnalytics(e: Event) {
    const enabled = (e.target as HTMLInputElement).checked;
    analyticsEnabled = enabled;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, analytics_enabled: enabled });
      track('feature_toggled', { feature: 'anonymous_analytics', enabled });
    } catch (err) {
      console.error(t('log.generalSaveAnalyticsFailed'), err);
    }
  }

  // 保存 httplog 保留天数：非法/负数按 0（不清理）处理，并回填规范化后的值。
  async function changeHttpLogRetention(e: Event) {
    const input = e.target as HTMLInputElement;
    // 空值不视为 0，回填原值避免误把清理关掉。
    if (input.value.trim() === '') {
      input.value = String(httpLogRetention);
      return;
    }
    const days = Math.max(0, Math.floor(Number(input.value) || 0));
    httpLogRetention = days;
    input.value = String(days);
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, http_log: { ...cfg.http_log, retention_days: days } });
    } catch (err) {
      console.error(t('log.generalSaveHttpLogRetentionFailed'), err);
    }
  }

  // 保存单个更新开关：始终基于后端最新配置整段写回，避免覆盖其它设置。
  async function toggleUpdater(field: UpdaterField, e: Event) {
    const enabled = (e.target as HTMLInputElement).checked;
    if (field === 'prerelease') updaterPrerelease = enabled;
    else updaterAutoRestart = enabled;
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({ ...cfg, updater: { ...(cfg.updater ?? {}), [field]: enabled } } as any);
      track('feature_toggled', { feature: `updater_${field}`, enabled });
    } catch (err) {
      console.error(t('log.generalSaveUpdaterFailed'), err);
      // 保存失败：回滚开关显示，避免界面与 settings.json 不一致。
      if (field === 'prerelease') updaterPrerelease = !enabled;
      else updaterAutoRestart = !enabled;
    }
  }

  // 保存自动检查间隔（小时，0 = 关闭）：非法/负数按 0 处理，并回填规范化后的值。
  async function changeUpdaterInterval(e: Event) {
    const input = e.target as HTMLInputElement;
    // 空值不视为 0，回填原值避免误把自动检查关掉。
    if (input.value.trim() === '') {
      input.value = String(updaterInterval);
      return;
    }
    const hours = Math.max(0, Math.floor(Number(input.value) || 0));
    updaterInterval = hours;
    input.value = String(hours);
    try {
      const cfg = (await GetConfig()) ?? ({} as any);
      await SaveConfig({
        ...cfg,
        updater: { ...(cfg.updater ?? {}), check_interval_hours: hours },
      } as any);
      track('feature_toggled', { feature: 'updater_check_interval', enabled: hours > 0 });
    } catch (err) {
      console.error(t('log.generalSaveUpdaterFailed'), err);
    }
  }
</script>

<header class="mb-8">
  <h1 class="text-2xl font-semibold">{t('settings.generalTitle')}</h1>
  <p class="u-muted mt-1 text-sm">{t('settings.interface')}</p>
</header>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.language')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.languageHint')}</p>
  <div class="flex items-center">
    <select
      id="lang-sel"
      class="u-field u-select w-full max-w-[240px] px-3 py-2 text-sm"
      value={curLang}
      onchange={(e) => changeLang((e.target as HTMLSelectElement).value as LangCode)}
    >
      <option value={Lang.Auto}>{t('lang.auto')}</option>
      <option value={Lang.ZHCN}>{t('lang.zh-CN')}</option>
      <option value={Lang.ENUS}>{t('lang.en')}</option>
    </select>
  </div>
</div>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-3 text-sm font-medium">{t('settings.theme')}</div>
  <div class="u-segment">
    {#each themeOptions as opt}
      <button
        class="u-segment__item"
        class:is-active={$themeMode === opt.mode}
        onclick={() => changeTheme(opt.mode)}
      >
        {opt.label}
      </button>
    {/each}
  </div>
</div>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.analytics')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.analyticsHint')}</p>
  <label class="u-switch" aria-label={t('settings.analytics')}>
    <input type="checkbox" checked={analyticsEnabled} onchange={toggleAnalytics} />
    <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
  </label>
</div>

<div class="u-card u-card--panel mb-5 p-5">
  <div class="mb-1 text-sm font-medium">{t('settings.httpLogRetention')}</div>
  <p class="u-muted mb-3 text-xs">{t('settings.httpLogRetentionHint')}</p>
  <div class="flex items-center gap-2">
    <input
      class="u-field w-24 px-3 py-2 text-sm"
      type="number"
      min="0"
      step="1"
      value={httpLogRetention}
      onchange={changeHttpLogRetention}
      aria-label={t('settings.httpLogRetention')}
    />
    <span class="u-muted text-sm">{t('settings.httpLogRetentionUnit')}</span>
  </div>
</div>

<div class="u-card u-card--panel p-5">
  <div class="mb-4 text-sm font-medium">{t('settings.updater')}</div>

  <div class="flex items-start justify-between gap-6">
    <div>
      <div class="text-sm">{t('settings.updaterPrerelease')}</div>
      <p class="u-muted mt-1 text-xs">{t('settings.updaterPrereleaseHint')}</p>
    </div>
    <label class="u-switch shrink-0" aria-label={t('settings.updaterPrerelease')}>
      <input
        type="checkbox"
        checked={updaterPrerelease}
        onchange={(e) => toggleUpdater('prerelease', e)}
      />
      <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
    </label>
  </div>

  <div class="u-border-t mt-4 pt-4">
    <div class="text-sm">{t('settings.updaterCheckInterval')}</div>
    <p class="u-muted mt-1 text-xs">{t('settings.updaterCheckIntervalHint')}</p>
    <div class="mt-2 flex items-center gap-2">
      <input
        class="u-field w-24 px-3 py-2 text-sm"
        type="number"
        min="0"
        step="1"
        value={updaterInterval}
        onchange={changeUpdaterInterval}
        aria-label={t('settings.updaterCheckInterval')}
      />
      <span class="u-muted text-sm">{t('settings.updaterCheckIntervalUnit')}</span>
    </div>
  </div>

  <div class="u-border-t mt-4 flex items-start justify-between gap-6 pt-4">
    <div>
      <div class="text-sm">{t('settings.updaterAutoRestart')}</div>
      <p class="u-muted mt-1 text-xs">{t('settings.updaterAutoRestartHint')}</p>
    </div>
    <label class="u-switch shrink-0" aria-label={t('settings.updaterAutoRestart')}>
      <input
        type="checkbox"
        checked={updaterAutoRestart}
        onchange={(e) => toggleUpdater('auto_restart', e)}
      />
      <span class="u-switch__track"><span class="u-switch__thumb"></span></span>
    </label>
  </div>
</div>
