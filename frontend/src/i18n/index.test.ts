import { describe, expect, it, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { t, locale, setLocale, resolveLang, getSystemLocale, langName, engineName } from './index';
import { zh } from './zh-CN';
import { en } from './en-US';
import { Lang } from '../constants/lang';

// 把嵌套字典拍平成 a.b.c -> 字符串，便于逐键校验。
function flatten(obj: unknown, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(obj as Record<string, unknown>)) {
    const key = prefix ? `${prefix}.${k}` : k;
    if (v && typeof v === 'object') Object.assign(out, flatten(v, key));
    else out[key] = String(v);
  }
  return out;
}

const zhFlat = flatten(zh);
const enFlat = flatten(en);

afterEach(() => {
  // 用例外切换过语言，统一复位，避免污染后续用例。
  setLocale(Lang.ZHCN);
});

describe('t() 字典查询', () => {
  it('中文模式下每个 key 都能原样取出（字典接线未断）', () => {
    expect(Object.keys(zhFlat).length).toBeGreaterThan(100);
    for (const [key, val] of Object.entries(zhFlat)) {
      expect(t(key), `key=${key}`).toBe(val);
    }
  });

  it('英文模式下每个 key 都能原样取出', () => {
    setLocale(Lang.ENUS);
    expect(Object.keys(enFlat).length).toBeGreaterThan(100);
    for (const [key, val] of Object.entries(enFlat)) {
      expect(t(key), `key=${key}`).toBe(val);
    }
  });

  it('中英文字典的 key 集合完全一致（防止漏译）', () => {
    const zhKeys = Object.keys(zhFlat).sort();
    const enKeys = Object.keys(enFlat).sort();
    expect(enKeys).toEqual(zhKeys);
  });

  it('缺失的 key 回退为路径本身，便于在界面上直接发现', () => {
    expect(t('nope.missing.path')).toBe('nope.missing.path');
    // 指向中间节点（对象而非字符串）同样回退
    expect(t('settings')).toBe('settings');
  });

  it('{param} 占位符按传入参数替换，且可替换多个', () => {
    expect(t('settings.historyPageInfo', { cur: 1, total: 3, count: 27 })).toBe(
      '第 1 / 3 页 · 共 27 条',
    );
    // 重复出现同一占位符时全部替换
    setLocale(Lang.ENUS);
    const s = t('settings.historyPageInfo', { cur: 2, total: 5, count: 40 });
    expect(s).not.toContain('{');
    expect(s).toContain('2');
    expect(s).toContain('5');
    expect(s).toContain('40');
  });

  it('无参数的文案不受参数替换影响', () => {
    expect(t('common.cancel', { unused: 'x' })).toBe(zhFlat['common.cancel']);
  });
});

describe('语言解析', () => {
  it('setLocale 同步更新 locale store 与 t 的生效语言', () => {
    setLocale(Lang.ENUS);
    expect(get(locale)).toBe(Lang.ENUS);
    expect(t('common.cancel')).toBe(enFlat['common.cancel']);

    setLocale(Lang.ZHCN);
    expect(get(locale)).toBe(Lang.ZHCN);
    expect(t('common.cancel')).toBe(zhFlat['common.cancel']);
  });

  it('resolveLang 对具体语言原样返回', () => {
    expect(resolveLang(Lang.ZHCN)).toBe(Lang.ZHCN);
    expect(resolveLang(Lang.ENUS)).toBe(Lang.ENUS);
  });

  it('resolveLang 对 auto 解析为系统语言', () => {
    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'zh-Hans-CN',
      configurable: true,
    });
    expect(resolveLang(Lang.Auto)).toBe(Lang.ZHCN);

    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'en-GB',
      configurable: true,
    });
    expect(resolveLang(Lang.Auto)).toBe(Lang.ENUS);

    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'zh-CN',
      configurable: true,
    });
  });

  it('getSystemLocale 只区分中文/其它', () => {
    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'zh-TW',
      configurable: true,
    });
    expect(getSystemLocale()).toBe(Lang.ZHCN);

    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'ja-JP',
      configurable: true,
    });
    expect(getSystemLocale()).toBe(Lang.ENUS);

    Object.defineProperty(globalThis.navigator, 'language', {
      value: 'zh-CN',
      configurable: true,
    });
  });

  it('setLocale(auto) 也会解析成具体语言而非停留在 auto', () => {
    setLocale(Lang.Auto);
    expect([Lang.ZHCN, Lang.ENUS]).toContain(get(locale) as string);
  });
});

describe('langName / engineName', () => {
  it('已知语言码返回本地化名称', () => {
    expect(langName('zh')).not.toBe('zh');
    expect(langName('zh')).toBe(zhFlat['lang.zh']);
    expect(langName('en')).toBe(zhFlat['lang.en']);
  });

  it('去掉首尾空白后再查表', () => {
    expect(langName('  zh  ')).toBe(zhFlat['lang.zh']);
  });

  it('未知语言码原样返回，空值返回空串', () => {
    expect(langName('klingon')).toBe('klingon');
    expect(langName('')).toBe('');
  });

  it('引擎名大小写不敏感', () => {
    setLocale(Lang.ENUS);
    const lower = engineName('google');
    const upper = engineName('GOOGLE');
    expect(lower).not.toBe('GOOGLE');
    expect(upper).toBe(lower);
  });

  it('未知引擎原样返回，空值返回空串', () => {
    expect(engineName('unknown-engine')).toBe('unknown-engine');
    expect(engineName('')).toBe('');
  });
});
