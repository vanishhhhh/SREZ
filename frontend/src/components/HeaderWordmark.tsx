import { useState } from 'react';
import { Link } from 'react-router-dom';
import { useImage } from '../features/media/useImage';
import type { Brand } from '../lib/types';

export function HeaderWordmark({
  brand,
  authenticated = false,
}: {
  brand?: Brand;
  authenticated?: boolean;
}) {
  return brand ? (
    <BrandWordmark
      key={`${brand.id}:${brand.logo_url}`}
      brand={brand}
      authenticated={authenticated}
    />
  ) : (
    <Link to="/" className="wordmark" aria-label="SREZ — главная">
      SREZ<span>®</span>
    </Link>
  );
}

function BrandWordmark({ brand, authenticated }: { brand: Brand; authenticated: boolean }) {
  const image = useImage(brand.logo_url, authenticated, true, true);
  const [failed, setFailed] = useState(false);
  return (
    <Link to="/" className="brand-wordmark" aria-label={`${brand.name} — главная`}>
      {image.src && !image.failed && !failed ? (
        <img src={image.src} alt={brand.name} onError={() => setFailed(true)} />
      ) : (
        <span>{brand.name}</span>
      )}
    </Link>
  );
}
