import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

// runtime 与 RPC binding 在 node 环境不可用，全部替身化。
const { onEvent, handlers, getConfig } = vi.hoisted(() => {
  const handlers = new Map<string, (data: unknown) => void>();
  return {
    handlers,
    onEvent: vi.fn((name: string, cb: (data: unknown) => void) => {
      handlers.set(name, cb);
      return () => {
        handlers.delete(name);
      };
    }),
    getConfig: vi.fn(),
  };
});

vi.mock('../runtime', () => ({ onEvent }));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: getConfig,
}));

import { initWindow, userLang, activeWindow, currentLang } from './ui';
import { EventLocaleChanged } from '../utils/events';
import { Lang } from '../constants/lang';

beforeEach(() => {
  onEvent.mockClear();
  handlers.clear();
  userLang.set(Lang.Auto);
});

describe('initWindow()', () => {
  it('读取配置里的界面语言并同步 userLang 与生效语言', async () => {
    getConfig.mockResolvedValue({ language: Lang.ENUS });
    await initWindow();

    expect(get(userLang)).toBe(Lang.ENUS);
    expect(currentLang()).toBe(Lang.ENUS);
  });

  it('auto 模式下 userLang 保留 auto（下拉框要显示"自动"），生效语言才被解析', async () => {
    getConfig.mockResolvedValue({ language: Lang.Auto });
    await initWindow();

    expect(get(userLang)).toBe(Lang.Auto);
    expect([Lang.ZHCN, Lang.ENUS]).toContain(currentLang() as string);
  });

  it('配置读取失败时静默降级，不阻塞窗口初始化', async () => {
    getConfig.mockRejectedValue(new Error('RPC not ready'));
    await expect(initWindow()).resolves.toBeUndefined();
    expect(get(userLang)).toBe(Lang.Auto);
  });

  it('配置缺少 language 字段时保持默认', async () => {
    getConfig.mockResolvedValue({});
    await initWindow();
    expect(get(userLang)).toBe(Lang.Auto);
  });

  it('订阅语言变更事件，收到后同步 userLang 与生效语言', async () => {
    getConfig.mockResolvedValue({ language: Lang.ZHCN });
    await initWindow();
    expect(onEvent).toHaveBeenCalledWith(EventLocaleChanged, expect.any(Function));

    const handler = handlers.get(EventLocaleChanged);
    expect(handler).toBeTypeOf('function');
    handler!({ language: Lang.ENUS });

    expect(get(userLang)).toBe(Lang.ENUS);
    expect(currentLang()).toBe(Lang.ENUS);
  });

  it('事件 payload 缺少 language 时忽略（不清空当前设置）', async () => {
    getConfig.mockResolvedValue({ language: Lang.ENUS });
    await initWindow();

    handlers.get(EventLocaleChanged)!({});
    expect(get(userLang)).toBe(Lang.ENUS);
  });

  it('重复调用不会累积订阅（先退订旧的再订新的）', async () => {
    getConfig.mockResolvedValue({ language: Lang.ZHCN });
    await initWindow();
    await initWindow();

    expect(onEvent).toHaveBeenCalledTimes(2);
    expect(handlers.size).toBe(1);
  });
});

describe('模块导出', () => {
  it('activeWindow 初始为空串，由窗口入口自行写入', () => {
    expect(get(activeWindow)).toBe('');
  });

  it('currentLang() 始终返回解析后的具体语言', () => {
    expect([Lang.ZHCN, Lang.ENUS]).toContain(currentLang() as string);
  });
});
