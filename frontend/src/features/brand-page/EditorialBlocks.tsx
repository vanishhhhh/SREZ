import { EditorialMedia } from './ResponsiveMedia';
import { useRef } from 'react';
import type { CSSProperties } from 'react';
import { CompositionPair } from './Composition';
import type { compositionAttributes } from './Composition';
import { Button } from '../../components/ui';
import type { BrandPageBlock, BrandPageImage } from '../../lib/brand-page';

export function EditorialPhoto({ image, preview }: { image: BrandPageImage; preview: boolean }) {
  const x = image.crop?.x ?? 0.5;
  const y = image.crop?.y ?? 0.5;
  const style = {
    '--photo-x': x * 100 + '%',
    '--photo-y': y * 100 + '%',
    '--mobile-x': (image.mobile_crop?.x ?? x) * 100 + '%',
    '--mobile-y': (image.mobile_crop?.y ?? y) * 100 + '%',
  } as CSSProperties;
  return (
    <div className="editorial-photo" data-ratio={image.ratio || undefined} style={style}>
      <EditorialMedia
        src={'/media/assets/' + image.media_id + '/content'}
        alt={image.alt || ''}
        authenticated={preview}
      />
    </div>
  );
}

export function EditorialBlock({
  block,
  preview,
  attributes: shared,
}: {
  block: BrandPageBlock;
  preview: boolean;
  attributes: ReturnType<typeof compositionAttributes>;
}) {
  const content = block.content;
  const images = content.images ?? [];
  const photos = images.map((image, index) => (
    <EditorialPhoto key={index} image={image} preview={preview} />
  ));
  const rail = useRef<HTMLDivElement>(null);
  const title = content.title ? <h2>{content.title}</h2> : null;
  const body = content.text ? <p>{content.text}</p> : null;
  const attributes = {
    ...shared,
    'data-size': content.size || undefined,
    'data-gap': content.gap || undefined,
    'data-align': content.align || undefined,
    'data-columns': content.columns || undefined,
  };
  function scrollGallery(direction: number) {
    const node = rail.current;
    if (node) node.scrollBy({ left: direction * node.clientWidth * 0.85 });
  }
  if (preview && !photos.length && block.type !== 'text') {
    return (
      <section className="container composition-empty" {...attributes}>
        Выберите изображения для блока.
      </section>
    );
  }
  switch (block.type) {
    case 'image':
      return (
        <figure className="brand-editorial brand-editorial--image" {...attributes}>
          {photos}
          {content.text && <figcaption>{content.text}</figcaption>}
        </figure>
      );
    case 'photo-text':
      return (
        <section className="container brand-editorial" {...attributes}>
          <CompositionPair
            visual={photos}
            copy={
              title || body ? (
                <>
                  {title}
                  {body}
                </>
              ) : null
            }
            flow={
              block.layout === 'photo-right'
                ? 'visual-right'
                : block.layout === 'photo-left'
                  ? 'visual-left'
                  : block.layout === 'text-first'
                    ? 'text-first'
                    : 'visual-first'
            }
            columns={
              content.columns === 'wide-photo' || content.columns === 'wide-text'
                ? content.columns
                : 'equal'
            }
          />
        </section>
      );
    case 'horizontal-gallery':
      return (
        <section className="container brand-editorial brand-editorial--carousel" {...attributes}>
          <div className="brand-editorial__heading">
            {title}
            <div className="button-row" role="group" aria-label="Управление галереей">
              <Button
                variant="outline"
                aria-label="Предыдущие фотографии"
                onClick={() => scrollGallery(-1)}
              >
                ←
              </Button>
              <Button
                variant="outline"
                aria-label="Следующие фотографии"
                onClick={() => scrollGallery(1)}
              >
                →
              </Button>
            </div>
          </div>
          <div
            className="brand-editorial__photos"
            ref={rail}
            tabIndex={0}
            role="region"
            aria-label={content.title || 'Горизонтальная галерея'}
          >
            {photos}
          </div>
        </section>
      );
    case 'vertical-gallery':
    case 'editorial-grid':
      return (
        <section className="container brand-editorial" {...attributes}>
          {title}
          <div className="brand-editorial__photos">{photos}</div>
        </section>
      );
    case 'text':
      return (
        <section className="container brand-editorial brand-editorial--text" {...attributes}>
          <div className="brand-editorial__copy">
            {title}
            {body}
          </div>
        </section>
      );
  }
  return null;
}
