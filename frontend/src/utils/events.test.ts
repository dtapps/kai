import { describe, it, expect } from 'vitest';
import {
  EventWindowShow,
  EventWindowClosing,
  EventLocaleChanged,
  EventThemeChanged,
  EventInputFill,
} from './events';

describe('event name constants', () => {
  it('use kai: namespace to avoid collisions', () => {
    for (const name of [
      EventWindowShow,
      EventWindowClosing,
      EventLocaleChanged,
      EventThemeChanged,
      EventInputFill,
    ]) {
      expect(name.startsWith('kai:')).toBe(true);
    }
  });

  it('locale and theme changed events are distinct', () => {
    expect(EventLocaleChanged).not.toBe(EventThemeChanged);
    expect(EventLocaleChanged).toBe('kai:locale:changed');
    expect(EventThemeChanged).toBe('kai:theme:changed');
  });
});
