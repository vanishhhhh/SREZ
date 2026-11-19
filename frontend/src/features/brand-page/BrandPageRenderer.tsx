import { HeroMedia } from './ResponsiveMedia';
import type { CSSProperties } from 'react';
import { Link } from 'react-router-dom';
import { Icon, Media } from '../../components/ui';
import { api } from '../../lib/api';
import { useLoad } from '../../lib/hooks';
import { mediaURL } from '../../lib/media';
import { brandPalette } from '../../lib/brand-page';
import type { BrandPageBlock, BrandPageConfig } from '../../lib/brand-page';
import type { Brand } from '../../lib/types';
import { Pagination, ProductCollection } from '../storefront/shared';
import { CompositionPair, compositionAttributes } from './Composition';
import { EditorialBlock, EditorialPhoto } from './EditorialBlocks';
import './brand-page.css';
import './editorial.css';
import './composition.css';

export function useBrandPageProducts(brandID: number, page: string) {
  return useLoad(
    (signal) =>
      api.products(
        new URLSearchParams({ brand_id: String(brandID), limit: '12', sort: 'newest', page }),
        signal,
      ),
    [brandID, page],
  );
}

export function BrandPageRenderer({
  brand,
  config,
  products,
  onPage,
  preview,
  selectedID,
}: {
  brand: Brand;
  config: BrandPageConfig;
  products: ReturnType<typeof useBrandPageProducts>;
  onPage: (page: number) => void;
  preview?: 'desktop' | 'mobile';
  selectedID?: string;
}) {
  const palette = brandPalette(config.appearance);
  const style = {
    '--brand-background': `rgb(${palette.background.join(' ')})`,
    '--brand-text': `rgb(${palette.text.join(' ')})`,
    '--brand-accent': `rgb(${palette.accent.join(' ')})`,
  } as CSSProperties;
  return (
    <div className="brand-page-content" style={style}>
      {config.blocks.map((block) => {
        const attributes = compositionAttributes(block, preview ? selectedID : undefined);
        switch (block.type) {
          case 'hero': {
            const banner = mediaURL(brand.banner_url);
            return (
              <section
                key={block.id}
                {...attributes}
                data-size={block.content.size || undefined}
                className={`brand-hero brand-hero--${block.layout}${banner ? ' brand-hero--image' : ' brand-hero--empty'}`}
              >
                {banner && (
                  <HeroMedia
                    key={banner}
                    className="brand-hero__banner"
                    src={banner}
                    alt=""
                    split={block.layout === 'split'}
                    authenticated={!!preview}
                  />
                )}
                <div className="container">
                  <Link className="text-link" to="/brands">
                    <Icon name="back" size={16} /> Все бренды
                  </Link>
                  <h1>{brand.name}</h1>
                  {preview && !banner && <p>Добавьте баннер в настройках бренда.</p>}
                </div>
              </section>
            );
          }
          case 'story':
            return (
              <BrandStory
                key={block.id}
                brand={brand}
                block={block}
                preview={!!preview}
                attributes={attributes}
              />
            );
          case 'products':
            return (
              <section
                key={block.id}
                className="section container brand-page-products"
                id={preview ? undefined : 'brand-products'}
                {...attributes}
              >
                <div className="section-heading">
                  <h2>Вещи бренда</h2>
                  <Link className="text-link" to={'/catalog?brand_id=' + brand.id}>
                    Фильтры каталога <Icon name="arrow" />
                  </Link>
                </div>
                <ProductCollection {...products} preview={!!preview} />
                {!products.loading && products.data && (
                  <Pagination data={products.data} onPage={onPage} />
                )}
                {products.errorStatus === 400 && !preview && (
                  <Link className="text-link" to={'/brands/' + brand.slug}>
                    К первой странице <Icon name="arrow" />
                  </Link>
                )}
              </section>
            );
          case 'contacts':
            return (
              <BrandContacts
                key={block.id}
                brand={brand}
                preview={!!preview}
                attributes={attributes}
              />
            );
          default:
            return (
              <EditorialBlock
                key={block.id}
                block={block}
                preview={!!preview}
                attributes={attributes}
              />
            );
        }
      })}
    </div>
  );
}

type CompositionAttributes = ReturnType<typeof compositionAttributes>;

function BrandStory({
  brand,
  block,
  preview,
  attributes,
}: {
  brand: Brand;
  block: BrandPageBlock;
  preview: boolean;
  attributes: CompositionAttributes;
}) {
  const photo = block.content.images?.[0];
  const logo = mediaURL(brand.logo_url);
  const visual =
    block.layout !== 'text' &&
    (photo ? (
      <EditorialPhoto image={photo} preview={preview} />
    ) : logo ? (
      <Media
        className="brand-story-logo"
        src={logo}
        alt={'Логотип ' + brand.name}
        authenticated={preview}
      />
    ) : null);
  const description = brand.description.trim();
  if (!description && !preview) return null;
  const reversed = block.layout.endsWith('-reversed');
  const split = block.layout.startsWith('split') || block.layout.startsWith('editorial');
  return (
    <section className="brand-page-editorial brand-story-section" {...attributes}>
      <div className="container">
        <CompositionPair
          visual={description ? visual : null}
          copy={
            <>
              <h2>О бренде</h2>
              <p>{description || 'Добавьте описание в настройках бренда.'}</p>
              {preview && description && !visual && block.layout !== 'text' && (
                <p className="brand-story-section__hint">
                  Добавьте логотип или выберите фотографию для истории.
                </p>
              )}
            </>
          }
          flow={split ? (reversed ? 'visual-right' : 'visual-left') : 'visual-first'}
          columns={block.layout.startsWith('editorial') ? 'narrow-photo' : 'equal'}
        />
      </div>
    </section>
  );
}

function BrandContacts({
  brand,
  preview,
  attributes,
}: {
  brand: Brand;
  preview: boolean;
  attributes: CompositionAttributes;
}) {
  const website = mediaURL(brand.contacts.website_url);
  const telegram = mediaURL(brand.contacts.telegram_url);
  const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(brand.contacts.email) ? brand.contacts.email : '';
  if (!website && !telegram && !email) {
    return preview && attributes['data-preview-selected'] ? (
      <section className="container composition-empty" {...attributes}>
        Добавьте контакты в настройках бренда.
      </section>
    ) : null;
  }
  return (
    <section className="brand-page-editorial" {...attributes}>
      <div className="container brand-page-contacts">
        <h2>Контакты</h2>
        <div className="brand-contacts">
          {website && (
            <a href={website} target="_blank" rel="noopener noreferrer">
              Сайт бренда <Icon name="diagonal" size={15} />
            </a>
          )}
          {telegram && (
            <a href={telegram} target="_blank" rel="noopener noreferrer">
              Telegram <Icon name="diagonal" size={15} />
            </a>
          )}
          {email && (
            <a href={'mailto:' + email}>
              Написать бренду <Icon name="diagonal" size={15} />
            </a>
          )}
        </div>
      </div>
    </section>
  );
}
