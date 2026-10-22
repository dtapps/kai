import { describe, it, expect } from 'vitest';
import { THEME } from './theme';
import { WindowTranslate, WindowSettings, WindowScreenshot } from './window';

describe('THEME', () => {
  it('exposes auto/light/dark modes', () => {
    expect(THEME.Auto).toBe('auto');
    expect(THEME.Light).toBe('light');
    expect(THEME.Dark).toBe('dark');
  });
});

describe('window name constants', () => {
  it('are stable window identifiers', () => {
    expect(WindowTranslate).toBe('translate');
    expect(WindowSettings).toBe('settings');
    expect(WindowScreenshot).toBe('screenshot');
  });
});
