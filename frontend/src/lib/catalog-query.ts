import { parsePrice, rublesInput } from './format';

export const filterNames = ['category', 'brand_id', 'min_price', 'max_price', 'size', 'color'];
export type CatalogFilters = {
  category: string;
  brand_id: string;
  min: string;
  max: string;
  size: string;
  color: string;
};
function priceField(value: string | null) {
  if (!value || !/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) return '';
  return rublesInput(Number(value));
}
export function filtersFrom(params: URLSearchParams): CatalogFilters {
  return {
    category: params.get('category') || '',
    brand_id: params.get('brand_id') || '',
    min: priceField(params.get('min_price')),
    max: priceField(params.get('max_price')),
    size: params.get('size') || '',
    color: params.get('color') || '',
  };
}
export function applyFilters(filters: CatalogFilters, current: URLSearchParams) {
  const next = new URLSearchParams();
  for (const name of ['category', 'brand_id', 'size', 'color'] as const)
    if (filters[name].trim()) next.set(name, filters[name].trim());
  const min = filters.min.trim() ? parsePrice(filters.min) : null;
  const max = filters.max.trim() ? parsePrice(filters.max) : null;
  if (min !== null && max !== null && min > max)
    throw new Error('Минимальная цена не должна превышать максимальную.');
  if (min !== null) next.set('min_price', String(min));
  if (max !== null) next.set('max_price', String(max));
  next.set('sort', current.get('sort') || 'newest');
  if (current.has('limit')) next.set('limit', current.get('limit')!);
  next.set('page', '1');
  return next;
}
export function countThings(count: number) {
  const ending = count % 100;
  const word =
    ending >= 11 && ending <= 14
      ? 'вещей'
      : count % 10 === 1
        ? 'вещь'
        : count % 10 >= 2 && count % 10 <= 4
          ? 'вещи'
          : 'вещей';
  return `${count} ${word}`;
}
