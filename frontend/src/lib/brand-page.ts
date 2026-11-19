export type BrandPageAppearance = {
  accent_color: string;
  background_color: string;
  text_color: string;
};

export const brandPageBlocks = {
  hero: { label: 'Обложка', source: 'brand_banner', layouts: ['full-width', 'split'] },
  story: {
    label: 'О бренде',
    source: 'brand_story',
    layouts: [
      'text',
      'centered',
      'split',
      'split-reversed',
      'editorial',
      'editorial-reversed',
      'stacked',
    ],
  },
  products: { label: 'Товары', source: 'brand_catalog', layouts: ['grid'] },
  contacts: { label: 'Контакты', source: 'brand_contacts', layouts: ['inline'] },
  image: {
    label: 'Фотография',
    source: 'editorial',
    layouts: ['full-width', 'contained', 'portrait', 'landscape'],
  },
  'photo-text': {
    label: 'Фото и текст',
    source: 'editorial',
    layouts: ['photo-left', 'photo-right', 'stacked', 'text-first'],
  },
  'horizontal-gallery': {
    label: 'Горизонтальная галерея',
    source: 'editorial',
    layouts: ['carousel'],
  },
  'vertical-gallery': { label: 'Вертикальная галерея', source: 'editorial', layouts: ['stacked'] },
  'editorial-grid': {
    label: 'Редакционная сетка',
    source: 'editorial',
    layouts: ['feature', 'diptych', 'wide-top', 'asymmetric', 'unequal', 'staggered'],
  },
  text: { label: 'Текст', source: 'editorial', layouts: ['text'] },
} as const;

export type BlockType = keyof typeof brandPageBlocks;
export type EditorialBlock = Exclude<BlockType, 'hero' | 'story' | 'products' | 'contacts'>;
export const editorialBlocks: EditorialBlock[] = [
  'image',
  'photo-text',
  'horizontal-gallery',
  'vertical-gallery',
  'editorial-grid',
  'text',
];
export type BrandPageCrop = { x: number; y: number };
export type BrandPageImage = {
  media_id: number;
  ratio?: string;
  alt?: string;
  crop?: BrandPageCrop;
  mobile_crop?: BrandPageCrop;
};
export const compositionChoices = {
  vertical: ['top', 'center', 'bottom'],
  text_width: ['narrow', 'regular', 'wide'],
  spacing: ['compact', 'default', 'spacious'],
} as const;
export type BrandComposition = {
  [K in keyof typeof compositionChoices]?: (typeof compositionChoices)[K][number];
};
export type BrandPageBlock = {
  composition?: BrandComposition;
  id: string;
  type: BlockType;
  layout: string;
  content: {
    source: (typeof brandPageBlocks)[BlockType]['source'];
    images?: BrandPageImage[];
    title?: string;
    text?: string;
    size?: string;
    align?: string;
    gap?: string;
    columns?: string;
  };
};
export type BrandPageConfig = {
  schema_version: 1 | 2 | 3;
  blocks: BrandPageBlock[];
  appearance: BrandPageAppearance;
};
export type BrandPageEditorState = {
  draft: BrandPageConfig;
  published: BrandPageConfig | null;
  revision: number;
  published_revision: number | null;
  media_ids: number[];
};

export function defaultBrandPage(): BrandPageConfig {
  return {
    schema_version: 1,
    appearance: { accent_color: '', background_color: '', text_color: '' },
    blocks: [
      { id: 'hero', type: 'hero', layout: 'full-width', content: { source: 'brand_banner' } },
      { id: 'story', type: 'story', layout: 'split', content: { source: 'brand_story' } },
      { id: 'products', type: 'products', layout: 'grid', content: { source: 'brand_catalog' } },
      { id: 'contacts', type: 'contacts', layout: 'inline', content: { source: 'brand_contacts' } },
    ],
  };
}

const object = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === 'object' && !Array.isArray(v);

export const isBrandColor = (v: unknown): v is string =>
  typeof v === 'string' && /^(?:#(?:[0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8}))?$/i.test(v);

export const validBrandAppearance = (v: unknown): v is BrandPageAppearance =>
  object(v) &&
  isBrandColor(v.accent_color) &&
  isBrandColor(v.background_color) &&
  isBrandColor(v.text_color);

const positiveID = (id: unknown): id is number =>
  typeof id === 'number' && Number.isSafeInteger(id) && id > 0;
const cropNumber = (value: unknown) => typeof value === 'number' && value >= 0 && value <= 1;
const validCrop = (v: unknown) =>
  v === undefined || (object(v) && cropNumber(v.x) && cropNumber(v.y));
const plain = (v: unknown, limit: number) =>
  v === undefined || (typeof v === 'string' && [...v].length <= limit);
const optionalPreset = (v: unknown, options: readonly string[]) =>
  v === undefined || v === '' || (typeof v === 'string' && options.includes(v));

export function imageLimits(block: BrandPageBlock): [number, number] {
  switch (block.type) {
    case 'story':
      return [0, block.layout === 'text' ? 0 : 1];
    case 'image':
    case 'photo-text':
      return [1, 1];
    case 'horizontal-gallery':
    case 'vertical-gallery':
      return [2, 8];
    case 'editorial-grid':
      return ['diptych', 'unequal'].includes(block.layout) ? [2, 2] : [3, 3];
    default:
      return [0, 0];
  }
}

function validImage(v: unknown): v is BrandPageImage {
  return (
    object(v) &&
    positiveID(v.media_id) &&
    plain(v.alt, 160) &&
    optionalPreset(v.ratio, ['square', 'portrait', 'landscape', 'wide']) &&
    validCrop(v.crop) &&
    validCrop(v.mobile_crop)
  );
}

function validBlock(v: unknown, version: BrandPageConfig['schema_version']): v is BrandPageBlock {
  if (
    !object(v) ||
    typeof v.id !== 'string' ||
    !/^[a-z][a-z0-9-]{0,39}$/.test(v.id) ||
    typeof v.type !== 'string' ||
    !Object.hasOwn(brandPageBlocks, v.type) ||
    !object(v.content)
  )
    return false;
  const definition = brandPageBlocks[v.type as BlockType];
  const c = v.content;
  if (c.source !== definition.source || !definition.layouts.some((layout) => layout === v.layout))
    return false;
  if (definition.source === 'editorial' && version < 2) return false;
  if (
    version < 3 &&
    ((v.type === 'story' &&
      ((v.layout !== 'text' && v.layout !== 'split') || c.images !== undefined || c.align)) ||
      v.layout === 'text-first' ||
      v.layout === 'unequal' ||
      v.layout === 'staggered')
  )
    return false;
  if (v.composition !== undefined) {
    const p = v.composition;
    const text = v.type === 'story' || v.type === 'photo-text' || v.type === 'text';
    if (
      version !== 3 ||
      !object(p) ||
      !optionalPreset(p.spacing, compositionChoices.spacing) ||
      !optionalPreset(p.vertical, compositionChoices.vertical) ||
      !optionalPreset(p.text_width, compositionChoices.text_width) ||
      (p.text_width && !text) ||
      (p.vertical && (!text || v.type === 'text'))
    )
      return false;
  }
  if (
    !plain(c.title, 100) ||
    !plain(c.text, 1200) ||
    !optionalPreset(c.size, ['compact', 'medium', 'large', 'viewport']) ||
    !optionalPreset(c.align, ['start', 'center', 'end']) ||
    !optionalPreset(c.gap, ['tight', 'normal', 'loose']) ||
    !optionalPreset(c.columns, ['equal', 'wide-photo', 'wide-text'])
  )
    return false;
  const [min, max] = imageLimits(v as BrandPageBlock);
  if (v.type === 'text' && !c.title && !c.text) return false;
  if (c.images === undefined) return min === 0;
  return (
    Array.isArray(c.images) &&
    c.images.length >= min &&
    c.images.length <= max &&
    c.images.every(validImage)
  );
}

export function readBrandPageConfig(v: unknown): BrandPageConfig | undefined {
  if (
    !object(v) ||
    (v.schema_version !== 1 && v.schema_version !== 2 && v.schema_version !== 3) ||
    !validBrandAppearance(v.appearance) ||
    !Array.isArray(v.blocks)
  )
    return;
  const version = v.schema_version;
  const blocks: unknown[] = v.blocks;
  if (
    blocks.length < (version === 3 ? 2 : 3) ||
    blocks.length > (version === 1 ? 4 : 20) ||
    !blocks.every((block) => validBlock(block, version))
  )
    return;
  const system = blocks.filter((b) => b.content.source !== 'editorial').map((b) => b.type);
  if (
    system.length !== new Set(system).size ||
    !system.includes('hero') ||
    !system.includes('products') ||
    (version < 3 && !system.includes('story')) ||
    new Set(blocks.map((block) => block.id)).size !== blocks.length
  )
    return;
  return { schema_version: version, blocks, appearance: v.appearance };
}

export function readBrandPageEditor(v: unknown): BrandPageEditorState | undefined {
  if (
    !object(v) ||
    typeof v.revision !== 'number' ||
    !Number.isSafeInteger(v.revision) ||
    v.revision < 0
  )
    return;
  const draft = readBrandPageConfig(v.draft);
  if (!draft) return;
  const published = v.published === null ? null : readBrandPageConfig(v.published);
  if (published === undefined) return;
  const revision = v.published_revision;
  if (
    published === null
      ? revision !== null
      : typeof revision !== 'number' ||
        !Number.isSafeInteger(revision) ||
        revision < 1 ||
        revision > v.revision
  )
    return;
  return {
    revision: v.revision,
    draft,
    published,
    media_ids: Array.isArray(v.media_ids) ? v.media_ids.filter(positiveID) : [],
    published_revision: typeof revision === 'number' ? revision : null,
  };
}

type RGB = [number, number, number];
const paper: RGB = [246, 245, 241];
const ink: RGB = [23, 23, 22];
const white: RGB = [255, 255, 255];
const black: RGB = [0, 0, 0];

function composite(hex: string, surface: RGB, fallback: RGB): RGB {
  if (!hex || !isBrandColor(hex)) return fallback;
  const digits = hex.slice(1);
  const full = digits.length < 5 ? [...digits].map((digit) => digit.repeat(2)).join('') : digits;
  const opacity = full.length === 8 ? Number.parseInt(full.slice(6), 16) / 255 : 1;
  return [
    ...surface.map((channel, i) =>
      Math.round(
        Number.parseInt(full.slice(i * 2, i * 2 + 2), 16) * opacity + channel * (1 - opacity),
      ),
    ),
  ] as RGB;
}

function luminance(rgb: RGB) {
  const coefficients = [0.2126, 0.7152, 0.0722];
  return rgb.reduce((total, channel, i) => {
    const x = channel / 255;
    return total + coefficients[i] * (x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4);
  }, 0);
}

export function brandContrast(a: RGB, b: RGB) {
  const l = luminance(a),
    r = luminance(b);
  return (Math.max(l, r) + 0.05) / (Math.min(l, r) + 0.05);
}

export function brandPalette(appearance: BrandPageAppearance) {
  const background = composite(appearance.background_color, paper, paper);
  const requestedText = composite(appearance.text_color, background, ink);
  const text =
    brandContrast(requestedText, background) >= 4.5
      ? requestedText
      : brandContrast(black, background) >= brandContrast(white, background)
        ? black
        : white;
  const requestedAccent = composite(appearance.accent_color, background, text);
  const accent = brandContrast(requestedAccent, background) >= 4.5 ? requestedAccent : text;
  return {
    background,
    text,
    accent,
    adjusted: text !== requestedText || accent !== requestedAccent,
  };
}

// The server omits optional zero values and JSONB does not preserve key order.
export function pageFingerprint(config: BrandPageConfig) {
  function normalize(value: unknown): unknown {
    if (Array.isArray(value)) return value.map(normalize);
    if (!object(value)) return value;
    return Object.fromEntries(
      Object.keys(value)
        .sort()
        .flatMap((key) => {
          const child = normalize(value[key]);
          if (
            child === undefined ||
            child === '' ||
            (Array.isArray(child) && !child.length) ||
            (object(child) && !Object.keys(child).length)
          )
            return [];
          return [[key, child]];
        }),
    );
  }
  return JSON.stringify(normalize(config));
}
