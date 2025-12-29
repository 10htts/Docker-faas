const { defineConfig } = require('@playwright/test');

const gatewayBase = (process.env.GATEWAY_URL || 'http://localhost:8080').replace(/\/+$/, '');
const baseURL = (process.env.UI_BASE_URL || `${gatewayBase}/ui`).replace(/\/+$/, '') + '/';

module.exports = defineConfig({
  testDir: './tests',
  timeout: 60000,
  expect: {
    timeout: 10000
  },
  use: {
    baseURL,
    viewport: { width: 1280, height: 720 },
    acceptDownloads: true,
    trace: 'retain-on-failure'
  },
  reporter: [['list'], ['html', { outputFolder: 'playwright-report', open: 'never' }]],
  projects: [
    { name: 'chromium', use: { browserName: 'chromium' } }
  ]
});
