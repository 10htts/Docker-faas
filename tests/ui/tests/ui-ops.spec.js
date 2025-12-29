const { test, expect } = require('@playwright/test');
const fs = require('fs/promises');

const gatewayUrl = (process.env.GATEWAY_URL || 'http://localhost:8080').replace(/\/+$/, '');
const username = process.env.AUTH_USER || process.env.DOCKER_FAAS_USER || 'admin';
const password = process.env.AUTH_PASSWORD || process.env.DOCKER_FAAS_PASSWORD || 'admin';

async function login(page) {
  await page.goto('/');
  await page.fill('#gateway-url', gatewayUrl);
  await page.fill('#username', username);
  await page.fill('#password', password);
  await page.click('#login-btn');
  await expect(page.locator('#app-screen')).toHaveClass(/active/);
}

test('export functions downloads backup JSON', async ({ page }, testInfo) => {
  await login(page);

  const downloadPromise = page.waitForEvent('download');
  await page.click('#export-functions-btn');
  const download = await downloadPromise;
  const targetPath = testInfo.outputPath('docker-faas-backup.json');
  await download.saveAs(targetPath);

  const raw = await fs.readFile(targetPath, 'utf8');
  const data = JSON.parse(raw);

  expect(Array.isArray(data.functions)).toBe(true);
});

test('metrics view loads raw metrics', async ({ page }) => {
  await login(page);

  await page.click('.nav-item[data-view="metrics"]');
  await expect(page.locator('#metrics-view')).toHaveClass(/active/);
  await expect(page.locator('#metrics-raw')).toContainText('gateway_http_requests_total');
});
