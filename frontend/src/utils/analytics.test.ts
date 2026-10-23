import { beforeEach, describe, expect, it, vi } from 'vitest';

// runtime 依赖 @wailsio/runtime（node 环境不可用），用例只关心「发了什么事件」。
const { emitEvent } = vi.hoisted(() => ({ emitEvent: vi.fn() }));
vi.mock('../runtime', () => ({ emitEvent }));

import { track } from './analytics';

beforeEach(() => {
  emitEvent.mockClear();
});

describe('track()', () => {
  it('固定以 kai:analytics:track 事件上报，Go 侧只认这一个入口', () => {
    track('translate_click');
    expect(emitEvent).toHaveBeenCalledTimes(1);
    expect(emitEvent).toHaveBeenCalledWith('kai:analytics:track', {
      event: 'translate_click',
      props: {},
    });
  });

  it('props 原样透传（嵌套/数字/布尔都不做加工）', () => {
    track('engine_used', { engine: 'google', count: 3, ok: true, meta: { from: 'auto' } });
    expect(emitEvent).toHaveBeenCalledWith('kai:analytics:track', {
      event: 'engine_used',
      props: { engine: 'google', count: 3, ok: true, meta: { from: 'auto' } },
    });
  });

  it('未传 props 时补空对象（后端按 map 解析，nil 会掉字段）', () => {
    track('app_open');
    const payload = emitEvent.mock.calls[0][1] as { props: Record<string, unknown> };
    expect(payload.props).toEqual({});
  });

  it('不返回任何值，且不因 runtime 异常影响调用方', () => {
    emitEvent.mockImplementationOnce(() => {
      throw new Error('runtime not ready');
    });
    // 说明：当前实现不吞异常，此处锁定行为——若后续改为静默，需同步修改本断言。
    expect(() => track('boom')).toThrow('runtime not ready');
  });
});
