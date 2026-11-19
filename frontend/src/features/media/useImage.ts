import { useEffect, useState, useSyncExternalStore } from 'react';
import { session } from '../../lib/client';
import { ApiError } from '../../lib/http';
import { mediaURL } from '../../lib/media';
// The backend deliberately bounds image delivery. All managed images share this
// queue, including public cards, to avoid a browser fan-out causing HTTP 429.
let active = 0;
const queue: Array<() => void> = [];
function schedule<T>(job: () => Promise<T>, signal: AbortSignal, priority: boolean): Promise<T> {
  return new Promise((resolve, reject) => {
    const run = () => {
      signal.removeEventListener('abort', cancel);
      active++;
      void job()
        .then(resolve, reject)
        .finally(() => {
          active--;
          queue.shift()?.();
        });
    };
    function cancel() {
      const i = queue.indexOf(run);
      if (i >= 0) queue.splice(i, 1);
      reject(new DOMException('Image load cancelled', 'AbortError'));
    }
    if (signal.aborted) {
      cancel();
      return;
    }
    if (active < 1) run();
    else {
      if (priority) queue.unshift(run);
      else queue.push(run);
      signal.addEventListener('abort', cancel, { once: true });
    }
  });
}
async function waitForRetry(delay: number, signal: AbortSignal) {
  signal.throwIfAborted();
  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => {
      signal.removeEventListener('abort', cancel);
      resolve();
    }, delay);
    function cancel() {
      clearTimeout(timeout);
      reject(signal.reason);
    }
    signal.addEventListener('abort', cancel, { once: true });
  });
}

async function retryImage<T>(attempt: () => Promise<T>, signal: AbortSignal): Promise<T> {
  for (let index = 0; ; index++) {
    try {
      return await attempt();
    } catch (error) {
      if (!(error instanceof ApiError) || ![429, 503].includes(error.status) || index >= 2)
        throw error;
      const delay = Math.max(error.retryAfterMs ?? 0, 250 * 2 ** index);
      if (delay > 5000) throw error;
      await waitForRetry(delay + Math.random() * 150, signal);
    }
  }
}
const cache = new Map<
  string,
  { refs: number; promise: Promise<Blob>; controller: AbortController }
>();
export function useImage(
  src: string | undefined,
  authenticated: boolean,
  visible = true,
  priority = false,
) {
  const auth = useSyncExternalStore(session.subscribe, session.snapshot, session.snapshot);
  const resolved = mediaURL(src);
  const managed = !!resolved?.startsWith('/media/assets/');
  const allowed = !authenticated || auth.status === 'authenticated';
  const key = managed ? `${authenticated ? auth.user?.id || 0 : 'public'}:${resolved}` : '';
  const [image, setImage] = useState<{ key: string; url?: string; failed?: boolean } | null>(null);
  useEffect(() => {
    if (!managed || !resolved || !allowed || !visible) return;
    let cancelled = false,
      objectURL: string | undefined;
    let entry = cache.get(key);
    if (!entry) {
      const controller = new AbortController();
      const request = () =>
        session.request<Blob>(resolved, {
          auth: authenticated,
          signal: controller.signal,
          responseType: 'blob',
        });
      entry = {
        refs: 0,
        controller,
        promise: retryImage(
          () =>
            schedule(
              () =>
                navigator.locks
                  ? navigator.locks.request(
                      'srez-managed-image',
                      { signal: controller.signal },
                      request,
                    )
                  : request(),
              controller.signal,
              priority,
            ),
          controller.signal,
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
      if (entry!.refs === 0 && cache.get(key) === entry) {
        const unused = entry;
        queueMicrotask(() => {
          if (unused.refs !== 0 || cache.get(key) !== unused) return;
          cache.delete(key);
          unused.controller.abort();
        });
      }
    };
  }, [key, managed, resolved, allowed, authenticated, visible, priority]);
  if (!managed) return { src: resolved, loading: false, failed: false };
  return {
    src: allowed && image?.key === key ? image.url : undefined,
    loading: allowed && image?.key !== key,
    failed: !allowed || (image?.key === key && !!image.failed),
  };
}
