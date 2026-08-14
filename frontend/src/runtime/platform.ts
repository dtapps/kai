import { System } from '@wailsio/runtime';

// Wails v3 多窗口场景下，部分窗口（如截图翻译浮窗 screenshot）的 window._wails
// 未被注入，导致 System.IsMac() 误返回 false（实测 screenshot 窗口 _wails 为空）。
// 因此优先读取 window._wails.environment.OS，缺失时回退到 navigator.userAgent
// （在所有窗口均可靠，已验证 screenshot 窗口 UA 为 Macintosh）。
export function isMac(): boolean {
  const wailsEnv = (window as any)._wails?.environment;
  if (wailsEnv && typeof wailsEnv.OS === 'string') {
    return wailsEnv.OS === 'darwin';
  }
  return /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent);
}

export function isWindows(): boolean {
  const wailsEnv = (window as any)._wails?.environment;
  if (wailsEnv && typeof wailsEnv.OS === 'string') {
    return wailsEnv.OS === 'windows';
  }
  return /Win/i.test(navigator.userAgent);
}

export function isLinux(): boolean {
  const wailsEnv = (window as any)._wails?.environment;
  if (wailsEnv && typeof wailsEnv.OS === 'string') {
    return wailsEnv.OS === 'linux';
  }
  return /Linux|X11/i.test(navigator.userAgent);
}

// 保留 System 引用以防 tree-shaking 移除（同时便于需要 Environment() 异步信息时使用）。
export { System };
