import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';

const { frontendLog } = vi.hoisted(() => ({ frontendLog: vi.fn() }));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/logutil/frontendlogservice.ts', () => ({
  FrontendLog: frontendLog,
}));

// 保留真实 console，用例结束后还原（installFrontendLogging 会全局改写 console）。
const originals = {
  log: console.log,
  info: console.info,
  warn: console.warn,
  error: console.error,
  debug: console.debug,
};

// window 桩：只关心事件监听注册。
const listeners = new Map<string, (e: unknown) => void>();
Object.defineProperty(globalThis, 'window', {
  value: {
    addEventListener: (name: string, cb: (e: unknown) => void) => {
      listeners.set(name, cb);
    },
  },
  configurable: true,
  writable: true,
});

// 记录真实 console 的输出（转发不可吞掉终端可见性）。
const printed: string[] = [];
const silent =
  () =>
  (...args: unknown[]) => {
    printed.push(args.map((a) => String(a)).join(' '));
  };

beforeEach(() => {
  vi.resetModules(); // 重置模块级 installed 标记，让每个用例都能重新安装
  frontendLog.mockReset();
  frontendLog.mockResolvedValue(undefined);
  listeners.clear();
  printed.length = 0;

  console.log = silent();
  console.info = silent();
  console.warn = silent();
  console.error = silent();
  console.debug = silent();
});

afterAll(() => {
  console.log = originals.log;
  console.info = originals.info;
  console.warn = originals.warn;
  console.error = originals.error;
  console.debug = originals.debug;
});

describe('installFrontendLogging()', () => {
  it('console.log/info → info 级别，且原始输出不被吞掉', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    console.log('hello', 'world');
    expect(printed).toContain('hello world');
    expect(frontendLog).toHaveBeenCalledWith('info', 'hello world');

    console.info('info-msg');
    expect(frontendLog).toHaveBeenLastCalledWith('info', 'info-msg');
  });

  it('warn / error / debug 映射到对应级别', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    console.warn('w');
    expect(frontendLog).toHaveBeenLastCalledWith('warn', 'w');

    console.error('e');
    expect(frontendLog).toHaveBeenLastCalledWith('error', 'e');

    console.debug('d');
    expect(frontendLog).toHaveBeenLastCalledWith('debug', 'd');
  });

  it('非字符串参数序列化为 JSON，循环引用不抛异常', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    console.log({ a: 1 }, 42);
    expect(frontendLog).toHaveBeenLastCalledWith('info', '{"a":1} 42');

    const circular: Record<string, unknown> = {};
    circular.self = circular;
    expect(() => console.log(circular)).not.toThrow();
    // 序列化兜底为 String(value)，保证日志不丢
    const last = frontendLog.mock.calls.at(-1)![1] as string;
    expect(last).toContain('[object Object]');
  });

  it('重复安装只生效一次（多窗口入口都会调用）', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();
    installFrontendLogging();

    console.log('once');
    expect(frontendLog).toHaveBeenCalledTimes(1);
  });

  it('未捕获异常与未处理的 Promise 拒绝都上报为 error', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    listeners.get('error')!({
      message: 'boom',
      filename: 'app.js',
      lineno: 10,
      colno: 3,
      error: new Error('boom'),
    });
    const errMsg = frontendLog.mock.calls.at(-1)![1] as string;
    expect(frontendLog.mock.calls.at(-1)![0]).toBe('error');
    expect(errMsg).toContain('boom');

    listeners.get('unhandledrejection')!({ reason: new Error('rejected!') });
    expect(frontendLog.mock.calls.at(-1)![0]).toBe('error');
    expect(frontendLog.mock.calls.at(-1)![1]).toContain('rejected!');

    // 非 Error 的 reason 也要能转成字符串
    listeners.get('unhandledrejection')!({ reason: 'plain-string' });
    expect(frontendLog.mock.calls.at(-1)![1]).toContain('plain-string');
  });

  it('没有 error 对象的 onerror 事件退化为 message @ file:line:col', async () => {
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    listeners.get('error')!({ message: 'plain', filename: 'x.js', lineno: 1, colno: 2 });
    expect(frontendLog.mock.calls.at(-1)![1]).toContain('plain @ x.js:1:2');
  });

  it('转发失败被吞掉，不影响前端主流程', async () => {
    frontendLog.mockRejectedValue(new Error('backend down'));
    const { installFrontendLogging } = await import('./frontendLog');
    installFrontendLogging();

    expect(() => console.log('still works')).not.toThrow();
    // 让 catch 回调执行，确认没有未处理的 rejection
    await Promise.resolve();
    await Promise.resolve();
    expect(printed).toContain('still works');
  });
});
