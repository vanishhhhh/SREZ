import { useId } from 'react';
import { Field } from '../../components/ui';
import { brandPageBlocks } from '../../lib/brand-page';
import type { BrandComposition, BrandPageBlock } from '../../lib/brand-page';

const diagrams: Record<string, [string, string, string]> = {
  text: ['Только текст', 'one', 't'],
  centered: ['По центру', 'centered', 'it'],
  split: ['Изображение слева', 'pair', 'it'],
  'split-reversed': ['Изображение справа', 'pair', 'ti'],
  editorial: ['Узкое изображение слева', 'narrow-left', 'it'],
  'editorial-reversed': ['Узкое изображение справа', 'narrow-right', 'ti'],
  stacked: ['Изображение сверху', 'vertical', 'it'],
  'photo-left': ['Фото слева', 'pair', 'it'],
  'photo-right': ['Фото справа', 'pair', 'ti'],
  'text-first': ['Текст сверху', 'vertical', 'ti'],
  feature: ['Крупное и два малых', 'feature', 'iii'],
  diptych: ['Два равных фото', 'pair', 'ii'],
  unequal: ['Два фото 60/40', 'narrow-right', 'ii'],
  'wide-top': ['Широкое фото сверху', 'wide-top', 'iii'],
  asymmetric: ['Асимметрия', 'asymmetric', 'iii'],
  staggered: ['Три высоты', 'staggered', 'iii'],
};

export function LayoutCards({
  block,
  change,
}: {
  block: BrandPageBlock;
  change: (layout: string) => void;
}) {
  const name = useId();
  return (
    <fieldset className="composition-options">
      <legend>Композиция блока «{brandPageBlocks[block.type].label}»</legend>
      <div className="composition-options__cards">
        {brandPageBlocks[block.type].layouts.map((layout) => {
          const [label, diagram, marks] = diagrams[layout];
          return (
            <label key={layout}>
              <input
                type="radio"
                name={name}
                value={layout}
                checked={block.layout === layout}
                onChange={() => change(layout)}
              />
              <span className="composition-diagram" data-diagram={diagram} aria-hidden="true">
                {[...marks].map((mark, index) => (
                  <i
                    key={index}
                    className={mark === 't' ? 'composition-diagram__text' : undefined}
                  />
                ))}
              </span>
              <span>{label}</span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

const spacing: Record<NonNullable<BrandComposition['spacing']>, string> = {
  compact: 'Компактные',
  default: 'Стандартные',
  spacious: 'Просторные',
};
const widths: Record<NonNullable<BrandComposition['text_width']>, string> = {
  narrow: 'Узкая',
  regular: 'Средняя',
  wide: 'Широкая',
};
const vertical: Record<NonNullable<BrandComposition['vertical']>, string> = {
  top: 'Сверху',
  center: 'По центру',
  bottom: 'Снизу',
};

function Preset<K extends string>({
  label,
  options,
  value,
  change,
}: {
  label: string;
  options: Record<K, string>;
  value?: K;
  change: (value: K | undefined) => void;
}) {
  return (
    <Field label={label}>
      <select
        value={value || ''}
        onChange={(e) => change((e.target.value || undefined) as K | undefined)}
      >
        <option value="">По умолчанию</option>
        {Object.keys(options).map((key) => (
          <option key={key} value={key}>
            {options[key as K]}
          </option>
        ))}
      </select>
    </Field>
  );
}

export function CompositionControls({
  block,
  change,
}: {
  block: BrandPageBlock;
  change: (block: BrandPageBlock) => void;
}) {
  const text = ['story', 'photo-text', 'text'].includes(block.type);
  const split = [
    'split',
    'split-reversed',
    'editorial',
    'editorial-reversed',
    'photo-left',
    'photo-right',
  ].includes(block.layout);
  const patch = (next: BrandComposition) =>
    change({ ...block, composition: { ...block.composition, ...next } });
  return (
    <>
      {split && (
        <Preset
          label="По высоте"
          options={vertical}
          value={block.composition?.vertical}
          change={(value) => patch({ vertical: value })}
        />
      )}
      {text && (
        <Preset
          label="Ширина текста"
          options={widths}
          value={block.composition?.text_width}
          change={(value) => patch({ text_width: value })}
        />
      )}
      <Preset
        label="Отступы секции"
        options={spacing}
        value={block.composition?.spacing}
        change={(value) => patch({ spacing: value })}
      />
    </>
  );
}
