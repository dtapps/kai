import { writable, get } from 'svelte/store';
import { onEvent } from '../runtime';
import { EventLocaleChanged, type LocaleChangedPayload } from '../utils/events';
import { locale, setLocale, resolveLang, type LangCode } from '../i18n';
import { Lang } from '../constants/lang';
import { GetConfig } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';

export const activeWindow = writable<string>('');

// userLang 保存用户设置的「界面语言 mode」（auto/zh-CN/en-US），与 locale（解析后的实际生效语言）区分。
// 设置页下拉框应绑定 userLang，否则 auto 用户会被显示成解析后的具体语言。
export const userLang = writable<LangCode>(Lang.Auto);

let unregister: Array<() => void> = [];

export async function initWindow(): Promise<void> {
  try {
    // 读取界面语言配置（Settings.Language 为界面语言：auto/zh-CN/en-US）
    const cfg = await GetConfig();
    if (cfg?.language) {
      userLang.set(cfg.language as LangCode);
      setLocale(resolveLang(cfg.language as LangCode));
    }
  } catch {
    // ignore
  }

  unregister.forEach((u) => u());
  unregister = [];
  unregister.push(
    onEvent(EventLocaleChanged, (payload: LocaleChangedPayload) => {
      // Language 为界面语言 mode（auto/zh-CN/en-US）。
      if (payload?.language) {
        userLang.set(payload.language as LangCode);
        setLocale(resolveLang(payload.language as LangCode));
      }
    }),
  );
}

export function currentLang(): LangCode {
  return get(locale);
}
