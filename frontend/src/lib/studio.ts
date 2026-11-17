import { api } from './api';
import { ApiError } from './client';
// HTML patterns use Unicode Sets (/v); a literal hyphen must be escaped.
export const brandSlugPattern = String.raw`[a-z0-9\-]{3,60}`;
export async function loadBrandContext(id: string, signal?: AbortSignal) {
  const [brand, mine] = await Promise.all([api.manageBrand(id, signal), api.myBrands(signal)]);
  const membership = mine.find((item) => item.brand.id === brand.id);
  if (!membership) throw new ApiError(404, 'resource not found');
  return { brand, role: membership.role };
}
export function slugFromName(value: string): string {
  const alphabet: Record<string, string> = {
    а: 'a',
    б: 'b',
    в: 'v',
    г: 'g',
    д: 'd',
    е: 'e',
    ё: 'e',
    ж: 'zh',
    з: 'z',
    и: 'i',
    й: 'y',
    к: 'k',
    л: 'l',
    м: 'm',
    н: 'n',
    о: 'o',
    п: 'p',
    р: 'r',
    с: 's',
    т: 't',
    у: 'u',
    ф: 'f',
    х: 'kh',
    ц: 'ts',
    ч: 'ch',
    ш: 'sh',
    щ: 'sch',
    ъ: '',
    ы: 'y',
    ь: '',
    э: 'e',
    ю: 'yu',
    я: 'ya',
  };
  return value
    .toLowerCase()
    .split('')
    .map((letter) => alphabet[letter] ?? letter)
    .join('')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 60);
}
