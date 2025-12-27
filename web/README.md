# Docker FaaS Web UI

Modern, production-focused web interface for Docker FaaS.

## Quick Start

1. Start the gateway:
   ```bash
   docker-compose up -d
   ```

2. Open browser to:
   ```
   http://localhost:8080/ui/
   ```

3. Login with default credentials:
   - Username: `admin`
   - Password: `admin`

## Features

- 🔐 **Secure Login** - Basic auth with session persistence
- 📊 **System Dashboard** - Real-time health and metrics
- 📦 **Function Management** - Deploy, update, scale, delete
- 🚀 **Live Invocation** - Test functions with custom requests
- 🔑 **Secrets Management** - Create and manage secrets
- 📝 **Logs Viewer** - Real-time function logs
- 🐛 **Debug Controls** - Enable/disable debug mode with warnings
- 🎨 **Beautiful Design** - Dark theme with smooth animations

## Architecture

```
web/
└── static/
    ├── index.html          # Main HTML (SPA structure)
    ├── css/
    │   └── styles.css      # Complete styling (~800 lines)
    └── js/
        └── app.js          # Application logic (~1000 lines)
```

## Technology Stack

- **Pure JavaScript** - No frameworks, vanilla ES6+
- **CSS Grid & Flexbox** - Modern responsive layout
- **LocalStorage** - Session persistence
- **Fetch API** - RESTful API calls with Basic Auth
- **CSS Variables** - Easy theming

## API Integration

The UI communicates with the Docker FaaS gateway REST API:

- System endpoints (`/system/info`, `/system/functions`, etc.)
- Function invocation (`/function/{name}`)
- Secrets management (`/system/secrets`)
- Logs retrieval (`/system/logs`)

All requests include `Authorization: Basic <credentials>` header.

## Development

No build step required - pure HTML/CSS/JS.

### Local Development

1. Ensure gateway is running
2. Open `web/static/index.html` in browser
3. Or use any HTTP server:
   ```bash
   cd web/static
   python -m http.server 3000
   ```

### Customization

**Colors**: Edit CSS variables in `styles.css`:
```css
:root {
    --accent-primary: #3b82f6;
    --accent-success: #10b981;
    /* ... */
}
```

**Auto-refresh**: Edit `app.js`:
```javascript
startAutoRefresh() {
    this.refreshInterval = setInterval(() => {
        this.refreshCurrentView();
    }, 30000); // Change interval
}
```

## Deployment

### Docker Compose (Included)

The UI is automatically served by the gateway at `/ui/`:

```yaml
volumes:
  - ./web/static:/app/web/static
```

### Production

For production deployments:

1. **Serve via Gateway** (default):
   - UI available at `http://gateway:8080/ui/`
   - No additional configuration needed

2. **Separate Web Server** (optional):
   ```nginx
   server {
       listen 80;
       root /path/to/docker-faas/web/static;

       location /api/ {
           proxy_pass http://gateway:8080/;
       }
   }
   ```

3. **CDN/Static Hosting** (optional):
   - Upload `web/static/*` to CDN
   - Configure gateway URL in login screen

## Browser Support

- Chrome/Edge 90+
- Firefox 88+
- Safari 14+

Requires:
- ES6+ (async/await, arrow functions, template literals)
- CSS Grid and Flexbox
- LocalStorage API
- Fetch API

## Security

- **No secrets in localStorage** - Only session credentials
- **Basic Auth on all API calls** - Never stored in plain text
- **HTTPS recommended** - Use reverse proxy in production
- **Debug warnings** - Clear UI warnings for security risks

## Documentation

See [Web UI Guide](../docs/WEB_UI.md) for:
- Detailed feature documentation
- Usage examples
- Troubleshooting
- Production deployment guide

## License

MIT License - Same as Docker FaaS project
