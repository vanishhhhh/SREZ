import { useLayoutEffect } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { useLocation, useOutletContext } from 'react-router-dom';
import type { Brand } from '../../lib/types';

export type BrandHeader = { brand: Brand; pathname: string };
type HeaderContext = { setBrandHeader: Dispatch<SetStateAction<BrandHeader | null>> };

export function useBrandHeader(brand: Brand) {
  const { pathname } = useLocation();
  const { setBrandHeader } = useOutletContext<HeaderContext>();
  useLayoutEffect(() => {
    const registration = { brand, pathname };
    setBrandHeader(registration);
    return () => setBrandHeader((current) => (current === registration ? null : current));
  }, [brand, pathname, setBrandHeader]);
}
