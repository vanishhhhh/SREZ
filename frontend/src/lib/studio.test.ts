import { expect, it } from 'vitest';
import { brandSlugPattern, slugFromName } from './studio';
it('brand slug HTML pattern compiles in modern Unicode Sets mode', () => {
  const rule = new RegExp('^(?:' + brandSlugPattern + ')$', 'v');
  for (const value of ['srez', 'my-brand', 'brand-2026']) expect(rule.test(value)).toBe(true);
  for (const value of ['two words', 'Бренд', 'bad_slug', 'UPPER', 'a'.repeat(61), ''])
    expect(rule.test(value)).toBe(false);
  expect(rule.test(slugFromName('Свой Бренд'))).toBe(true);
});
