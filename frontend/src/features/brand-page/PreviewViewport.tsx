import { useEffect, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';

const frameHTML =
  '<!doctype html><html lang="ru"><head><meta charset="utf-8"></head><body><div id="preview-root"></div></body></html>';

export function PreviewViewport({
  device,
  focus,
  children,
}: {
  device: 'desktop' | 'mobile';
  focus?: { id: string };
  children: ReactNode;
}) {
  const [viewport, setViewport] = useState<Document>();
  const container = useRef<HTMLDivElement>(null);
  const [available, setAvailable] = useState(0);
  const width = device === 'desktop' ? 1440 : 390;
  const height = device === 'desktop' ? 900 : 844;
  const scale = available ? Math.min(1, available / width) : 1;
  useEffect(() => {
    const element = container.current;
    if (!element) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry.contentRect.width) setAvailable(entry.contentRect.width);
    });
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  useEffect(() => {
    if (!viewport || !focus) return;
    const section = viewport.querySelector<HTMLElement>(`[data-block-id="${focus.id}"]`);
    if (!section) return;
    section.tabIndex = -1;
    container.current?.scrollIntoView({ block: 'nearest' });
    section.scrollIntoView({ block: 'start' });
    section.focus({ preventScroll: true });
  }, [focus, viewport]);
  function initialize(frame: HTMLIFrameElement) {
    const doc = frame.contentDocument;
    if (!doc?.getElementById('preview-root')) return;
    for (const node of document.head.querySelectorAll('style, link[rel="stylesheet"]')) {
      doc.head.append(doc.importNode(node, true));
    }
    setViewport(doc);
  }
  return (
    <div
      ref={container}
      className={`brand-builder__frame brand-builder__frame--${device}`}
      style={{ maxWidth: width, height: height * scale }}
    >
      <iframe
        title="Предпросмотр витрины"
        srcDoc={frameHTML}
        sandbox="allow-same-origin"
        onLoad={(event) => initialize(event.currentTarget)}
        style={{ width, height, transform: `scale(${scale})` }}
      />
      {viewport &&
        createPortal(
          <div
            onClickCapture={(event) => {
              const element = event.target as Element;
              if (element.closest?.('a')) {
                event.preventDefault();
                event.stopPropagation();
              }
            }}
          >
            {children}
          </div>,
          viewport.getElementById('preview-root')!,
        )}
    </div>
  );
}
