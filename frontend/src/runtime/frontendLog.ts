import { FrontendLog } from '@bindings/cnb.cool/dtapp/kai/internal/logutil/frontendlogservice.ts';

// 把前端的 console 日志与 JS 错误转发到 Go，统一写入 logs/frontend.log。
// 避免在 dev 环境之外丢失前端报错线索（正式包无 devtools）。

let installed = false;

type Level = 'debug' | 'info' | 'warn' | 'error';

// 发送单条日志到 Go。异步、吞掉异常，避免前端日志回环影响主流程。
function send(level: Level, msg: string) {
  FrontendLog(level, msg).catch(() => {
    /* 忽略转发失败，不阻塞前端 */
  });
}

function serialize(args: unknown[]): string {
  return args
    .map((a) => {
      if (typeof a === 'string') return a;
      try {
        return JSON.stringify(a);
      } catch {
        return String(a);
      }
    })
    .join(' ');
}

// 安装全局错误捕获与 console 转发。每个窗口入口调用一次即可（幂等）。
export function installFrontendLogging() {
  if (installed) return;
  installed = true;

  const orig = {
    log: console.log,
    info: console.info,
    warn: console.warn,
    error: console.error,
    debug: console.debug,
  };

  console.log = (...args: unknown[]) => {
    orig.log(...args);
    send('info', serialize(args));
  };
  console.info = (...args: unknown[]) => {
    orig.info(...args);
    send('info', serialize(args));
  };
  console.debug = (...args: unknown[]) => {
    orig.debug(...args);
    send('debug', serialize(args));
  };
  console.warn = (...args: unknown[]) => {
    orig.warn(...args);
    send('warn', serialize(args));
  };
  console.error = (...args: unknown[]) => {
    orig.error(...args);
    send('error', serialize(args));
  };

  // 未捕获的同步/异步错误
  window.addEventListener('error', (e: ErrorEvent) => {
    const detail = e.error?.stack || `${e.message} @ ${e.filename}:${e.lineno}:${e.colno}`;
    send('error', `Uncaught Error: ${detail}`);
  });

  // 未处理的 Promise 拒绝
  window.addEventListener('unhandledrejection', (e: PromiseRejectionEvent) => {
    const reason = e.reason?.stack || e.reason?.message || String(e.reason);
    send('error', `Unhandled Rejection: ${reason}`);
  });
}
