import { writable } from 'svelte/store';
import { zh, type Dict } from './zh-CN';
import { en } from './en-US';
import { Lang, type LangCode } from '../constants/lang';

// 对外暴露 LangCode 类型（界面语言：auto/zh-CN/en-US），供 index.ts 及组件复用。
export type { LangCode };

// i18n 实际生效语言只能是 zh-CN/en-US（auto 由 resolveLang 解析），故 dicts 仅两键。
type ResolvedLang = typeof Lang.ZHCN | typeof Lang.ENUS;
const dicts: Record<ResolvedLang, Dict> = { [Lang.ZHCN]: zh, [Lang.ENUS]: en };

// 用 runes 状态持有当前语言，使 t() 在任意组件内自动响应式刷新
let currentLang = $state<ResolvedLang>(Lang.ZHCN);

// 兼容旧的 writable 用法（ui.ts 等仍通过 setLocale/get 操作）
const localeStore = writable<ResolvedLang>(Lang.ZHCN);
localeStore.subscribe((l) => {
  if (l !== currentLang) currentLang = l;
});

export const locale = localeStore;

type Path = string;

function lookup(dict: unknown, path: string): string {
  const parts = path.split('.');
  let cur: any = dict;
  for (const p of parts) {
    if (cur == null) return path;
    cur = cur[p];
  }
  return typeof cur === 'string' ? cur : path;
}

export function t(path: Path, params?: Record<string, string | number>): string {
  let str = lookup(dicts[currentLang], path);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      str = str.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v));
    }
  }
  return str;
}

// 把界面语言（Lang：auto/zh-CN/en-US）解析为实际生效语言（zh-CN/en-US）。
// auto 时跟随系统语言，与界面语言体系一致，与翻译语言无关。
export function resolveLang(lang: LangCode): typeof Lang.ZHCN | typeof Lang.ENUS {
  if (lang === Lang.ZHCN || lang === Lang.ENUS) return lang;
  const nav = (typeof navigator !== 'undefined' && navigator.language) || '';
  return nav.toLowerCase().startsWith(Lang.ZHCN) ? Lang.ZHCN : Lang.ENUS;
}

export function setLocale(lang: LangCode): void {
  const resolved = resolveLang(lang);
  currentLang = resolved;
  localeStore.set(resolved);
}

export function langName(code: string): string {
  if (!code) return '';
  const key = `lang.${code.trim()}`;
  const val = lookup(dicts[currentLang], key);
  return val === key ? code : val;
}

export function engineName(code: string): string {
  if (!code) return '';
  const key = `engine.${code.trim().toLowerCase()}`;
  const val = lookup(dicts[currentLang], key);
  return val === key ? code : val;
}
