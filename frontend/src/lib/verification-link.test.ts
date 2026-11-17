import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  captureVerificationLink,
  forgetVerificationLink,
  verificationLinkToken,
} from './verification-link';
afterEach(() => {
  forgetVerificationLink();
  vi.unstubAllGlobals();
});
describe('verification link secrets', () => {
  it('captures the fragment in memory and strips it before router initialization', () => {
    const raw = 'a'.repeat(43),
      replace = vi.fn();
    vi.stubGlobal('window', {
      location: {
        pathname: '/verify-email',
        href: 'http://localhost:5174/verify-email#token=' + raw,
      },
      history: { state: null, replaceState: replace },
    });
    const forbidden = new Proxy(
      {},
      {
        get() {
          throw Error('Persistent storage forbidden');
        },
      },
    );
    vi.stubGlobal('localStorage', forbidden);
    vi.stubGlobal('sessionStorage', forbidden);
    captureVerificationLink();
    expect(verificationLinkToken()).toBe(raw);
    expect(replace).toHaveBeenCalledWith(null, '', '/verify-email');
    forgetVerificationLink();
    expect(verificationLinkToken()).toBe('');
  });
  it('removes query tokens as well, while rejecting malformed encodings', () => {
    const replace = vi.fn();
    vi.stubGlobal('window', {
      location: {
        pathname: '/verify-email',
        href: 'http://localhost:5174/verify-email?token=bad%3Cscript%3E&from=srez#debug',
      },
      history: { state: {}, replaceState: replace },
    });
    captureVerificationLink();
    expect(verificationLinkToken()).toBe('');
    expect(replace).toHaveBeenCalledWith({}, '', '/verify-email?from=srez');
  });
});
