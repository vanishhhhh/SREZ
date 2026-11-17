import { defineConfig } from '@playwright/test';
import base, { assertIsolatedLiveTarget } from './playwright.config';
const baseURL = process.env.SREZ_PUBLIC_E2E_URL || 'http://localhost:5175';
assertIsolatedLiveTarget(baseURL);
export default defineConfig(base, {
  testMatch: 'public.spec.ts',
  use: { baseURL },
});
