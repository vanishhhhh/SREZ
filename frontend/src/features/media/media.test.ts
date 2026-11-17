import { describe, it, expect, vi, afterEach } from 'vitest';
import { jsonRequest } from '../../lib/http';
import { session } from '../../lib/client';
import { beginUpload, validateFile } from './api';

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
describe('Media transport uses Go instead of direct storage', () => {
  it('sends raw File bytes with their MIME, without setting Content-Length', async () => {
    const file = new File(['image bytes'], 'photo.png', { type: 'image/png' });
    let sent: RequestInit | undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init: RequestInit) => {
        sent = init;
        return new Response(JSON.stringify({ ok: true }), {
          headers: { 'Content-Type': 'application/json' },
        });
      }),
    );
    await jsonRequest('/media/uploads/1/content', { method: 'PUT', body: file });
    expect(sent?.body).toBe(file);
    expect(sent?.headers).toMatchObject({ 'Content-Type': 'image/png' });
    expect(new Headers(sent?.headers).has('Content-Length')).toBe(false);
  });
  it('reads image bytes without attempting JSON decoding', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(new Blob(['normalized'], { type: 'image/jpeg' }), {
            headers: { 'Content-Type': 'image/jpeg' },
          }),
      ),
    );
    const { data } = await jsonRequest<Blob>('/media/assets/1/content', { responseType: 'blob' });
    expect(data.type).toBe('image/jpeg');
    expect(await data.text()).toBe('normalized');
  });
  it('rejects an external upload URL before sending any bearer-protected file', async () => {
    vi.spyOn(session, 'request').mockResolvedValue({
      upload: { id: 1 },
      content_url: 'https://storage.example.com/private',
    });
    await expect(
      beginUpload(new File(['a'], 'a.png', { type: 'image/png' }), { purpose: 'user_avatar' }),
    ).rejects.toThrow();
    expect(session.request).toHaveBeenCalledTimes(1);
  });
  it('rejects empty files and unsupported formats before POST', () => {
    expect(() => validateFile(new File([], 'empty.png', { type: 'image/png' }))).toThrow();
    expect(() => validateFile(new File(['svg'], 'a.svg', { type: 'image/svg+xml' }))).toThrow();
  });
  it('enforces the existing 5 MiB source limit', () => {
    expect(() =>
      validateFile(
        new File([new Uint8Array(5 * 1024 * 1024 + 1)], 'large.png', { type: 'image/png' }),
      ),
    ).toThrow();
  });
});
