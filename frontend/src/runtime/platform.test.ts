import { beforeEach, describe, expect, it, vi } from 'vitest';

// System API 在 node 环境不存在，用可变替身模拟「可用 / 抛错」两种状态。
const { system } = vi.hoisted(() => ({
  system: {
    IsMac: vi.fn<() => boolean>(),
    IsWindows: vi.fn<() => boolean>(),
    IsLinux: vi.fn<() => boolean>(),
  },
}));
vi.mock('@wailsio/runtime', () => ({ System: system }));

import { isMac, isWindows, isLinux } from './platform';

function setUA(ua: string) {
  Object.defineProperty(globalThis.navigator, 'userAgent', { value: ua, configurable: true });
}

beforeEach(() => {
  system.IsMac.mockReset();
  system.IsWindows.mockReset();
  system.IsLinux.mockReset();
});

describe('平台判断', () => {
  it('优先使用 Wails System API 的结果', () => {
    system.IsMac.mockReturnValue(true);
    system.IsWindows.mockReturnValue(false);
    system.IsLinux.mockReturnValue(false);
    setUA('Mozilla/5.0 (Windows NT 10.0)');

    expect(isMac()).toBe(true);
    expect(isWindows()).toBe(false);
    expect(isLinux()).toBe(false);
    // UA 不该被读取（System 可用时以它为准）
    expect(system.IsMac).toHaveBeenCalled();
  });

  it('System API 抛错时回退到 userAgent（截图窗口 _wails 为空的实际场景）', () => {
    for (const fn of [system.IsMac, system.IsWindows, system.IsLinux]) {
      fn.mockImplementation(() => {
        throw new Error('window._wails is undefined');
      });
    }
    setUA('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)');
    expect(isMac()).toBe(true);
    expect(isWindows()).toBe(false);
    expect(isLinux()).toBe(false);

    setUA('Mozilla/5.0 (Windows NT 10.0; Win64; x64)');
    expect(isWindows()).toBe(true);
    expect(isMac()).toBe(false);

    setUA('Mozilla/5.0 (X11; Linux x86_64)');
    expect(isLinux()).toBe(true);
    expect(isWindows()).toBe(false);
  });

  it('System 明确返回 false 时不因 UA 覆盖判断', () => {
    system.IsMac.mockReturnValue(false);
    setUA('Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)');
    expect(isMac()).toBe(false);
  });

  it('iOS/iPad UA 也算 mac 家族（仅回退路径）', () => {
    system.IsMac.mockImplementation(() => {
      throw new Error('boom');
    });
    setUA('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)');
    expect(isMac()).toBe(true);
  });
});
