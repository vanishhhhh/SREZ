import { afterEach, expect, it, vi } from 'vitest';
import { SessionClient } from './client';
afterEach(() => vi.unstubAllGlobals());
it('does not create a session after registration, including failed SMTP', async () => {
  const response = {
    id: 7,
    username: 'pending',
    display_name: 'Pending',
    avatar_url: '',
    email_verification_required: true,
    email_delivery_status: 'unavailable',
    resend_after_seconds: 60,
  };
  const fetcher = vi.fn(
    async (_url: string, _options?: RequestInit) =>
      new Response(JSON.stringify(response), { status: 201 }),
  );
  vi.stubGlobal('fetch', fetcher);
  const client = new SessionClient();
  const result = await client.register({
    email: 'pending@example.com',
    username: 'pending',
    display_name: 'Pending',
    password: 'long password 123',
  });
  expect(result.email_delivery_status).toBe('unavailable');
  expect(fetcher).toHaveBeenCalledTimes(1);
  expect(fetcher.mock.calls[0]).toHaveLength(2);
  expect(JSON.stringify(client.snapshot())).not.toContain('access_token');
});
it('keeps an unverified password-checked account anonymous, rather than a server outage', async () => {
  const fetcher = vi.fn(
    async () =>
      new Response(JSON.stringify({ error: 'email verification required' }), { status: 403 }),
  );
  vi.stubGlobal('fetch', fetcher);
  const client = new SessionClient();
  await expect(client.login('pending', 'password')).rejects.toMatchObject({
    code: 'email verification required',
  });
  expect(client.snapshot().status).toBe('anonymous');
  expect(fetcher).toHaveBeenCalledTimes(1);
});
