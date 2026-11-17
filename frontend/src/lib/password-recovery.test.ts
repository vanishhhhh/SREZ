import { afterEach, expect, it, vi } from 'vitest';
import {
  captureVerificationLink,
  resetLinkToken,
  forgetResetLink,
  verificationLinkToken,
  forgetVerificationLink,
} from './verification-link';
import { SessionClient } from './client';
afterEach(() => {
  forgetResetLink();
  forgetVerificationLink();
  vi.unstubAllGlobals();
});
it('reset fragment is memory-only, removed before routing and separated from verification', () => {
  const replace = vi.fn(),
    raw = 'r'.repeat(43);
  vi.stubGlobal('window', {
    location: {
      pathname: '/reset-password',
      href: 'http://localhost:5174/reset-password#token=' + raw,
    },
    history: { state: null, replaceState: replace },
  });
  captureVerificationLink();
  expect(resetLinkToken()).toBe(raw);
  expect(verificationLinkToken()).toBe('');
  expect(replace).toHaveBeenCalledWith(null, '', '/reset-password');
  forgetResetLink();
  expect(resetLinkToken()).toBe('');
});
it('reset submits strict token/password body without authorization or persistence', async () => {
  const fetcher = vi
    .fn()
    .mockResolvedValue(
      new Response(JSON.stringify({ message: 'password reset; sign in again' }), { status: 200 }),
    );
  vi.stubGlobal('fetch', fetcher);
  const session = new SessionClient();
  await session.resetPassword('r'.repeat(43), 'a valid new password 123');
  expect(fetcher.mock.calls[0][0]).toBe('/api/auth/reset-password');
  expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({
    token: 'r'.repeat(43),
    password: 'a valid new password 123',
  });
  expect(fetcher.mock.calls[0][1].headers.Authorization).toBeUndefined();
  expect(session.snapshot().status).toBe('anonymous');
});
it('failed reset keeps the current auth state and propagates the safe error', async () => {
  vi.stubGlobal(
    'fetch',
    vi
      .fn()
      .mockResolvedValue(
        new Response(JSON.stringify({ error: 'invalid or expired reset link' }), { status: 400 }),
      ),
  );
  const session = new SessionClient();
  await expect(
    session.resetPassword('r'.repeat(43), 'a valid new password 123'),
  ).rejects.toMatchObject({ status: 400, code: 'invalid or expired reset link' });
  expect(session.snapshot().status).toBe('checking');
});
