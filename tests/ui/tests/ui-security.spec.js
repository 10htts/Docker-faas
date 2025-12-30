const { test, expect } = require('@playwright/test');

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

test('login stores session without password', async ({ page }) => {
  await login(page);

  const session = await page.evaluate(() => localStorage.getItem('dockerfaas-session'));
  const sessionData = session ? JSON.parse(session) : {};

  expect(sessionData.gatewayUrl).toBe(gatewayUrl);
  expect(sessionData.username).toBe(username);
  expect(sessionData.password).toBeUndefined();
  expect(sessionData.token).toBeTruthy();

  await expect(page.locator('#password')).toHaveValue('');
});

test('session timeout logs out after inactivity', async ({ page }) => {
  await login(page);

  await page.evaluate(() => {
    app.sessionTimeoutMs = 200;
    app.resetInactivityTimer();
  });

  await expect(page.locator('#login-screen')).toHaveClass(/active/, { timeout: 2000 });
  await expect(page.locator('#app-screen')).not.toHaveClass(/active/);
});

test('loading overlay appears during API calls', async ({ page }) => {
  await page.route('**/system/info', async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 300));
    await route.continue();
  });

  await login(page);

  const requestPromise = page.evaluate(async () => {
    const response = await app.api('/system/info', { showLoading: true });
    return response.ok;
  });

  await expect(page.locator('#loading-overlay')).toHaveClass(/active/);
  await requestPromise;
  await expect(page.locator('#loading-overlay')).not.toHaveClass(/active/);
});
