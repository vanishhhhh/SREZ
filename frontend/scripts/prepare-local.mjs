import { randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync, existsSync, copyFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { resolve, dirname } from 'node:path';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const target = resolve(root, '.env');
if (!existsSync(target)) copyFileSync(resolve(root, '.env.example'), target);
const original = readFileSync(target, 'utf8');
const lines = original.replace(/^\uFEFF/, '').split(/\r?\n/);
const values = [];
for (const key of ['SREZ_JWT_KEY', 'SREZ_CSRF_KEY', 'SREZ_WISHLIST_KEY']) {
  const matches = lines
    .map((line, i) => (line.startsWith(key + '=') ? i : -1))
    .filter((i) => i >= 0);
  const index = matches.at(-1) ?? -1;
  let value =
    index < 0
      ? ''
      : lines[index]
          .slice(key.length + 1)
          .trim()
          .replace(/^(['"])(.*)\1$/, '$2');
  if (!value) {
    value = randomBytes(32).toString('base64');
    if (index < 0) lines.push(key + '=' + value);
    else lines[index] = key + '=' + value;
  } else if (!/^[A-Za-z0-9+/]+={0,2}$/.test(value) || Buffer.from(value, 'base64').length < 32) {
    throw new Error(
      `${key}: уже заданное значение некорректно. Исправьте локальный .env; скрипт его не перезаписывает.`,
    );
  }
  values.push(value);
}
if (new Set(values).size !== values.length)
  throw new Error('JWT, CSRF и вишлисты требуют разных ключей. Существующие значения не изменены.');
for (const key of ['SREZ_S3_ACCESS_KEY', 'SREZ_S3_SECRET_KEY']) {
  const index = lines.findLastIndex((line) => line.startsWith(key + '='));
  const value =
    index < 0
      ? ''
      : lines[index]
          .slice(key.length + 1)
          .trim()
          .replace(/^(['"])(.*)\1$/, '$2');
  if (!value) {
    const entry = key + '=' + randomBytes(32).toString('hex');
    if (index < 0) lines.push(entry);
    else lines[index] = entry;
  }
}
const output = lines.join('\n');
if (output !== original) writeFileSync(target, output, { mode: 0o600 });
console.log(
  'Локальный .env готов. Существующие значения сохранены; пустые ключи заполнены независимо. Секреты не выводятся.',
);
console.log(
  'Для интеграционной копии используйте compose.integration.yaml и http://localhost:5174.',
);
