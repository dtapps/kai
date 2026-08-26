import { writable, derived, get } from 'svelte/store';
import { onEvent, System } from '../runtime';
import { EventLocaleChanged, EventThemeChanged, type ThemeChangedPayload } from '../utils/events';
import {
  GetTheme,
  SetTheme,
} from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
import { THEME, type ThemeMode, type ResolvedTheme } from '../constants/theme';

export const themeMode = writable<ThemeMode>(THEME.Auto);
export const systemDark = writable<boolean>(false);

export const resolvedTheme = derived([themeMode, systemDark], ([mode, sysDark]): ResolvedTheme => {
  if (mode === THEME.Auto) return sysDark ? THEME.Dark : THEME.Light;
  return mode as ResolvedTheme;
});

export const isDark = derived(resolvedTheme, ($r) => $r === THEME.Dark);

const lightVars: Record<string, string> = {
  '--app-bg': '#ffffff',
  '--app-fg': '#1f2329',
  '--app-muted': '#8a8f99',
  '--app-border': '#e5e6eb',
  '--app-card': '#f7f8fa',
  '--app-accent': '#3b82f6',
  '--app-accent-fg': '#ffffff',
  '--app-input-bg': '#ffffff',
};

const darkVars: Record<string, string> = {
  '--app-bg': '#18181c',
  '--app-fg': '#e6e6e6',
  '--app-muted': '#8a8f99',
  '--app-border': '#2c2c32',
  '--app-card': '#232328',
  '--app-accent': '#3b82f6',
  '--app-accent-fg': '#ffffff',
  '--app-input-bg': '#232328',
};

export const rootStyle = derived(isDark, ($dark) => ($dark ? darkVars : lightVars));

let unregister: Array<() => void> = [];

export async function initTheme(): Promise<void> {
  try {
    const mode = await GetTheme();
    if (mode === THEME.Auto || mode === THEME.Light || mode === THEME.Dark)
      themeMode.set(mode as ThemeMode);
    // 使用 @wailsio/runtime 原生 API 获取系统暗色模式，减少 RPC 调用
    const isDarkMode = await System.IsDarkMode();
    systemDark.set(isDarkMode);
  } catch {
    // ignore
  }

  applyClass();

  unregister.forEach((u) => u());
  unregister = [];
  unregister.push(
    onEvent(EventThemeChanged, (data: ThemeChangedPayload) => {
      // 单一主题事件：mode=用户配置模式，theme=系统真实外观。
      // 非 auto 用 mode；auto 时用 theme 跟随系统。
      if (data?.mode === THEME.Auto || data?.mode === THEME.Light || data?.mode === THEME.Dark) {
        themeMode.set(data.mode as ThemeMode);
      }
      if (data?.theme === THEME.Dark || data?.theme === THEME.Light) {
        systemDark.set(data.theme === THEME.Dark);
      }
      applyClass();
    }),
  );
  unregister.push(onEvent(EventLocaleChanged, () => applyClass()));
}

function applyClass(): void {
  const dark = get(isDark);
  document.documentElement.classList.toggle('dark', dark);
}

export async function setTheme(mode: ThemeMode): Promise<void> {
  themeMode.set(mode);
  applyClass();
  try {
    await SetTheme(mode);
  } catch {
    // ignore
  }
}
