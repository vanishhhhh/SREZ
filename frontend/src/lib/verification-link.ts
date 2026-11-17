// Called before React/router initialization. The secret never becomes a query
// request, router state, persistent storage, cookie, or analytics value.
let captured = '';
let resetCaptured = '';
export function captureVerificationLink() {
  if (!['/verify-email', '/reset-password'].includes(window.location.pathname)) return;
  const url = new URL(window.location.href);
  const raw =
    new URLSearchParams(url.hash.slice(1)).get('token') || url.searchParams.get('token') || '';
  const token = /^[A-Za-z0-9_-]{43}$/.test(raw) ? raw : '';
  if (window.location.pathname === '/reset-password') resetCaptured = token;
  else captured = token;
  url.searchParams.delete('token');
  window.history.replaceState(window.history.state, '', url.pathname + url.search);
}
export const verificationLinkToken = () => captured;
export const forgetVerificationLink = () => {
  captured = '';
};
export const resetLinkToken = () => resetCaptured;
export const forgetResetLink = () => {
  resetCaptured = '';
};

// This dependency evaluates before App creates the browser router.
if (typeof window !== 'undefined') captureVerificationLink();
