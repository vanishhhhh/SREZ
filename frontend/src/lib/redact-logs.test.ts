import { expect, it } from 'vitest';
import { redactWishlistLinks } from '../../scripts/redact-logs';
it('redacts verification fragments, query strings and formatted POST bodies', () => {
  const raw = 's'.repeat(43);
  for (const text of [
    '/verify-email#token=' + raw,
    '/verify-email?token=' + raw,
    JSON.stringify({ token: raw }),
  ])
    expect(redactWishlistLinks(text)).not.toContain(raw);
});
it('redacts API and browser capability paths without altering useful diagnostics', () => {
  const token = 'a'.repeat(43);
  for (const path of ['/api/wishlists/shared/', '/wishlists/shared/', '/w/']) {
    const safe = redactWishlistLinks('proxy error: ' + path + token + '?check=1');
    expect(safe).not.toContain(token);
    expect(safe).toContain('proxy error: ' + path + '[redacted]?check=1');
  }
  expect(redactWishlistLinks('/api/products/12 ECONNREFUSED')).toBe(
    '/api/products/12 ECONNREFUSED',
  );
});
