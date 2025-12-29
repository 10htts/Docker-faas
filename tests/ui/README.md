# UI E2E Tests (Playwright)

These tests validate the Web UI behaviors added for security and usability.

Prerequisites:
- Docker FaaS gateway running at `http://localhost:8080`
- Node.js 18+ installed

Install dependencies:
```
cd tests/ui
npm install
npx playwright install
```

Run tests:
```
cd tests/ui
npx playwright test
```

Environment overrides:
- `GATEWAY_URL` (default: `http://localhost:8080`)
- `UI_BASE_URL` (default: `${GATEWAY_URL}/ui`)
- `AUTH_USER` / `AUTH_PASSWORD` (defaults: `admin` / `admin`)
