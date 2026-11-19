import type { ReactNode } from 'react';
import type { BrandPageBlock } from '../../lib/brand-page';

export function compositionAttributes(block: BrandPageBlock, selectedID?: string) {
  return {
    'data-block-id': block.id,
    'data-brand-block': block.type,
    'data-layout': block.layout,
    'data-spacing': block.composition?.spacing,
    'data-vertical': block.composition?.vertical,
    'data-text-width': block.composition?.text_width,
    'data-align': block.content.align || undefined,
    'data-preview-selected': block.id === selectedID || undefined,
  };
}

export function CompositionPair({
  visual,
  copy,
  flow,
  columns,
}: {
  visual: ReactNode;
  copy: ReactNode;
  flow: 'visual-left' | 'visual-right' | 'visual-first' | 'text-first';
  columns?: 'equal' | 'wide-photo' | 'wide-text' | 'narrow-photo';
}) {
  const image = visual && <div className="composition-pair__visual">{visual}</div>;
  const text = copy && <div className="composition-pair__copy">{copy}</div>;
  const reverse = flow === 'visual-right' || flow === 'text-first';
  return (
    <div
      className="composition-pair"
      data-flow={flow}
      data-proportions={columns}
      data-single={!visual || !copy || undefined}
    >
      {reverse ? text : image}
      {reverse ? image : text}
    </div>
  );
}
