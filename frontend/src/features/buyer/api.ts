import { session } from '../../lib/client';
import { ApiError } from '../../lib/http';
import { readProductCard } from '../../lib/public-data';
import type { Cart, Favorite, Order } from './types';
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === 'object' && !Array.isArray(v);
const num = (v: unknown, min = 0): v is number =>
  typeof v === 'number' && Number.isSafeInteger(v) && v >= min;
const quantity = (v: unknown) => num(v, 1) && v <= 100;
const text = (v: unknown) => typeof v === 'string';
const date = (v: unknown) => typeof v === 'string' && Number.isFinite(Date.parse(v));
function ensure(v: unknown): asserts v {
  if (!v) throw new ApiError(502, 'invalid response');
}
export function readFavorites(v: unknown): Favorite[] {
  ensure(Array.isArray(v));
  return v.map((f) => {
    ensure(obj(f) && num(f.product_id, 1));
    if (f.product !== null) {
      const card = readProductCard(f.product);
      ensure(card.id === f.product_id);
    }
    return f as unknown as Favorite;
  });
}
export function readCart(v: unknown): Cart {
  ensure(obj(v) && Array.isArray(v.items) && num(v.total_kopecks));
  let total = 0;
  for (const line of v.items) {
    ensure(
      obj(line) &&
        num(line.id, 1) &&
        quantity(line.quantity) &&
        num(line.subtotal_kopecks) &&
        ['available', 'insufficient_stock', 'unavailable'].includes(String(line.availability)) &&
        (line.product_variant_id === null || num(line.product_variant_id, 1)),
    );
    if (line.availability === 'unavailable') {
      ensure(line.product === null && line.subtotal_kopecks === 0);
    } else {
      const p = line.product;
      ensure(
        obj(p) &&
          num(p.product_id, 1) &&
          num(p.brand_id, 1) &&
          num(p.unit_price_kopecks) &&
          ['name', 'preview_image_url', 'color', 'size'].every((k) => text(p[k])) &&
          num(line.product_variant_id, 1),
      );
      ensure(
        line.subtotal_kopecks ===
          (line.availability === 'available'
            ? p.unit_price_kopecks * (line.quantity as number)
            : 0),
      );
    }
    total += line.subtotal_kopecks as number;
    ensure(num(total));
  }
  ensure(total === v.total_kopecks);
  return v as unknown as Cart;
}
export function readOrder(v: unknown): Order {
  ensure(
    obj(v) &&
      num(v.id, 1) &&
      ['unpaid', 'cancelled', 'expired'].includes(String(v.status)) &&
      num(v.total_kopecks) &&
      date(v.created_at) &&
      date(v.cancel_before) &&
      Date.parse(v.cancel_before as string) > Date.parse(v.created_at as string) &&
      Array.isArray(v.items) &&
      v.items.length > 0,
  );
  ensure(
    (v.cancelled_at === null || date(v.cancelled_at)) &&
      (v.expired_at === null || date(v.expired_at)),
  );
  let total = 0;
  for (const line of v.items) {
    ensure(
      obj(line) &&
        ['product_variant_id', 'product_id', 'brand_id'].every((k) => num(line[k], 1)) &&
        ['product_name', 'color', 'size'].every((k) => text(line[k])) &&
        quantity(line.quantity) &&
        num(line.unit_price_kopecks) &&
        num(line.subtotal_kopecks) &&
        line.subtotal_kopecks === line.unit_price_kopecks * (line.quantity as number),
    );
    total += line.subtotal_kopecks as number;
    ensure(num(total));
  }
  ensure(total === v.total_kopecks);
  return v as unknown as Order;
}
const get = <T>(path: string, read: (v: unknown) => T, signal?: AbortSignal) =>
  session.request<unknown>(path, { auth: true, signal }).then(read);
const write = <T>(path: string, method: string, body: unknown, read: (v: unknown) => T) =>
  session.request<unknown>(path, { auth: true, method, body }).then(read);
export const buyerAPI = {
  favorites: (signal?: AbortSignal) => get('/favorites', readFavorites, signal),
  favorite: (id: number, add: boolean) =>
    session.request<void>(`/favorites/${id}`, { method: add ? 'POST' : 'DELETE', auth: true }),
  cart: (signal?: AbortSignal) => get('/cart', readCart, signal),
  addCart: (variant: number, quantity: number) =>
    write('/cart/items', 'POST', { product_variant_id: variant, quantity }, readCart),
  setQuantity: (id: number, quantity: number) =>
    write(`/cart/items/${id}`, 'PATCH', { quantity }, readCart),
  removeCart: (id: number) =>
    session.request<void>(`/cart/items/${id}`, { method: 'DELETE', auth: true }),
  checkout: () => write('/orders', 'POST', {}, readOrder),
  orders: (signal?: AbortSignal) =>
    get(
      '/orders',
      (v) => {
        ensure(Array.isArray(v));
        return v.map(readOrder);
      },
      signal,
    ),
  order: (id: string, signal?: AbortSignal) =>
    get('/orders/' + encodeURIComponent(id), readOrder, signal),
  cancel: (id: number) => write(`/orders/${id}/cancel`, 'POST', {}, readOrder),
};
export const cartFingerprint = (cart: Cart) => JSON.stringify(cart);
