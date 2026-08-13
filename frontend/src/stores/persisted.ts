import { writable, type Writable } from 'svelte/store';

// localStorage-backed writable store：状态变更自动持久化，初始化时从 localStorage 读取。
export function persisted<T>(key: string, initial: T): Writable<T> {
  let start = initial;
  try {
    const raw = localStorage.getItem(key);
    if (raw !== null) start = JSON.parse(raw) as T;
  } catch {
    // ignore 损坏值，回退 initial
  }
  const store = writable<T>(start);
  store.subscribe((v) => {
    try {
      localStorage.setItem(key, JSON.stringify(v));
    } catch {
      // ignore 写入失败（如隐私模式）
    }
  });
  return store;
}

// 按窗口名生成置顶状态的持久化 key，确保各窗口（translate/settings/selection）的置顶状态相互独立。
export function pinKey(windowName: string): string {
  return `kai:${windowName}:pinned`;
}
