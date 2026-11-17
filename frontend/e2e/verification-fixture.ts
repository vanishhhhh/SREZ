import { expect } from '@playwright/test';
import type { APIRequestContext } from '@playwright/test';

// Only the isolated Mailpit instance. Never query a real mailbox/provider.
export async function verificationMessage(api: APIRequestContext, email: string, exceptID = '') {
  return accountMessage(api, email, 'verify-email', exceptID);
}
export async function resetMessage(
  api: APIRequestContext,
  email: string,
  exceptID: string | string[] = '',
) {
  return accountMessage(api, email, 'reset-password', exceptID);
}
async function accountMessage(
  api: APIRequestContext,
  email: string,
  path: string,
  exceptID: string | string[] = '',
) {
  const excluded = Array.isArray(exceptID) ? exceptID : [exceptID];
  const subject = path === 'reset-password' ? 'Сброс пароля — SREZ' : 'Подтвердите email — SREZ';
  let id = '';
  await expect
    .poll(
      async () => {
        const response = await api.get(
          'http://127.0.0.1:18025/api/v1/search?query=' + encodeURIComponent('to:' + email),
        );
        if (!response.ok()) return false;
        const data = await response.json();
        id =
          data.messages?.find(
            (m: { ID: string; Subject: string }) =>
              !excluded.includes(m.ID) && m.Subject === subject,
          )?.ID || '';
        return !!id;
      },
      { timeout: 15000 },
    )
    .toBe(true);
  const response = await api.get('http://127.0.0.1:18025/api/v1/message/' + encodeURIComponent(id));
  expect(response.status()).toBe(200);
  const message = await response.json();
  const link = (message.Text as string).match(
    new RegExp('http://localhost:5175/' + path + '#token=[A-Za-z0-9_-]{43}'),
  )?.[0];
  if (!link) throw Error('Mailpit verification message has no expected local link');
  return {
    id,
    link,
    token: new URLSearchParams(new URL(link).hash.slice(1)).get('token')!,
    message,
  };
}
export async function verifyFixtureAccount(api: APIRequestContext, email: string) {
  const { token } = await verificationMessage(api, email);
  const response = await api.post('/api/auth/verify-email', {
    headers: { Origin: 'http://localhost:5175' },
    data: { token },
  });
  expect(response.status()).toBe(200);
}
