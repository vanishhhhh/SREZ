import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, SessionClient } from './client';

const profile = {
  id: 1,
  username: 'owner',
  display_name: 'Owner',
  avatar_url: '',
  email: 'owner@example.com',
  email_verified_at: null,
  email_verification_required: false,
};
const json = (data: unknown, status = 200) =>
  new Response(JSON.stringify(data), { status, headers: { 'Content-Type': 'application/json' } });
afterEach(() => vi.unstubAllGlobals());

describe('memory-only browser session', () => {
  it('restores with HttpOnly cookie and CSRF, sharing one bootstrap', async () => {
    const calls: { url: string; init?: RequestInit }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        calls.push({ url, init });
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf-from-cookie' });
        if (url.endsWith('/auth/refresh'))
          return json({ access_token: 'memory-access', csrf_token: 'csrf-from-cookie' });
        return json(profile);
      }),
    );
    vi.stubGlobal(
      'localStorage',
      new Proxy(
        {},
        {
          get() {
            throw new Error('Web Storage is forbidden');
          },
        },
      ),
    );
    vi.stubGlobal(
      'sessionStorage',
      new Proxy(
        {},
        {
          get() {
            throw new Error('Web Storage is forbidden');
          },
        },
      ),
    );
    const client = new SessionClient();
    await Promise.all([client.bootstrap(), client.bootstrap()]);
    expect(client.snapshot().status).toBe('authenticated');
    expect(client.snapshot().user).toEqual(profile);
    expect(calls.filter((c) => c.url.endsWith('/auth/refresh'))).toHaveLength(1);
    expect(calls.every((c) => c.init?.credentials === 'include')).toBe(true);
    const refresh = calls.find((c) => c.url.endsWith('/auth/refresh'))!;
    expect(new Headers(refresh.init?.headers).get('X-CSRF-Token')).toBe('csrf-from-cookie');
    expect(refresh.init?.body).toBe('{}');
    const getProfile = calls.find((c) => c.url.endsWith('/profile'))!;
    expect(new Headers(getProfile.init?.headers).get('Authorization')).toBe('Bearer memory-access');
    expect(JSON.stringify(client.snapshot())).not.toContain('memory-access');
  });
  it('treats an absent refresh cookie as a normal anonymous visit', async () => {
    const fetcher = vi.fn(async () => json({ error: 'invalid credentials' }, 401));
    vi.stubGlobal('fetch', fetcher);
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot()).toEqual({
      status: 'anonymous',
      user: null,
      error: null,
      errorStatus: null,
    });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });
  it('uses one refresh for concurrent expired requests and retries with the new token', async () => {
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) return json({ access_token: 'old', csrf_token: 'csrf' });
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) {
          refreshes++;
          await new Promise((resolve) => setTimeout(resolve, 10));
          return json({ access_token: 'new', csrf_token: 'csrf' });
        }
        return new Headers(init?.headers).get('Authorization') === 'Bearer new'
          ? json({ ok: true })
          : json({ error: 'invalid access token' }, 401);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'a password');
    const results = await Promise.all(
      [1, 2, 3].map(() => client.request('/private', { auth: true })),
    );
    expect(refreshes).toBe(1);
    expect(results).toEqual([{ ok: true }, { ok: true }, { ok: true }]);
  });
  it('does not retry a forbidden write or hide the permission error', async () => {
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login'))
          return json({ access_token: 'access', csrf_token: 'csrf' });
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/refresh')) refreshes++;
        return json({ error: 'operation forbidden' }, 403);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'a password');
    await expect(
      client.request('/brands/1', { method: 'PATCH', auth: true, body: { name: 'Changed' } }),
    ).rejects.toMatchObject({ status: 403 });
    expect(refreshes).toBe(0);
    expect(client.snapshot().status).toBe('authenticated');
  });
  it('logs out with current-cookie CSRF and clears the in-memory identity', async () => {
    let logoutHeaders = new Headers();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login'))
          return json({ access_token: 'access', csrf_token: 'csrf' });
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'current-csrf' });
        logoutHeaders = new Headers(init?.headers);
        return new Response(null, { status: 204 });
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'a password');
    await client.logout();
    expect(logoutHeaders.get('X-CSRF-Token')).toBe('current-csrf');
    expect(logoutHeaders.has('Authorization')).toBe(false);
    expect(client.snapshot().status).toBe('anonymous');
  });
  it('surfaces network failures instead of pretending logout succeeded', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login'))
          return json({ access_token: 'access', csrf_token: 'csrf' });
        if (url.endsWith('/profile')) return json(profile);
        throw new TypeError('offline');
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'a password');
    await expect(client.logout()).rejects.toBeInstanceOf(ApiError);
    expect(client.snapshot().status).toBe('authenticated');
  });
});
