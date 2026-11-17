import { defineConfig, devices } from '@playwright/test';
export function assertIsolatedLiveTarget(url: string) {
  const live = ['AUTH', 'PUBLIC', 'BUYER', 'MVP'].some(
    (scope) => process.env[`SREZ_${scope}_E2E_LIVE`] === '1',
  );
  if (live && new URL(url).origin !== 'http://localhost:5175') {
    throw Error(
      'Live E2E may write only to isolated QA localhost:5175. Working catalog was left unchanged.',
    );
  }
}
const baseURL = process.env.SREZ_E2E_BASE_URL || 'http://localhost:5175';
assertIsolatedLiveTarget(baseURL);
export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 180000,
  expect: { timeout: 12000 },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    actionTimeout: 15000,
    navigationTimeout: 20000,
    ...devices['Desktop Chrome'],
    channel: 'chrome',
    headless: true,
    trace: 'off',
    screenshot: 'only-on-failure',
    video: 'off',
  },
});
