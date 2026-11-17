import { session } from '../../lib/client';
import { ApiError } from '../../lib/http';

export interface UploadTarget {
  purpose: 'user_avatar' | 'brand_logo' | 'brand_banner' | 'product_image';
  brand_id?: number;
  product_id?: number;
  color_id?: number;
  image_id?: number;
  position?: number;
}
export interface MediaUpload {
  id: number;
  media_id: number;
  image_id?: number;
  state: string;
  asset: { id: number; state: string };
}
export function validateFile(file: File) {
  if (!['image/jpeg', 'image/png', 'image/webp'].includes(file.type))
    throw new Error('Выберите JPEG, PNG или WebP.');
  if (file.size < 1 || file.size > 5 * 1024 * 1024)
    throw new Error('Размер фотографии должен быть от 1 байта до 5 МБ.');
}
export async function beginUpload(file: File, target: UploadTarget, signal?: AbortSignal) {
  validateFile(file);
  const result = await session.request<{ upload: MediaUpload; content_url: string }>(
    '/media/uploads',
    {
      method: 'POST',
      auth: true,
      signal,
      body: { ...target, content_type: file.type, size_bytes: file.size },
    },
  );
  if (
    !result ||
    !Number.isSafeInteger(result.upload?.id) ||
    result.upload.id <= 0 ||
    result.content_url !== `/media/uploads/${result.upload.id}/content`
  )
    throw new ApiError(502, 'invalid response');
  return result.content_url;
}
export async function putUpload(path: string, file: File, signal?: AbortSignal) {
  if (!/^\/media\/uploads\/[1-9]\d*\/content$/.test(path))
    throw new ApiError(502, 'invalid response');
  const result = await session.request<MediaUpload>(path, {
    method: 'PUT',
    auth: true,
    signal,
    body: file,
  });
  if (
    !result ||
    result.state !== 'complete' ||
    result.asset?.state !== 'ready' ||
    !Number.isSafeInteger(result.media_id)
  )
    throw new ApiError(502, 'invalid response');
  return result;
}
export const privateImage = (path: string, signal: AbortSignal) =>
  session.request<Blob>(path, { auth: true, signal, responseType: 'blob' });
export const deleteAvatar = () =>
  session.request<void>('/profile/avatar', { method: 'DELETE', auth: true });
export const deleteBrandImage = (brand: number, purpose: 'brand_logo' | 'brand_banner') =>
  session.request<void>(`/brands/${brand}/media/${purpose}`, { method: 'DELETE', auth: true });
