import { writable, get } from 'svelte/store';
import { onEvent } from '../runtime';
import { EventLocaleChanged, type LocaleChangedPayload } from '../utils/events';
import { locale, setLocale, resolveLang, type LangCode } from '../i18n';
import { GetConfig } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';

export const activeWindow = writable<string>('');

let unregister: Array<() => void> = [];

export async function initWindow(): Promise<void> {
  try {
    // 读取界面语言配置（Settings.Language 为界面语言：auto/zh-CN/en-US）
    const cfg = await GetConfig();
    if (cfg?.language) setLocale(resolveLang(cfg.language as LangCode));
  } catch {
    // ignore
  }

  unregister.forEach((u) => u());
  unregister = [];
  unregister.push(
    onEvent(EventLocaleChanged, (payload: LocaleChangedPayload) => {
      // Language 为界面语言（auto/zh-CN/en-US），直接用。
      if (payload?.language) setLocale(payload.language as LangCode);
    }),
  );
}

export function currentLang(): LangCode {
  return get(locale);
}
