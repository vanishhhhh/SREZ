import { session } from '../../lib/client';
import { ApiError } from '../../lib/http';
import { readProductCard } from '../../lib/public-data';
import type { ProductCard } from '../../lib/types';

export type WishlistSummary = {
  id: number;
  title: string;
  description: string;
  visibility: 'private' | 'public';
  item_count: number;
  created_at: string;
  updated_at: string;
};
export type WishlistVariant = { id: number; color: string; size: string; stock: number };
export type WishlistItem = {
  id: number;
  product_id: number;
  product_variant_id: number | null;
  position: number;
  product: ProductCard | null;
  variant: WishlistVariant | null;
};
export type Wishlist = WishlistSummary & { share_path: string | null; items: WishlistItem[] };
export type SharedWishlist = {
  title: string;
  description: string;
  updated_at: string;
  items: WishlistItem[];
};
export const wishlistLimits = { lists: 20, items: 200, title: 100, description: 2000 };
const obj = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === 'object' && !Array.isArray(v);
const int = (v: unknown, min = 0): v is number =>
  typeof v === 'number' && Number.isSafeInteger(v) && v >= min;
const date = (v: unknown) => typeof v === 'string' && Number.isFinite(Date.parse(v));
function ensure(value: unknown): asserts value {
  if (!value) throw new ApiError(502, 'invalid response');
}
export function readWishlistSummary(value: unknown): WishlistSummary {
  ensure(
    obj(value) &&
      int(value.id, 1) &&
      typeof value.title === 'string' &&
      typeof value.description === 'string' &&
      ['private', 'public'].includes(String(value.visibility)) &&
      int(value.item_count) &&
      value.item_count <= 200 &&
      date(value.created_at) &&
      date(value.updated_at),
  );
  return value as unknown as WishlistSummary;
}
function items(value: unknown): WishlistItem[] {
  ensure(Array.isArray(value) && value.length <= 200);
  const ids = new Set<number>(),
    products = new Set<number>();
  return value.map((row, index) => {
    ensure(
      obj(row) &&
        int(row.id, 1) &&
        int(row.product_id, 1) &&
        row.position === index &&
        (row.product_variant_id === null || int(row.product_variant_id, 1)) &&
        !ids.has(row.id) &&
        !products.has(row.product_id),
    );
    ids.add(row.id);
    products.add(row.product_id);
    if (row.product !== null) ensure(readProductCard(row.product).id === row.product_id);
    if (row.variant !== null)
      ensure(
        obj(row.variant) &&
          row.product !== null &&
          row.variant.id === row.product_variant_id &&
          int(row.variant.id, 1) &&
          int(row.variant.stock) &&
          typeof row.variant.color === 'string' &&
          typeof row.variant.size === 'string',
      );
    return row as unknown as WishlistItem;
  });
}
export function readWishlist(value: unknown): Wishlist {
  const summary = readWishlistSummary(value);
  ensure(obj(value));
  const rows = items(value.items);
  ensure(
    rows.length === summary.item_count &&
      (summary.visibility === 'private'
        ? value.share_path === null
        : typeof value.share_path === 'string' &&
          /^\/w\/[A-Za-z0-9_-]{43}$/.test(value.share_path)),
  );
  return { ...summary, items: rows, share_path: value.share_path as string | null };
}
export function readSharedWishlist(value: unknown): SharedWishlist {
  ensure(
    obj(value) &&
      typeof value.title === 'string' &&
      typeof value.description === 'string' &&
      date(value.updated_at),
  );
  const rows = items(value.items);
  ensure(rows.every((row) => row.product !== null));
  return {
    title: value.title,
    description: value.description,
    updated_at: value.updated_at as string,
    items: rows,
  };
}
const root = (id: number | string) => '/wishlists/' + encodeURIComponent(id);
const write = (method: string, path: string, body?: unknown) =>
  session.request<unknown>(path, { method, auth: true, body });
export const wishlistAPI = {
  async list(signal?: AbortSignal) {
    const value = await session.request<unknown>('/wishlists', { auth: true, signal });
    ensure(Array.isArray(value) && value.length <= 20);
    return value.map(readWishlistSummary);
  },
  get: (id: string | number, signal?: AbortSignal) =>
    session.request<unknown>(root(id), { auth: true, signal }).then(readWishlist),
  shared: (token: string, signal?: AbortSignal) =>
    session
      .request<unknown>('/wishlists/shared/' + encodeURIComponent(token), { signal })
      .then(readSharedWishlist),
  create: (title: string, description: string) =>
    write('POST', '/wishlists', { title: title.trim(), description: description.trim() }).then(
      readWishlist,
    ),
  update: (id: number, title: string, description: string) =>
    write('PATCH', root(id), { title: title.trim(), description: description.trim() }).then(
      readWishlist,
    ),
  remove: (id: number) => write('DELETE', root(id)),
  add: (id: number, product_id: number, product_variant_id: number | null) =>
    write('POST', root(id) + '/items', { product_id, product_variant_id }).then(readWishlist),
  variant: (id: number, item: number, product_variant_id: number | null) =>
    write('PATCH', root(id) + '/items/' + item, { product_variant_id }).then(readWishlist),
  removeItem: (id: number, item: number) =>
    write('DELETE', root(id) + '/items/' + item).then(readWishlist),
  reorder: (id: number, item_ids: number[]) =>
    write('POST', root(id) + '/items/reorder', { item_ids }).then(readWishlist),
  share: (id: number) => write('POST', root(id) + '/share', {}).then(readWishlist),
  unshare: (id: number) => write('DELETE', root(id) + '/share').then(readWishlist),
};
