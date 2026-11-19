import type { BrandPageAppearance, BrandPageConfig } from './brand-page';

export type Role = 'owner' | 'editor';
export type ProductStatus = 'draft' | 'published' | 'hidden';
export type { Profile, AuthResult, RegisterInput } from './auth-types';
export interface Brand {
  id: number;
  name: string;
  slug: string;
  description: string;
  logo_url: string;
  banner_url: string;
  contacts: { email: string; telegram_url: string; website_url: string };
  appearance: BrandPageAppearance;
  page?: BrandPageConfig;
  status: 'draft' | 'active' | 'suspended';
  created_at: string;
  updated_at: string;
}
export interface MyBrand {
  brand: Brand;
  role: Role;
}
export interface Membership {
  user_id: number;
  brand_id: number;
  role: Role;
  status: 'invited' | 'active' | 'revoked' | 'declined';
}
export interface Product {
  id: number;
  brand_id: number;
  name: string;
  description: string;
  price_kopecks: number;
  category: string;
  materials: string[];
  status: ProductStatus;
  created_at: string;
  updated_at: string;
}
export interface CardColor {
  id: number;
  color: string;
  preview_image_url: string;
  is_available: boolean;
  available_sizes: string[];
}
export interface ProductCard extends Product {
  preview_image_url: string;
  brand_name: string;
  brand_slug: string;
  is_available: boolean;
  available_colors: string[];
  available_sizes: string[];
  colors: CardColor[];
}
export interface PublicVariant {
  id: number;
  size: string;
  stock: number;
  sku: string;
}
export interface PublicColor {
  id: number;
  color: string;
  images: string[];
  variants: PublicVariant[];
}
export interface ProductDetail extends Product {
  colors: PublicColor[];
}
export interface ManagedColor {
  id: number;
  product_id: number;
  color: string;
  images: string[];
}
export interface ManagedVariant extends PublicVariant {
  product_color_id: number;
}
export interface ManagedImage {
  media_id?: number;
  id: number;
  product_color_id: number;
  url: string;
  position: number;
}
export interface ProductPage {
  items: ProductCard[];
  total: number;
  page: number;
  limit: number;
  pages: number;
}
export type ProductInput = Pick<
  Product,
  'name' | 'description' | 'price_kopecks' | 'category' | 'materials'
>;
export type BrandInput = Pick<Brand, 'name' | 'slug' | 'description' | 'contacts'>;
export const categories = [
  ['tshirt', 'Футболки'],
  ['hoodie', 'Худи и свитшоты'],
  ['pants', 'Брюки'],
  ['outerwear', 'Верхняя одежда'],
  ['footwear', 'Обувь'],
  ['accessories', 'Аксессуары'],
  ['other', 'Другое'],
] as const;
export const categoryName = (value: string) =>
  categories.find(([id]) => id === value)?.[1] || 'Без категории';
