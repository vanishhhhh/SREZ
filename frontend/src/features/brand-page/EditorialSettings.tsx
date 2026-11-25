import { CompositionControls, LayoutCards } from './CompositionSettings';
import { Button, Field, Media } from '../../components/ui';
import { brandPageBlocks, imageLimits } from '../../lib/brand-page';
import type { BrandPageBlock, BrandPageCrop, BrandPageImage } from '../../lib/brand-page';
import { UploadField } from '../media/UploadField';

const labels: Record<string, string> = {
  'full-width': 'На всю ширину',
  contained: 'В пределах страницы',
  portrait: 'Портрет',
  landscape: 'Пейзаж',
  'photo-left': 'Фото слева',
  'photo-right': 'Фото справа',
  stacked: 'Друг под другом',
  feature: 'Крупное фото и два малых',
  diptych: 'Два фото рядом',
  'wide-top': 'Широкое фото сверху',
  asymmetric: 'Асимметрия',
  compact: 'Компактный',
  medium: 'Средний',
  large: 'Большой',
  viewport: 'По высоте экрана',
  start: 'По левому краю',
  center: 'По центру',
  end: 'По правому краю',
  equal: 'Равные',
  'wide-photo': 'Фото шире',
  'wide-text': 'Текст шире',
  tight: 'Узкий',
  normal: 'Обычный',
  loose: 'Широкий',
  wide: '16:9',
  square: '1:1',
};
const sizes = ['compact', 'medium', 'large', 'viewport'];
const alignment = ['start', 'center', 'end'];

export function EditorialSelect({
  label,
  value,
  choices,
  change,
  optional = true,
}: {
  label: string;
  value: string;
  choices: readonly string[];
  change: (value: string) => void;
  optional?: boolean;
}) {
  return (
    <Field label={label}>
      <select value={value} onChange={(e) => change(e.target.value)}>
        {optional && <option value="">По умолчанию</option>}
        {choices.map((choice) => (
          <option key={choice} value={choice}>
            {labels[choice] || choice}
          </option>
        ))}
      </select>
    </Field>
  );
}

export const HeroSize = ({
  block,
  change,
}: {
  block: BrandPageBlock;
  change: (value: string) => void;
}) => (
  <EditorialSelect
    label="Высота обложки"
    value={block.content.size || ''}
    choices={sizes}
    change={change}
  />
);

type Props = {
  block: BrandPageBlock;
  brandID: number;
  blockLabel: string;
  library: number[];
  onUploadStart: () => (() => void) | null;
  saving: boolean;
  onUpload: (id: number, index: number, previous?: number) => void;
  change: (block: BrandPageBlock, compositionVersion?: 3) => void;
};

export function EditorialSettings({
  block,
  brandID,
  blockLabel,
  library,
  onUpload,
  onUploadStart,
  saving,
  change,
}: Props) {
  const c = block.content;
  const images = c.images ?? [];
  const [min, max] = imageLimits(block);
  const gallery = max > 1;
  const single = block.type === 'image';
  const story = block.type === 'photo-text';
  const brandStory = block.type === 'story';
  const horizontal = block.type === 'horizontal-gallery';
  const text = block.type === 'text';
  const patch = (fields: Partial<BrandPageBlock['content']>) =>
    change({ ...block, content: { ...c, ...fields } }, brandStory ? 3 : undefined);
  function setImage(index: number, image: BrandPageImage) {
    const next = [...images];
    next[index] = image;
    patch({ images: next });
  }
  function moveImage(index: number, direction: -1 | 1) {
    const next = [...images];
    const to = index + direction;
    if (to < 0 || to >= next.length) return;
    [next[index], next[to]] = [next[to], next[index]];
    patch({ images: next });
  }
  const layoutOptions = brandPageBlocks[block.type].layouts;
  return (
    <div className="brand-editorial-settings">
      {brandStory || story || block.type === 'editorial-grid' ? (
        <LayoutCards
          block={block}
          change={(layout) =>
            change(
              {
                ...block,
                layout,
                content: brandStory && layout === 'text' ? { ...c, images: undefined } : c,
              },
              3,
            )
          }
        />
      ) : (
        layoutOptions.length > 1 && (
          <EditorialSelect
            label="Композиция"
            value={block.layout}
            choices={layoutOptions}
            optional={false}
            change={(layout) => change({ ...block, layout })}
          />
        )
      )}
      {!single && !brandStory && (
        <Field label="Заголовок">
          <input
            maxLength={100}
            value={c.title || ''}
            onChange={(e) => patch({ title: e.target.value })}
          />
        </Field>
      )}
      {(single || story || text) && (
        <Field label={single ? 'Подпись фотографии' : 'Текст'}>
          <textarea
            rows={4}
            maxLength={1200}
            value={c.text || ''}
            onChange={(e) => patch({ text: e.target.value })}
          />
        </Field>
      )}
      {(single || horizontal) && (
        <EditorialSelect
          label="Размер"
          value={c.size || ''}
          choices={horizontal ? sizes.slice(0, 3) : sizes}
          change={(size) => patch({ size })}
        />
      )}
      {(single || story || text || brandStory) && (
        <EditorialSelect
          label="Выравнивание"
          value={c.align || ''}
          choices={alignment}
          change={(align) => patch({ align })}
        />
      )}
      {story && (block.layout === 'photo-left' || block.layout === 'photo-right') && (
        <EditorialSelect
          label="Ширина колонок"
          value={c.columns || ''}
          choices={['equal', 'wide-photo', 'wide-text']}
          change={(columns) => patch({ columns })}
        />
      )}
      {gallery && (
        <EditorialSelect
          label="Интервал"
          value={c.gap || ''}
          choices={['tight', 'normal', 'loose']}
          change={(gap) => patch({ gap })}
        />
      )}
      <CompositionControls block={block} change={(next) => change(next, 3)} />
      {brandStory && max > 0 && (
        <p className="field__hint">Без выбранного фото отображается только текст истории.</p>
      )}
      {images.length > max && (
        <p className="field__hint">Уберите лишние фото для этой композиции.</p>
      )}
      {max > 0 && images.length < min && (
        <p className="field__hint">
          Добавьте {min === 1 ? 'фотографию' : `минимум ${min} фотографии`}.
        </p>
      )}
      {Array.from(
        { length: Math.max(images.length, Math.min(images.length + 1, max)) },
        (_, index) => {
          const image = images[index];
          const select = (id: number) => {
            if (id > 0) setImage(index, { ...image, media_id: id });
          };
          return (
            <fieldset key={index} className="editorial-image-settings">
              <legend>Фото {index + 1}</legend>
              {image && (
                <Media
                  src={'/media/assets/' + image.media_id + '/content'}
                  alt={image.alt || ''}
                  authenticated
                />
              )}
              {library.length > 0 && (
                <Field label={`Фото ${index + 1} из библиотеки`}>
                  <select
                    aria-label={`Фото ${index + 1} из библиотеки, ${blockLabel}`}
                    value={image?.media_id ?? ''}
                    onChange={(e) => select(Number(e.target.value))}
                  >
                    <option value="" disabled>
                      Выбрать фотографию
                    </option>
                    {[...library].reverse().map((id, i) => (
                      <option key={id} value={id}>
                        Фото из библиотеки №{i + 1}
                      </option>
                    ))}
                  </select>
                </Field>
              )}
              <UploadField
                onStart={onUploadStart}
                disabled={saving}
                label={`Загрузить фото ${index + 1}, ${blockLabel}`}
                target={{ brand_id: brandID, purpose: 'brand_editorial_image' }}
                onComplete={(id) => onUpload(id, index, image?.media_id)}
              />
              {image && (
                <>
                  <Field
                    label="Альтернативное описание"
                    hint="Для скринридера. Оставьте пустым, если фото декоративное."
                  >
                    <input
                      value={image.alt || ''}
                      maxLength={160}
                      onChange={(e) => setImage(index, { ...image, alt: e.target.value })}
                    />
                  </Field>
                  <EditorialSelect
                    label="Пропорции"
                    value={image.ratio || ''}
                    choices={['portrait', 'landscape', 'square', 'wide']}
                    change={(ratio) => setImage(index, { ...image, ratio })}
                  />
                  <CropFields
                    label="Кадрирование"
                    crop={image.crop}
                    change={(crop) => setImage(index, { ...image, crop })}
                  />
                  <label>
                    <input
                      type="checkbox"
                      checked={!!image.mobile_crop}
                      onChange={(e) =>
                        setImage(index, {
                          ...image,
                          mobile_crop: e.target.checked
                            ? (image.crop ?? { x: 0.5, y: 0.5 })
                            : undefined,
                        })
                      }
                    />{' '}
                    Отдельное кадрирование для телефона
                  </label>
                  {image.mobile_crop && (
                    <CropFields
                      label="Кадрирование телефона"
                      crop={image.mobile_crop}
                      change={(mobile_crop) => setImage(index, { ...image, mobile_crop })}
                    />
                  )}
                  <div className="button-row">
                    {gallery && (
                      <>
                        <Button
                          variant="outline"
                          aria-label={`Переместить фото ${index + 1} назад`}
                          disabled={index === 0}
                          onClick={() => moveImage(index, -1)}
                        >
                          Назад
                        </Button>
                        <Button
                          variant="outline"
                          aria-label={`Переместить фото ${index + 1} вперёд`}
                          disabled={index === images.length - 1}
                          onClick={() => moveImage(index, 1)}
                        >
                          Вперёд
                        </Button>
                      </>
                    )}
                    <Button
                      variant="ghost"
                      onClick={() => patch({ images: images.filter((_, at) => at !== index) })}
                    >
                      {`Убрать фото ${index + 1}`}
                    </Button>
                  </div>
                </>
              )}
            </fieldset>
          );
        },
      )}
    </div>
  );
}

function CropFields({
  label,
  crop,
  change,
}: {
  label: string;
  crop?: BrandPageCrop;
  change: (value: BrandPageCrop) => void;
}) {
  const point = crop ?? { x: 0.5, y: 0.5 };
  return (
    <fieldset className="editorial-image-settings">
      <legend>{label}</legend>
      {(['x', 'y'] as const).map((axis) => (
        <Field key={axis} label={axis === 'x' ? 'По горизонтали' : 'По вертикали'}>
          <input
            type="range"
            min={0}
            max={100}
            value={Math.round(point[axis] * 100)}
            onChange={(e) => change({ ...point, [axis]: Number(e.target.value) / 100 })}
          />
        </Field>
      ))}
    </fieldset>
  );
}
