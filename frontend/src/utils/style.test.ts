import { describe, it, expect } from 'vitest';
import { rootStyleToStyle } from './style';

describe('rootStyleToStyle', () => {
  it('joins css vars with "; " separator', () => {
    expect(rootStyleToStyle({ '--a': '1', '--b': '2' })).toBe('--a: 1; --b: 2');
  });

  it('returns empty string for empty map', () => {
    expect(rootStyleToStyle({})).toBe('');
  });

  it('handles single entry', () => {
    expect(rootStyleToStyle({ '--x': 'y' })).toBe('--x: y');
  });
});
