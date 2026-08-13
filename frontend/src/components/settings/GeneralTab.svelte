<script lang="ts">
  import { t, locale, resolveLang } from '../../i18n';
  import { themeMode, setTheme } from '../../stores/theme';
  import {
    GetConfig,
    SaveConfig,
  } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
  import { Lang, type LangCode } from '../../constants/lang';
  import { THEME, type ThemeMode } from '../../constants/theme';

  let { curLang = $bindable<LangCode>(Lang.ZHCN) }: { curLang: LangCode } = $props();

  const themeOptions = $derived.by<{ mode: ThemeMode; label: string }[]>(() => [
    { mode: THEME.Auto, label: t('settings.themeAuto') },
    { mode: THEME.Light, label: t('settings.themeLight') },
    { mode: THEME.Dark, label: t('settings.themeDark') },
  ]);

  async function changeLang(l: LangCode) {
    curLang = l; // 下拉高亮保留原始 mode（auto/zh-CN/en-US）
    locale.set(resolveLang(l)); // i18n 实际生效语言立即解析，避免 auto 时显示 key
    try {
      const cfg = (await GetConfig()) ?? ({ language: l } as any);
      await SaveConfig({ ...cfg, language: l });
    } catch (e) {
      console.error('[通用] 保存语言设置失败', e);
    }
  }

  async function changeTheme(m: ThemeMode) {
    await setTheme(m);
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

<div class="u-card u-card--panel p-5">
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
