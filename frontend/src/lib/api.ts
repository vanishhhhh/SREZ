import { readBrandPageEditor } from './brand-page';
import type { BrandPageConfig } from './brand-page';
import { ApiError } from './http';
import { session } from './client';
import { readBrand, readBrands, readProduct, readProductPage } from './public-data';
import type {
  Brand,
  BrandInput,
  ManagedColor,
  ManagedImage,
  ManagedVariant,
  Membership,
  MyBrand,
  Product,
  ProductInput,
  ProductPage,
} from './types';

const privateGet = <T>(path: string, signal?: AbortSignal) =>
  session.request<T>(path, { auth: true, signal });
const write = <T>(method: string, path: string, body?: unknown, signal?: AbortSignal) =>
  session.request<T>(path, { method, auth: true, body, signal });
const colorPath = (product: number, color: number) => `/products/${product}/colors/${color}`;
function brandPageEditor(value: unknown) {
  const editor = readBrandPageEditor(value);
  if (!editor) throw new ApiError(502, 'invalid response');
  return editor;
}
export const api = {
  brands: async (signal?: AbortSignal) =>
    readBrands(await session.request<unknown>('/brands', { signal })),
  brand: (slug: string, signal?: AbortSignal) =>
    session.request<unknown>('/brands/' + encodeURIComponent(slug), { signal }).then(readBrand),
  product: (id: string, signal?: AbortSignal) =>
    session.request<unknown>('/products/' + encodeURIComponent(id), { signal }).then(readProduct),
  async products(params: URLSearchParams, signal?: AbortSignal): Promise<ProductPage> {
    const { data, headers } = await session.requestWithHeaders<ProductPage['items']>(
      '/products?' + params.toString(),
      { signal },
    );
    return readProductPage(data, headers);
  },
  myBrands: (signal?: AbortSignal) => privateGet<MyBrand[]>('/me/brands', signal),
  invitations: (signal?: AbortSignal) => privateGet<MyBrand[]>('/me/brand-invitations', signal),
  manageBrand: (id: string, signal?: AbortSignal) =>
    privateGet<Brand>(`/brands/${encodeURIComponent(id)}/manage`, signal),
  brandPageEditor: (id: number, signal?: AbortSignal) =>
    privateGet<unknown>(`/brands/${id}/page/manage`, signal).then(brandPageEditor),
  saveBrandPage: (id: number, revision: number, config: BrandPageConfig, signal?: AbortSignal) =>
    write<unknown>('PUT', `/brands/${id}/page/draft`, { revision, config }, signal).then(
      brandPageEditor,
    ),
  publishBrandPage: (id: number, revision: number, signal?: AbortSignal) =>
    write<unknown>('POST', `/brands/${id}/page/publish`, { revision }, signal).then(
      brandPageEditor,
    ),
  manageProducts: (id: string, signal?: AbortSignal) =>
    privateGet<Product[]>(`/brands/${encodeURIComponent(id)}/products/manage`, signal),
  createBrand: (name: string, slug: string) => write<Brand>('POST', '/brands', { name, slug }),
  patchBrand: (id: number, input: BrandInput) => write<Brand>('PATCH', `/brands/${id}`, input),
  brandAction: (id: number, action: 'publish' | 'unpublish') =>
    write<Brand>('POST', `/brands/${id}/${action}`, {}),
  members: (id: number) => privateGet<Membership[]>(`/brands/${id}/members`),
  invite: (id: number, username: string) =>
    write<Membership>('POST', `/brands/${id}/invitations`, { username }),
  invitation: (id: number, accept: boolean) =>
    write<Membership>('POST', `/brands/${id}/invitations/${accept ? 'accept' : 'decline'}`, {}),
  revoke: (brand: number, user: number) =>
    write<void>('DELETE', `/brands/${brand}/members/${user}`),
  createProduct: (brand_id: number, input: ProductInput) =>
    write<Product>('POST', '/products', { brand_id, ...input }),
  patchProduct: (id: number, input: ProductInput) =>
    write<Product>('PATCH', `/products/${id}`, input),
  productAction: (id: number, action: 'publish' | 'hide' | 'unpublish') =>
    write<Product>('POST', `/products/${id}/${action}`, {}),
  colors: (id: number, signal?: AbortSignal) =>
    privateGet<ManagedColor[]>(`/products/${id}/colors`, signal),
  createColor: (id: number, color: string) =>
    write<ManagedColor>('POST', `/products/${id}/colors`, { color }),
  renameColor: (id: number, colorID: number, color: string) =>
    write<ManagedColor>('PATCH', colorPath(id, colorID), { color }),
  deleteColor: (id: number, colorID: number) => write<void>('DELETE', colorPath(id, colorID)),
  variants: (id: number, color: number, signal?: AbortSignal) =>
    privateGet<ManagedVariant[]>(colorPath(id, color) + '/variants', signal),
  createVariant: (
    id: number,
    color: number,
    input: Pick<ManagedVariant, 'size' | 'stock' | 'sku'>,
  ) => write<ManagedVariant>('POST', colorPath(id, color) + '/variants', input),
  patchVariant: (
    id: number,
    color: number,
    variant: number,
    input: Pick<ManagedVariant, 'size' | 'stock' | 'sku'>,
  ) => write<ManagedVariant>('PATCH', colorPath(id, color) + `/variants/${variant}`, input),
  deleteVariant: (id: number, color: number, variant: number) =>
    write<void>('DELETE', colorPath(id, color) + `/variants/${variant}`),
  images: (id: number, color: number, signal?: AbortSignal) =>
    privateGet<ManagedImage[]>(colorPath(id, color) + '/images', signal),
  createImage: (id: number, color: number, media_id: number) =>
    write<ManagedImage>('POST', colorPath(id, color) + '/images', { media_id }),
  patchImage: (
    id: number,
    color: number,
    image: number,
    input: { media_id?: number; position?: number },
  ) => write<ManagedImage>('PATCH', colorPath(id, color) + `/images/${image}`, input),
  deleteImage: (id: number, color: number, image: number) =>
    write<void>('DELETE', colorPath(id, color) + `/images/${image}`),
  reorderImages: (id: number, color: number, image_ids: number[]) =>
    write<ManagedImage[]>('POST', colorPath(id, color) + '/images/reorder', { image_ids }),
};
