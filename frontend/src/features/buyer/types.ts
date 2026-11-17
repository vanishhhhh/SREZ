import type { ProductCard } from '../../lib/types';
export interface Favorite {
  product_id: number;
  product: ProductCard | null;
}
export interface CartProduct {
  product_id: number;
  brand_id: number;
  name: string;
  preview_image_url: string;
  color: string;
  size: string;
  unit_price_kopecks: number;
}
export interface CartLine {
  id: number;
  product_variant_id: number | null;
  quantity: number;
  product: CartProduct | null;
  availability: 'available' | 'insufficient_stock' | 'unavailable';
  subtotal_kopecks: number;
}
export interface Cart {
  items: CartLine[];
  total_kopecks: number;
}
export interface OrderLine {
  product_variant_id: number;
  product_id: number;
  brand_id: number;
  product_name: string;
  color: string;
  size: string;
  quantity: number;
  unit_price_kopecks: number;
  subtotal_kopecks: number;
}
export interface Order {
  id: number;
  status: 'unpaid' | 'cancelled' | 'expired';
  total_kopecks: number;
  created_at: string;
  cancel_before: string;
  cancelled_at: string | null;
  expired_at: string | null;
  items: OrderLine[];
}
