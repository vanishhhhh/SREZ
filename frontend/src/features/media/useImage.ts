import { useEffect, useState, useSyncExternalStore } from 'react';
import { session } from '../../lib/client';
import { mediaURL } from '../../lib/media';
// The backend deliberately bounds image delivery. All managed images share this
// queue, including public cards, to avoid a browser fan-out causing HTTP 429.
let active = 0;
const queue: Array<() => void> = [];
function schedule<T>(job: () => Promise<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    const run = () => {
      active++;
      void job()
        .then(resolve, reject)
        .finally(() => {
          active--;
          queue.shift()?.();
        });
    };
    if (active < 1) run();
    else queue.push(run);
  });
}
const cache = new Map<string, { refs: number; promise: Promise<Blob> }>();
export function useImage(src: string | undefined, authenticated: boolean) {
  const auth = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  const resolved = mediaURL(src);
  const managed = !!resolved?.startsWith('/media/assets/');
  const allowed = !authenticated || auth.status === 'authenticated';
  const key = managed ? `${authenticated ? auth.user?.id || 0 : 'public'}:${resolved}` : '';
  const [image, setImage] = useState<{ key: string; url?: string; failed?: boolean } | null>(null);
  useEffect(() => {
    if (!managed || !resolved || !allowed) return;
    let cancelled = false,
      objectURL: string | undefined;
    let entry = cache.get(key);
    if (!entry) {
      entry = {
        refs: 0,
        promise: schedule(() =>
          session.request<Blob>(resolved, { auth: authenticated, responseType: 'blob' }),
        ),
      };
      cache.set(key, entry);
    }
    entry.refs++;
    void entry.promise
      .then((blob) => {
        if (!cancelled) {
          objectURL = URL.createObjectURL(blob);
          setImage({ key, url: objectURL });
        }
      })
      .catch(() => {
        if (!cancelled) setImage({ key, failed: true });
      });
    return () => {
      cancelled = true;
      if (objectURL) URL.revokeObjectURL(objectURL);
      entry!.refs--;
      if (entry!.refs === 0 && cache.get(key) === entry) cache.delete(key);
    };
  }, [key, managed, resolved, allowed, authenticated]);
  if (!managed) return { src: resolved, loading: false, failed: false };
  return {
    src: allowed && image?.key === key ? image.url : undefined,
    loading: allowed && image?.key !== key,
    failed: !allowed || (image?.key === key && !!image.failed),
  };
}
