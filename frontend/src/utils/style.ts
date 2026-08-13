export type CSSVars = Record<string, string>;

export function rootStyleToStyle(vars: CSSVars): string {
  return Object.entries(vars)
    .map(([k, v]) => `${k}: ${v}`)
    .join('; ');
}
