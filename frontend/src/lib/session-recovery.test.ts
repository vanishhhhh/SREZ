import { afterEach, describe, expect, it, vi } from 'vitest';
import { SessionClient } from './client';

const profile = {
  id: 1,
  username: 'owner',
  display_name: 'Owner',
  avatar_url: '',
  email: 'owner@example.com',
  email_verified_at: null,
  email_verification_required: false,
};
const json = (data: unknown, status = 200) => new Response(JSON.stringify(data), { status });
const tokens = (access = 'access') => json({ access_token: access, csrf_token: 'csrf' });
const deferred = <T>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
};
afterEach(() => vi.unstubAllGlobals());

describe('session recovery and identity boundaries', () => {
  it('aborts an old-user write if refresh reveals another cookie identity without a broadcast', async () => {
    let changed = false;
    let writesWithNewToken = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) return tokens('first');
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) {
          changed = true;
          return tokens('second');
        }
        if (init?.method === 'PATCH') {
          if (new Headers(init.headers).get('Authorization') === 'Bearer second')
            writesWithNewToken++;
          return json({}, 401);
        }
        return json({ ...profile, id: changed ? 2 : 1, username: changed ? 'second' : 'first' });
      }),
    );
    const client = new SessionClient();
    await client.login('first', 'password');
    await expect(client.updateProfile({ display_name: 'Old intent' })).rejects.toMatchObject({
      code: 'session changed',
    });
    expect(writesWithNewToken).toBe(0);
    expect(client.snapshot()).toMatchObject({
      status: 'authenticated',
      user: { id: 2, username: 'second' },
    });
  });
  it('revalidates identity after another tab logs in, broadcasting no tokens', async () => {
    const channels: TestChannel[] = [];
    const messages: unknown[] = [];
    let cookieUser = '';
    class TestChannel {
      onmessage: ((event: { data: unknown }) => void) | null = null;
      constructor() {
        channels.push(this);
      }
      postMessage(data: unknown) {
        messages.push(data);
        for (const channel of channels) if (channel !== this) channel.onmessage?.({ data });
      }
    }
    vi.stubGlobal('window', { BroadcastChannel: TestChannel });
    vi.stubGlobal('BroadcastChannel', TestChannel);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) {
          cookieUser = JSON.parse(String(init?.body)).identifier;
          return tokens(cookieUser);
        }
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) return tokens(cookieUser);
        const username = new Headers(init?.headers).get('Authorization')?.slice(7);
        return json({ ...profile, username });
      }),
    );
    const first = new SessionClient();
    const second = new SessionClient();
    await first.login('first', 'password');
    await vi.waitFor(() => expect(second.snapshot().user?.username).toBe('first'));
    await second.login('second', 'password');
    await vi.waitFor(() => expect(first.snapshot().user?.username).toBe('second'));
    expect(messages).toEqual(['session-changed', 'session-changed']);
    expect(JSON.stringify(first.snapshot())).not.toContain('access_token');
  });
  it('queues logout behind an ongoing refresh even without Web Locks', async () => {
    vi.stubGlobal('navigator', {});
    const started = deferred<void>();
    const finish = deferred<void>();
    let rotating = false;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login')) return tokens('old');
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) {
          if (rotating) throw new Error('cookie mutation overlapped');
          return json({ csrf_token: 'csrf' });
        }
        if (url.endsWith('/auth/refresh')) {
          rotating = true;
          started.resolve();
          await finish.promise;
          rotating = false;
          return tokens('new');
        }
        if (url.endsWith('/auth/logout')) return new Response(null, { status: 204 });
        return json({}, 401);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    const pending = client.request('/private', { auth: true });
    const rejected = expect(pending).rejects.toMatchObject({ code: 'session changed' });
    await started.promise;
    const logout = client.logout();
    finish.resolve();
    await logout;
    await rejected;
    expect(client.snapshot().status).toBe('anonymous');
  });
  it('a response from an older identity cannot refresh or clear a newer login', async () => {
    const pending = deferred<Response>();
    let identifier = 'first';
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) {
          identifier = JSON.parse(String(init?.body)).identifier;
          return tokens(identifier);
        }
        if (url.endsWith('/profile')) return json({ ...profile, username: identifier });
        if (url.endsWith('/auth/refresh')) refreshes++;
        return pending.promise;
      }),
    );
    const client = new SessionClient();
    await client.login('first', 'password');
    const request = client.request('/private', { auth: true });
    const rejected = expect(request).rejects.toMatchObject({ code: 'session changed' });
    await client.login('second', 'password');
    pending.resolve(json({}, 401));
    await rejected;
    expect(client.snapshot().user?.username).toBe('second');
    expect(refreshes).toBe(0);
  });
  it('recovers bootstrap after an unavailable server without calling login', async () => {
    let offline = true;
    let logins = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (offline) throw new TypeError('offline');
        if (url.endsWith('/auth/login')) logins++;
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) return tokens();
        return json(profile);
      }),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot()).toMatchObject({ status: 'unavailable', user: null, errorStatus: 0 });
    offline = false;
    await client.restore();
    expect(client.snapshot()).toMatchObject({ status: 'authenticated', user: profile });
    expect(logins).toBe(0);
  });
  it('retries only profile when refresh succeeded but profile temporarily failed', async () => {
    let failed = true;
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) {
          refreshes++;
          return tokens();
        }
        return failed ? json({ error: 'unavailable' }, 503) : json(profile);
      }),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot().status).toBe('unavailable');
    failed = false;
    await client.restore();
    expect(client.snapshot().status).toBe('authenticated');
    expect(refreshes).toBe(1);
  });
  it('keeps identity recoverable for transient refresh errors while blocking private requests', async () => {
    let offline = true;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) return tokens('old');
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) {
          if (offline) throw new TypeError('offline');
          return json({ csrf_token: 'csrf' });
        }
        if (url.endsWith('/auth/refresh')) return tokens('new');
        return new Headers(init?.headers).get('Authorization') === 'Bearer new'
          ? json({ ok: true })
          : json({}, 401);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    await expect(client.request('/private', { auth: true })).rejects.toMatchObject({ status: 0 });
    expect(client.snapshot()).toMatchObject({ status: 'unavailable', user: profile });
    await expect(client.request('/private', { auth: true })).rejects.toMatchObject({ status: 0 });
    offline = false;
    await client.restore();
    expect(client.snapshot().status).toBe('authenticated');
    await expect(client.request('/private', { auth: true })).resolves.toEqual({ ok: true });
  });
  it('ends an expired session on refresh 401 without an infinite request loop', async () => {
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login')) return tokens();
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) refreshes++;
        return json({}, 401);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    await expect(client.request('/private', { auth: true })).rejects.toMatchObject({ status: 401 });
    expect(client.snapshot()).toMatchObject({ status: 'anonymous', user: null, errorStatus: 401 });
    expect(refreshes).toBe(1);
  });
  it('rereads CSRF for the current cookie on one refresh conflict', async () => {
    let csrfReads = 0;
    let refreshes = 0;
    const csrfHeaders: (string | null)[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf-' + ++csrfReads });
        if (url.endsWith('/auth/refresh')) {
          csrfHeaders.push(new Headers(init?.headers).get('X-CSRF-Token'));
          return ++refreshes === 1 ? json({ error: 'refresh already rotated' }, 409) : tokens();
        }
        return json(profile);
      }),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot().status).toBe('authenticated');
    expect(csrfHeaders).toEqual(['csrf-1', 'csrf-2']);
  });
  it('stops after two 409 responses and exposes recovery instead of pretending logout', async () => {
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) =>
        url.endsWith('/auth/csrf')
          ? json({ csrf_token: 'csrf' })
          : (refreshes++, json({ error: 'refresh already rotated' }, 409)),
      ),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(refreshes).toBe(2);
    expect(client.snapshot()).toMatchObject({ status: 'unavailable', errorStatus: 409 });
  });
  it('does not turn CSRF/Origin 403 into an expired session', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => json({ error: 'request origin denied' }, 403)),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot()).toMatchObject({ status: 'unavailable', errorStatus: 403 });
  });
  it('does not refresh from a late 401 after logout', async () => {
    const pending = deferred<Response>();
    let refreshes = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login')) return tokens();
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/logout')) return new Response(null, { status: 204 });
        if (url.endsWith('/auth/refresh')) refreshes++;
        return pending.promise;
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    const request = client.request('/private', { auth: true });
    const rejected = expect(request).rejects.toMatchObject({ code: 'session changed' });
    await client.logout();
    pending.resolve(json({}, 401));
    await rejected;
    expect(refreshes).toBe(0);
    expect(client.snapshot().status).toBe('anonymous');
  });
  it('discards a late successful private response after logout too', async () => {
    const pending = deferred<Response>();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/login')) return tokens();
        if (url.endsWith('/profile')) return json(profile);
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/logout')) return new Response(null, { status: 204 });
        return pending.promise;
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    const request = client.request('/private', { auth: true });
    const rejected = expect(request).rejects.toMatchObject({ code: 'session changed' });
    await client.logout();
    pending.resolve(json({ private: true }));
    await rejected;
    expect(client.snapshot().user).toBe(null);
  });
  it('rejects a malformed profile and recovers without storing raw auth response data', async () => {
    let malformed = true;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) return tokens('private-access');
        return json(
          malformed ? { ...profile, id: 'bad', access_token: 'should-not-leak' } : profile,
        );
      }),
    );
    const client = new SessionClient();
    await client.bootstrap();
    expect(client.snapshot()).toMatchObject({
      status: 'unavailable',
      user: null,
      errorStatus: 502,
    });
    malformed = false;
    await client.restore();
    expect(JSON.stringify(client.snapshot())).not.toContain('access');
  });
  it('sends only permitted fields in PATCH profile', async () => {
    let body = '';
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (url.endsWith('/auth/login')) return tokens();
        if (init?.method === 'PATCH') body = String(init.body);
        return json(profile);
      }),
    );
    const client = new SessionClient();
    await client.login('owner', 'password');
    await client.updateProfile({
      display_name: 'Owner',
      ...{
        avatar_url: 'https://example.com/legacy.jpg',
        role: 'admin',
        email: 'other@example.com',
        email_verified_at: null,
        email_verification_required: false,
      },
    });
    expect(JSON.parse(body)).toEqual({ display_name: 'Owner' });
  });
  it('serializes refresh between two client instances using the same Web Lock', async () => {
    let tail = Promise.resolve();
    let active = 0;
    let maxActive = 0;
    vi.stubGlobal('navigator', {
      locks: {
        request: (_name: string, fn: () => Promise<unknown>) => {
          const run = tail.then(fn);
          tail = run.then(
            () => {},
            () => {},
          );
          return run;
        },
      },
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.endsWith('/auth/csrf')) return json({ csrf_token: 'csrf' });
        if (url.endsWith('/auth/refresh')) {
          active++;
          maxActive = Math.max(maxActive, active);
          await new Promise((resolve) => setTimeout(resolve, 5));
          active--;
          return tokens();
        }
        return json(profile);
      }),
    );
    const a = new SessionClient();
    const b = new SessionClient();
    await Promise.all([a.bootstrap(), b.bootstrap()]);
    expect(maxActive).toBe(1);
    expect(a.snapshot().status).toBe('authenticated');
    expect(b.snapshot().status).toBe('authenticated');
  });
});
