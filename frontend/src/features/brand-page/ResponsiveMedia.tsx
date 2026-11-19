import { useEffect, useRef, useState } from 'react';
import { Media } from '../../components/ui';
import { mediaURL } from '../../lib/media';

const widths = [480, 960, 1440] as const;
const variant = (src: string, width: number) => `${src}?width=${width}`;

type Props = { src: string; alt: string; className?: string; authenticated: boolean };

export function EditorialMedia(props: Props) {
  const root = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState<number>();
  useEffect(() => {
    const node = root.current;
    if (!node) return;
    const read = () => {
      const cssWidth = node.clientWidth;
      if (!cssWidth) return;
      const requested = cssWidth * (node.ownerDocument.defaultView?.devicePixelRatio ?? 1);
      setWidth(widths.find((size) => size >= requested) ?? widths[2]);
    };
    read();
    const observer = new ResizeObserver(read);
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  return (
    <div ref={root} className="editorial-image-source">
      {width && <Media {...props} src={variant(props.src, width)} />}
    </div>
  );
}

// Only an already published surface uses native loading. The endpoint still
// authorizes anonymously on each request. A failed native request can use the
// managed pipeline's bounded, Retry-After-aware recovery.
export function HeroMedia({
  src,
  alt,
  className,
  authenticated,
  split,
}: Props & { split: boolean }) {
  const [failed, setFailed] = useState<string>();
  if (authenticated || !src.startsWith('/media/assets/') || failed) {
    return (
      <Media
        src={failed || (src.startsWith('/media/assets/') ? variant(src, 960) : src)}
        alt={alt}
        className={className}
        eager
        authenticated={authenticated}
      />
    );
  }
  return (
    <div className={`media ${className ?? ''}`} data-media-src={src}>
      <img
        src={'/api' + variant(src, 960)}
        srcSet={widths.map((width) => '/api' + variant(src, width) + ' ' + width + 'w').join(', ')}
        sizes={split ? '(max-width: 550px) 100vw, 50vw' : '100vw'}
        alt={alt}
        fetchPriority="high"
        loading="eager"
        referrerPolicy="no-referrer"
        onError={(e) => {
          const url = new URL(e.currentTarget.currentSrc, window.location.href);
          setFailed(mediaURL(url.pathname.slice(4) + url.search) || variant(src, 960));
        }}
      />
    </div>
  );
}
