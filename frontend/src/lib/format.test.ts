import { describe, expect, it } from 'vitest';
import { httpsURL, parsePrice, parseStock, passwordValid, validReturnPath } from './format';

describe('money and stock transport', () => {
  it('converts rubles exactly, including decimal commas', () => {
    expect(parsePrice('2500')).toBe(250000);
    expect(parsePrice('0,01')).toBe(1);
    expect(parsePrice('10.1')).toBe(1010);
    expect(parsePrice('0')).toBe(0);
  });
  it.each(['-1', '1.001', '1e3', '', '9007199254740992'])(
    'rejects invalid or imprecise price %s',
    (input) => {
      expect(() => parsePrice(input)).toThrow();
    },
  );
  it('accepts zero and arbitrary integral stock, rejects unsafe numbers', () => {
    expect(parseStock('0')).toBe(0);
    expect(parseStock('123')).toBe(123);
    for (const value of ['-1', '1.5', '', '1e2', '9007199254740992'])
      expect(() => parseStock(value)).toThrow();
  });
});
describe('safe user inputs', () => {
  it('allows HTTPS URLs without credentials', () => {
    expect(httpsURL('https://example.com/a.jpg')).toBe(true);
    for (const url of [
      'http://example.com/a.jpg',
      'javascript:alert(1)',
      'https://user:pass@example.com/a',
      'https://example.com/a b',
    ])
      expect(httpsURL(url)).toBe(false);
    expect(httpsURL('', true)).toBe(true);
  });
  it('checks Unicode character count and UTF-8 password length without trimming', () => {
    expect(passwordValid('a'.repeat(7))).toBe(false);
    expect(passwordValid('я'.repeat(8))).toBe(true);
    expect(passwordValid('🙂'.repeat(7))).toBe(false);
    expect(passwordValid('🙂'.repeat(257))).toBe(false);
  });
  it('never accepts cross-origin or auth-loop return paths', () => {
    expect(validReturnPath('/studio/brands/1?tab=settings')).toBe('/studio/brands/1?tab=settings');
    for (const path of [
      'https://evil.test',
      '//evil.test',
      '/\\evil.test',
      '/login',
      '/register?from=x',
      '/unavailable',
      '/\n/evil.test',
      null,
    ])
      expect(validReturnPath(path)).toBe('/studio');
  });
});
