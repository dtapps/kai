// 测试环境桩：node 环境没有 localStorage / navigator，而 store 常在导入期读取它们，不补桩会加载即崩。

const memory = new Map<string, string>();

const localStorageStub: Storage = {
  get length() {
    return memory.size;
  },
  clear() {
    memory.clear();
  },
  getItem(key: string) {
    return memory.get(key) ?? null;
  },
  key(index: number) {
    return Array.from(memory.keys())[index] ?? null;
  },
  removeItem(key: string) {
    memory.delete(key);
  },
  setItem(key: string, value: string) {
    memory.set(key, value);
  },
};

Object.defineProperty(globalThis, 'localStorage', {
  value: localStorageStub,
  configurable: true,
  writable: true,
});

// navigator.language：node 无此属性；i18n 'auto' 模式解析系统语言时需要
Object.defineProperty(globalThis, 'navigator', {
  value: { language: 'zh-CN' },
  configurable: true,
  writable: true,
});

// 屏蔽 debug 噪音；console.error 保留可见，避免掩盖真实问题
console.debug = () => {};
