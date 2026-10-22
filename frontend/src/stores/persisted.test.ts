import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { persisted, pinKey } from './persisted';

describe('pinKey', () => {
  it('builds per-window pin key', () => {
    expect(pinKey('translate')).toBe('kai:translate:pinned');
    expect(pinKey('settings')).toBe('kai:settings:pinned');
  });
});

describe('persisted', () => {
  beforeEach(() => localStorage.clear());

  it('returns initial value when nothing stored', () => {
    const s = persisted('kai:test:x', 42);
    expect(get(s)).toBe(42);
  });

  it('reads previously stored value on init', () => {
    localStorage.setItem('kai:test:y', JSON.stringify({ a: 1 }));
    const s = persisted<{ a: number }>('kai:test:y', { a: 0 });
    expect(get(s)).toEqual({ a: 1 });
  });

  it('persists on update', () => {
    const s = persisted('kai:test:z', 'init');
    s.set('updated');
    expect(localStorage.getItem('kai:test:z')).toBe(JSON.stringify('updated'));
  });

  it('falls back to initial on corrupted stored value', () => {
    localStorage.setItem('kai:test:b', 'not-json');
    const s = persisted('kai:test:b', 'fallback');
    expect(get(s)).toBe('fallback');
  });
});
