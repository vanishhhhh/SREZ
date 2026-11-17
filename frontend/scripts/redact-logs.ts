// Capability URLs grant read access. Never include them in proxy error logs.
export function redactWishlistLinks(message: string): string {
  return message
    .replace(/((?:\/api)?\/wishlists\/shared\/|\/w\/)[A-Za-z0-9_-]+/g, '$1[redacted]')
    .replace(/([#?&]token=)[A-Za-z0-9_%+-]+/g, '$1[redacted]')
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b/g, '[redacted-jwt]')
    .replace(/(?<![A-Za-z0-9_-])[A-Za-z0-9_-]{43}(?![A-Za-z0-9_-])/g, '[redacted-secret]');
}
