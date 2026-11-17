import { createLogger, defineConfig, loadEnv } from 'vite';
import type { Logger } from 'vite';
import { redactWishlistLinks } from './scripts/redact-logs.ts';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'SREZ_');
  const baseLogger = createLogger();
  const customLogger: Logger = {
    ...baseLogger,
    info: (message, options) => baseLogger.info(redactWishlistLinks(message), options),
    warn: (message, options) => baseLogger.warn(redactWishlistLinks(message), options),
    warnOnce: (message, options) => baseLogger.warnOnce(redactWishlistLinks(message), options),
    error: (message, options) => baseLogger.error(redactWishlistLinks(message), options),
  };
  const port = Number(env.SREZ_FRONTEND_PORT || 5174);
  if (!Number.isInteger(port) || port < 1024 || port > 65535)
    throw new Error('Invalid local frontend port');
  const proxy = {
    '/api': {
      target: env.SREZ_API_TARGET || 'http://127.0.0.1:8081',
      changeOrigin: false,
      rewrite: (path: string) => path.replace(/^\/api(?=\/|$)/, ''),
    },
    '/media': {
      target: env.SREZ_API_TARGET || 'http://127.0.0.1:8081',
      changeOrigin: false,
    },
  };
  return {
    customLogger,
    plugins: [react()],
    server: {
      host: 'localhost',
      port,
      strictPort: true,
      cors: false,
      proxy,
      headers: { 'Referrer-Policy': 'no-referrer', 'Cache-Control': 'no-store' },
    },
    preview: { host: 'localhost', port, strictPort: true, cors: false, proxy },
    build: { sourcemap: false, assetsInlineLimit: 0 },
  };
});
