import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

const { onEvent, handlers, getTheme, setThemeRPC, system, windowApi } = vi.hoisted(() => {
  const handlers = new Map<string, (data: unknown) => void>();
  return {
    handlers,
    onEvent: vi.fn((name: string, cb: (data: unknown) => void) => {
      handlers.set(name, cb);
      return () => {
        handlers.delete(name);
      };
    }),
    getTheme: vi.fn(),
    setThemeRPC: vi.fn(),
    system: { IsDarkMode: vi.fn() },
    windowApi: { SetSystemDefaultTheme: vi.fn(), SetDarkTheme: vi.fn(), SetLightTheme: vi.fn() },
  };
});

vi.mock('../runtime', () => ({ onEvent, System: system, Window: windowApi }));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetTheme: getTheme,
  SetTheme: setThemeRPC,
}));

import {
  themeMode,
  systemDark,
  resolvedTheme,
  isDark,
  rootStyle,
  initTheme,
  setTheme,
} from './theme';
import { THEME } from '../constants/theme';
import { EventLocaleChanged, EventThemeChanged } from '../utils/events';

// theme 会操作真实 DOM 的 documentElement.classList，node 环境需打桩。
const toggle = vi.fn();

beforeEach(() => {
  onEvent.mockClear();
  handlers.clear();
  getTheme.mockReset();
  setThemeRPC.mockReset();
  system.IsDarkMode.mockReset();
  toggle.mockClear();

  Object.defineProperty(globalThis, 'document', {
    value: { documentElement: { classList: { toggle } } },
    configurable: true,
    writable: true,
  });

  themeMode.set(THEME.Auto);
  systemDark.set(false);
});

describe('主题解析', () => {
  it('auto 跟随系统：系统深色即深色', () => {
    themeMode.set(THEME.Auto);
    systemDark.set(true);
    expect(get(resolvedTheme)).toBe(THEME.Dark);
    expect(get(isDark)).toBe(true);
  });

  it('auto 跟随系统：系统浅色即浅色', () => {
    themeMode.set(THEME.Auto);
    systemDark.set(false);
    expect(get(resolvedTheme)).toBe(THEME.Light);
    expect(get(isDark)).toBe(false);
  });

  it('显式模式忽略系统外观', () => {
    systemDark.set(true);
    themeMode.set(THEME.Light);
    expect(get(resolvedTheme)).toBe(THEME.Light);

    systemDark.set(false);
    themeMode.set(THEME.Dark);
    expect(get(resolvedTheme)).toBe(THEME.Dark);
  });

  it('rootStyle 随主题切换出不同的 CSS 变量', () => {
    themeMode.set(THEME.Light);
    const light = get(rootStyle);
    themeMode.set(THEME.Dark);
    const dark = get(rootStyle);

    expect(light['--app-bg']).not.toBe(dark['--app-bg']);
    expect(Object.keys(light).sort()).toEqual(Object.keys(dark).sort());
  });
});

describe('setTheme()', () => {
  it('更新状态、切换根节点 class 并持久化到后端', async () => {
    setThemeRPC.mockResolvedValue(undefined);
    await setTheme(THEME.Dark);

    expect(get(themeMode)).toBe(THEME.Dark);
    expect(toggle).toHaveBeenLastCalledWith('dark', true);
    expect(setThemeRPC).toHaveBeenCalledWith(THEME.Dark);
  });

  it('后端持久化失败不影响界面（本地已生效）', async () => {
    setThemeRPC.mockRejectedValue(new Error('rpc down'));
    await expect(setTheme(THEME.Light)).resolves.toBeUndefined();
    expect(get(themeMode)).toBe(THEME.Light);
    expect(toggle).toHaveBeenLastCalledWith('dark', false);
  });
});

describe('initTheme()', () => {
  it('读取后端模式与系统深色偏好，并同步原生标题栏', async () => {
    getTheme.mockResolvedValue(THEME.Dark);
    system.IsDarkMode.mockResolvedValue(true);

    await initTheme();

    expect(get(themeMode)).toBe(THEME.Dark);
    expect(get(systemDark)).toBe(true);
    expect(toggle).toHaveBeenLastCalledWith('dark', true);
    expect(windowApi.SetDarkTheme).toHaveBeenCalled();
  });

  it('后端读不到配置时不影响初始化，且仍应用主题', async () => {
    getTheme.mockRejectedValue(new Error('rpc down'));
    system.IsDarkMode.mockRejectedValue(new Error('no system api'));

    await expect(initTheme()).resolves.toBeUndefined();
    expect(toggle).toHaveBeenCalled();
  });

  it('订阅主题事件：mode 决定用户模式，theme 决定系统外观', async () => {
    getTheme.mockResolvedValue(THEME.Auto);
    system.IsDarkMode.mockResolvedValue(false);
    await initTheme();

    const onTheme = handlers.get(EventThemeChanged);
    expect(onTheme).toBeTypeOf('function');
    onTheme!({ mode: THEME.Auto, theme: THEME.Dark });
    expect(get(systemDark)).toBe(true);
    expect(get(resolvedTheme)).toBe(THEME.Dark);
    expect(toggle).toHaveBeenLastCalledWith('dark', true);

    onTheme!({ mode: THEME.Light, theme: THEME.Dark });
    expect(get(themeMode)).toBe(THEME.Light);
    expect(get(resolvedTheme)).toBe(THEME.Light);
  });

  it('忽略非法事件值（防止后端脏数据把主题打崩）', async () => {
    getTheme.mockResolvedValue(THEME.Light);
    system.IsDarkMode.mockResolvedValue(false);
    await initTheme();

    handlers.get(EventThemeChanged)!({ mode: 'rainbow', theme: 'neon' });
    expect(get(themeMode)).toBe(THEME.Light);
    expect(get(systemDark)).toBe(false);
  });

  it('语言变更时重新应用根节点 class', async () => {
    getTheme.mockResolvedValue(THEME.Light);
    await initTheme();
    toggle.mockClear();

    handlers.get(EventLocaleChanged)!({ language: 'en-US' });
    expect(toggle).toHaveBeenCalled();
  });

  it('重复 initTheme 不累积订阅', async () => {
    getTheme.mockResolvedValue(THEME.Auto);
    await initTheme();
    await initTheme();

    expect(onEvent).toHaveBeenCalledTimes(4); // 每轮 init 订阅 2 个（主题 + 语言），两轮共 4 次
    expect(handlers.size).toBe(2);
  });
});
