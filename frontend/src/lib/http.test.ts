import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, jsonRequest } from './http';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
describe('JSON transport', () => {
  it('keeps cookies and JSON headers on the same-origin API prefix', async () => {
    const fetcher = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    vi.stubGlobal('fetch', fetcher);
    await jsonRequest('/profile', { method: 'PATCH', body: { display_name: 'Имя' } });
    const [url, init] = fetcher.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/profile');
    expect(init.credentials).toBe('include');
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json');
    expect(init.body).toBe(JSON.stringify({ display_name: 'Имя' }));
  });
  it('supports empty 204 without parsing JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(null, { status: 204 })),
    );
    expect((await jsonRequest('/auth/logout')).data).toBe(null);
  });
  it('retains HTTP status for non-JSON proxy/server errors without displaying raw bodies', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('<html>internal detail</html>', { status: 503 })),
    );
    await expect(jsonRequest('/profile')).rejects.toMatchObject({ status: 503 });
    await expect(jsonRequest('/profile')).rejects.not.toHaveProperty(
      'message',
      expect.stringContaining('internal detail'),
    );
  });
  it('rejects malformed successful JSON as a recoverable server error', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response('not JSON', { status: 200 })),
    );
    await expect(jsonRequest('/profile')).rejects.toMatchObject({
      status: 502,
      code: 'invalid response',
    });
  });
  it('distinguishes a timeout from caller cancellation', async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal!.addEventListener('abort', () =>
              reject(new DOMException('Aborted', 'AbortError')),
            );
          }),
      ),
    );
    const timed = jsonRequest('/profile');
    const assertion = expect(timed).rejects.toMatchObject({ status: 0, code: 'request timeout' });
    await vi.advanceTimersByTimeAsync(20000);
    await assertion;
    const controller = new AbortController();
    const cancelled = jsonRequest('/profile', { signal: controller.signal });
    const cancelledAssertion = expect(cancelled).rejects.toMatchObject({ name: 'AbortError' });
    controller.abort();
    await cancelledAssertion;
  });
  it('refuses cross-origin/ambiguous paths before sending credentials', async () => {
    const fetcher = vi.fn();
    vi.stubGlobal('fetch', fetcher);
    for (const path of ['https://example.com', '//example.com', '/\\example.com'])
      await expect(jsonRequest(path)).rejects.toThrow();
    expect(fetcher).not.toHaveBeenCalled();
    expect(new ApiError(403, 'unknown secret body').message).not.toContain('unknown secret body');
  });
});
