import { deflateSync } from 'node:zlib';
import type { APIRequestContext } from '@playwright/test';
import { expect } from '@playwright/test';
export function fixturePNG(tone = 35): Buffer {
  const width = 320,
    height = 400,
    raw = Buffer.alloc((width * 3 + 1) * height);
  for (let y = 0; y < height; y++) {
    const offset = y * (width * 3 + 1);
    for (let x = 0; x < width; x++) {
      const body =
        (y > 100 && y < 340 && x > 85 && x < 235) || (y > 80 && y < 200 && x > 50 && x < 270);
      const c = body ? tone : 225 + Math.floor(y / 40);
      for (let k = 0; k < 3; k++) raw[offset + 1 + x * 3 + k] = c;
    }
  }
  function chunk(type: string, data: Buffer) {
    const name = Buffer.from(type),
      body = Buffer.concat([name, data]);
    let crc = 0xffffffff;
    for (const byte of body) {
      crc ^= byte;
      for (let b = 0; b < 8; b++) crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    const out = Buffer.alloc(data.length + 12);
    out.writeUInt32BE(data.length);
    name.copy(out, 4);
    data.copy(out, 8);
    out.writeUInt32BE((crc ^ 0xffffffff) >>> 0, out.length - 4);
    return out;
  }
  const header = Buffer.alloc(13);
  header.writeUInt32BE(width);
  header.writeUInt32BE(height, 4);
  header[8] = 8;
  header[9] = 2;
  return Buffer.concat([
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw)),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}
export async function uploadFixture(
  api: APIRequestContext,
  token: string,
  target: Record<string, unknown>,
  tone = 35,
) {
  const bytes = fixturePNG(tone);
  const headers = { Authorization: 'Bearer ' + token };
  const create = await api.post('/api/media/uploads', {
    headers,
    data: { ...target, content_type: 'image/png', size_bytes: bytes.length },
  });
  expect(create.status()).toBe(201);
  const pending = await create.json();
  const uploaded = await api.put('/api' + pending.content_url, {
    headers: { ...headers, 'Content-Type': 'image/png' },
    data: bytes,
  });
  expect(uploaded.status()).toBe(200);
  return uploaded.json();
}
