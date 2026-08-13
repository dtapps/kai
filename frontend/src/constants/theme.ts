// 主题模式常量（auto 跟随系统 / light 浅色 / dark 深色）。
// 与后端 internal/settings 的 ThemeAuto/ThemeLight/ThemeDark 对齐，
// 避免在各组件散落 'auto' / 'light' / 'dark' 等裸字符串，便于前后端同步修改。

export const THEME = {
  Auto: 'auto',
  Light: 'light',
  Dark: 'dark',
} as const;

// 用户可配置的主题模式（auto / light / dark）。
export type ThemeMode = (typeof THEME)[keyof typeof THEME];

// 实际生效的主题（解析 auto 后只剩 light / dark）。
export type ResolvedTheme = typeof THEME.Light | typeof THEME.Dark;
