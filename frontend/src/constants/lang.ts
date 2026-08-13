// 语言码统一常量：前端复用后端 model 的 enum 定义（wails 自动生成），
// 避免在各组件散落 'auto' / 'zh-CN' / 'en-US' 等裸字符串，便于前后端同步修改。
//
// 两套语义（互不混用，这是翻译软件的本质约束）：
//  - 翻译语言（TranslateLang / TRANSLATE_LANG）：翻译引擎的目标/源语言，9 种含 auto（自动检测），
//    对齐后端 model.Language（auto/zh/en/ja/...），与界面语言完全独立。
//  - 界面语言（Lang）：应用界面显示语言（auto / zh-CN / en-US，auto 由系统语言解析），
//    对齐后端 settings.Language，与主题（theme）平级，就叫"语言"不额外加前缀。

import { Language } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';

// 翻译语言别名，组件里用 TRANSLATE_LANG.Auto / TRANSLATE_LANG.ZH / TRANSLATE_LANG.EN 等。
export const TRANSLATE_LANG = Language;

// 翻译语言码（含 auto）。与后端 model.AllLanguages() 顺序一致。
export type TranslateLang = Language;

// 界面语言码（auto / zh-CN / en-US），对齐后端 settings.Language。就叫"语言"，不加 UI 前缀。
export type LangCode = 'auto' | 'zh-CN' | 'en-US';
// 界面语言码常量。组件里用 Lang.Auto / Lang.ZHCN / Lang.ENUS。
export const Lang = {
  Auto: 'auto',
  ZHCN: 'zh-CN',
  ENUS: 'en-US',
} as const;

// 全部翻译语言（含 auto），供下拉等场景直接遍历。
export const ALL_TRANSLATE_LANGS: TranslateLang[] = [
  Language.Auto,
  Language.ZH,
  Language.EN,
  Language.JA,
  Language.KO,
  Language.FR,
  Language.DE,
  Language.ES,
  Language.RU,
];

// 目标翻译语言需排除 auto（引擎不支持自动检测目标语言）。
export const TARGET_TRANSLATE_LANGS: TranslateLang[] = ALL_TRANSLATE_LANGS.filter(
  (c) => c !== Language.Auto,
);
