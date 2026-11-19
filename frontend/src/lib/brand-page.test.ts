import { describe, expect, it } from 'vitest';
import {
  brandContrast,
  brandPalette,
  defaultBrandPage,
  readBrandPageConfig,
  readBrandPageEditor,
} from './brand-page';

describe('brand page configuration', () => {
  it('preserves block order and rejects unsupported sources, versions and duplicates', () => {
    const config = defaultBrandPage();
    config.blocks.reverse();
    expect(readBrandPageConfig(config)).toEqual(config);
    expect(readBrandPageConfig({ ...config, schema_version: 4 })).toBeUndefined();
    expect(
      readBrandPageConfig({ ...config, blocks: [config.blocks[0], ...config.blocks] }),
    ).toBeUndefined();
    const blocks: unknown[] = [...config.blocks];
    blocks[0] = { ...config.blocks[0], content: { source: 'https://example.test/media' } };
    expect(readBrandPageConfig({ ...config, blocks })).toBeUndefined();
    expect(
      readBrandPageEditor({ revision: 2, draft: config, published: config, published_revision: 3 }),
    ).toBeUndefined();
  });

  it('reads repeated V2 blocks and rejects invalid focal points and incomplete galleries', () => {
    const v1 = defaultBrandPage();
    const photo = {
      id: 'one',
      type: 'image',
      layout: 'contained',
      content: {
        source: 'editorial',
        images: [{ media_id: 12, crop: { x: 0.12, y: 1 } }],
      },
    };
    const v2 = { ...v1, schema_version: 2, blocks: [...v1.blocks, photo, { ...photo, id: 'two' }] };
    expect(readBrandPageConfig(v2)).toEqual(v2);
    expect(readBrandPageConfig({ ...v2, schema_version: 1 })).toBeUndefined();
    const invalid = {
      ...photo,
      content: { ...photo.content, images: [{ media_id: 12, crop: { x: -1, y: 0 } }] },
    };
    expect(readBrandPageConfig({ ...v2, blocks: [...v1.blocks, invalid] })).toBeUndefined();
    const gallery = { ...photo, type: 'horizontal-gallery', layout: 'carousel' };
    expect(readBrandPageConfig({ ...v2, blocks: [...v1.blocks, gallery] })).toBeUndefined();
  });

  it('composites short and long alpha HEX and replaces unreadable colors', () => {
    const palette = brandPalette({
      background_color: '#0008',
      text_color: '#ffffff22',
      accent_color: '#fff0',
    });
    expect(palette.background).toEqual([115, 114, 112]);
    expect(palette.adjusted).toBe(true);
    expect(brandContrast(palette.text, palette.background)).toBeGreaterThanOrEqual(4.5);
    expect(brandContrast(palette.accent, palette.background)).toBeGreaterThanOrEqual(4.5);
    expect(brandPalette(defaultBrandPage().appearance).adjusted).toBe(false);
  });
});
