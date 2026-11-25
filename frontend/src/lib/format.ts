export function money(kopecks: number): string {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    maximumFractionDigits: kopecks % 100 ? 2 : 0,
  }).format(kopecks / 100);
}
export function rublesInput(kopecks: number): string {
  return (kopecks / 100).toFixed(2).replace(/\.00$/, '');
}
export function parsePrice(value: string): number {
  const match = /^(\d+)(?:[.,](\d{1,2}))?$/.exec(value.trim());
  if (!match) throw new Error('Укажите цену в рублях, не больше двух знаков после запятой.');
  const kopecks = BigInt(match[1]) * 100n + BigInt((match[2] || '').padEnd(2, '0'));
  if (kopecks > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error('Слишком большая цена.');
  return Number(kopecks);
}
export function parseStock(value: string): number {
  if (!/^\d+$/.test(value.trim()))
    throw new Error('Остаток должен быть целым неотрицательным числом.');
  const n = Number(value);
  if (!Number.isSafeInteger(n)) throw new Error('Слишком большой остаток.');
  return n;
}
export function httpsURL(value: string, optional = false): boolean {
  if (!value) return optional;
  try {
    const url = new URL(value);
    return (
      url.protocol === 'https:' &&
      !!url.hostname &&
      !url.username &&
      !url.password &&
      !/\s/.test(value)
    );
  } catch {
    return false;
  }
}
export function passwordValid(value: string): boolean {
  return Array.from(value).length >= 8 && new TextEncoder().encode(value).length <= 1024;
}
export function initials(value: string): string {
  return (
    value
      .trim()
      .split(/\s+/)
      .slice(0, 2)
      .map((s) => s[0])
      .join('')
      .toUpperCase() || 'S'
  );
}
export function validReturnPath(value: unknown): string {
  return typeof value === 'string' &&
    value.startsWith('/') &&
    !value.startsWith('//') &&
    !Array.from(value).some((char) => char === '\\' || char.charCodeAt(0) < 32) &&
    !/^\/(login|register|unavailable)(?:[?#]|$)/.test(value)
    ? value
    : '/studio';
}
export const statusName = (status: string) =>
  ({
    draft: 'Черновик',
    active: 'Опубликован',
    published: 'Опубликован',
    hidden: 'Скрыт',
    suspended: 'Заблокирован',
    invited: 'Приглашение',
    revoked: 'Доступ отозван',
    declined: 'Отклонено',
  })[status] || status;
export function errorText(error: unknown): string {
  if (error instanceof Error) return error.message;
  return 'Что-то пошло не так. Попробуйте ещё раз.';
}
