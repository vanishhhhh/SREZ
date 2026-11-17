// Managed files are served by Go through /media. Never construct storage URLs.
export function mediaURL(value?: string): string | undefined {
  if (!value || value !== value.trim() || /\s/.test(value)) return undefined;
  if (/^\/media\/assets\/[a-zA-Z0-9_-]+\/content$/.test(value)) return value;
  try {
    const url = new URL(value);
    if (url.protocol !== 'https:' || url.username || url.password) return undefined;
    // Signed S3 credentials belong to the backend, never to public image elements.
    if (
      [...url.searchParams.keys()].some((key) => /^(x-amz-|awsaccesskeyid$|signature$)/i.test(key))
    )
      return undefined;
    return url.href;
  } catch {
    return undefined;
  }
}
