import { System } from '@wailsio/runtime';

// Wails v3 多窗口场景下，部分窗口（如截图翻译浮窗 screenshot）的 window._wails
// 未被注入，导致 System.IsMac() 误返回 false（实测 screenshot 窗口 _wails 为空）。
// 因此优先使用 System.IsMac() 等原生 API，失败时回退到 navigator.userAgent
// （在所有窗口均可靠，已验证 screenshot 窗口 UA 为 Macintosh）。
export function isMac(): boolean {
  try {
    return System.IsMac();
  } catch {
    return /Mac|iPhone|iPad|iPod/i.test(navigator.userAgent);
  }
}

export function isWindows(): boolean {
  try {
    return System.IsWindows();
  } catch {
    return /Win/i.test(navigator.userAgent);
  }
}

export function isLinux(): boolean {
  try {
    return System.IsLinux();
  } catch {
    return /Linux|X11/i.test(navigator.userAgent);
  }
}

// 保留 System 引用以防 tree-shaking 移除（同时便于需要 Environment() 异步信息时使用）。
export { System };
