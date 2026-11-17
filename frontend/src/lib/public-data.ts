import { ApiError } from './http';
import type { Brand, ProductCard, ProductDetail, ProductPage } from './types';

const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === 'object' && !Array.isArray(v);
const strings = (v: unknown): v is string[] =>
  Array.isArray(v) && v.every((x) => typeof x === 'string');
const integer = (v: unknown, min = 0): v is number =>
  typeof v === 'number' && Number.isSafeInteger(v) && v >= min;
function ensure(condition: unknown): asserts condition {
  if (!condition) throw new ApiError(502, 'invalid response');
}
function product(v: unknown): v is Record<string, unknown> {
  return (
    object(v) &&
    integer(v.id, 1) &&
    integer(v.brand_id, 1) &&
    integer(v.price_kopecks) &&
    typeof v.name === 'string' &&
    typeof v.description === 'string' &&
    typeof v.category === 'string' &&
    strings(v.materials)
  );
}
export function readBrand(value: unknown): Brand {
  ensure(
    object(value) &&
      integer(value.id, 1) &&
      ['name', 'slug', 'description', 'logo_url', 'banner_url'].every(
        (k) => typeof value[k] === 'string',
      ) &&
      object(value.contacts) &&
      ['email', 'telegram_url', 'website_url'].every(
        (k) => typeof (value.contacts as Record<string, unknown>)[k] === 'string',
      ),
  );
  return value as unknown as Brand;
}
export function readBrands(value: unknown): Brand[] {
  ensure(Array.isArray(value));
  return value.map(readBrand);
}
export function readProduct(value: unknown): ProductDetail {
  ensure(product(value) && Array.isArray(value.colors));
  for (const c of value.colors) {
    ensure(
      object(c) &&
        integer(c.id, 1) &&
        typeof c.color === 'string' &&
        strings(c.images) &&
        Array.isArray(c.variants),
    );
    for (const v of c.variants)
      ensure(object(v) && integer(v.id, 1) && typeof v.size === 'string' && integer(v.stock));
  }
  return value as unknown as ProductDetail;
}
export function readProductCard(p: unknown): ProductCard {
  ensure(
    product(p) &&
      ['brand_name', 'brand_slug', 'preview_image_url'].every((k) => typeof p[k] === 'string') &&
      typeof p.is_available === 'boolean' &&
      strings(p.available_colors) &&
      strings(p.available_sizes) &&
      Array.isArray(p.colors),
  );
  return p as unknown as ProductCard;
}
export function readProductPage(value: unknown, headers: Headers): ProductPage {
  ensure(Array.isArray(value));
  const items = value.map(readProductCard);
  const read = (key: string, min: number) => {
    const raw = headers.get(key);
    ensure(raw !== null && /^\d+$/.test(raw) && integer(Number(raw), min));
    return Number(raw);
  };
  const total = read('X-Total-Count', 0),
    page = read('X-Page', 1),
    limit = read('X-Limit', 1),
    pages = read('X-Total-Pages', 0);
  ensure(limit <= 100 && pages === Math.ceil(total / limit) && value.length <= limit);
  return { items, total, page, limit, pages };
}
