import { beforeEach, describe, expect, it, vi } from 'vitest';

const { events, windowApi } = vi.hoisted(() => ({
  events: { On: vi.fn(), Emit: vi.fn() },
  windowApi: { Name: vi.fn() },
}));
vi.mock('@wailsio/runtime', () => ({ Events: events, Window: windowApi, System: {} }));

import { getWindowName, onEvent, emitEvent } from './index';

beforeEach(() => {
  events.On.mockReset();
  events.Emit.mockReset();
  windowApi.Name.mockReset();
});

describe('runtime 包装', () => {
  it('getWindowName 透出 Window.Name()', async () => {
    windowApi.Name.mockResolvedValue('settings');
    await expect(getWindowName()).resolves.toBe('settings');
  });

  it('onEvent 只把 payload 的 data 交给回调（不是整个 WailsEvent）', () => {
    const unsub = vi.fn();
    events.On.mockReturnValue(unsub);
    const cb = vi.fn();

    const off = onEvent('kai:test', cb);
    expect(events.On).toHaveBeenCalledWith('kai:test', expect.any(Function));

    // 模拟 Wails 事件对象结构
    const handler = events.On.mock.calls[0][1] as (e: unknown) => void;
    handler({ name: 'kai:test', data: { a: 1 } });
    expect(cb).toHaveBeenCalledWith({ a: 1 });

    expect(off).toBe(unsub);
  });

  it('emitEvent 原样转发事件名与数据', () => {
    emitEvent('kai:analytics:track', { event: 'x' });
    expect(events.Emit).toHaveBeenCalledWith('kai:analytics:track', { event: 'x' });

    emitEvent('kai:noop');
    expect(events.Emit).toHaveBeenLastCalledWith('kai:noop', undefined);
  });
});
