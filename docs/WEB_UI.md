# Docker FaaS Web UI

A modern, production-focused web interface for managing Docker FaaS functions, secrets, and system monitoring.

## Features

### 🔐 Authentication & Session Management
- Basic auth login with gateway URL configuration
- Session persistence in browser local storage
- Secure credential handling
- Quick disconnect/reconnect

### 📊 System Overview Dashboard
- Real-time health monitoring
- Function count and replica statistics
- Debug mode tracking
- Gateway version information
- Quick action buttons for common tasks
- Documentation links

### 📦 Functions Management
- List all deployed functions with search/filter
- View detailed function configuration
- Create new functions with full OpenFaaS compatibility
- Update existing functions
- Delete functions with confirmation
- Scale functions with visual replica controls
- Invoke functions with custom headers and body
- Real-time invocation response with latency tracking

### 🔑 Secrets Management
- Create secrets securely
- Update existing secrets
- Delete secrets with confirmation
- List all secrets (names only, values never exposed)
- Automatic validation before deployment

### 📝 Logs Viewer
- Select function from dropdown
- Configurable tail length (50/100/200/500 lines)
- Real-time log fetching
- Monospace display for readability

### 🐛 Debug Mode Support
- Toggle debug mode per function
- Visual warnings when debug ports exposed
- Port binding information display
- Security recommendations

### 🎨 Design Philosophy
- Bold, atmospheric dark theme
- Distinct sectioning with strong hierarchy
- Compact tables with clear status badges
- Smooth animations and transitions
- Mobile-responsive layout
- No default fonts - custom typography
- Gradient background with depth

## Access

The web UI is accessible at:

```
http://localhost:8080/ui/
```

Or simply navigate to the gateway root:

```
http://localhost:8080/
```

This will automatically redirect to the UI.

## Screenshots

### Login Screen
- Clean, centered login form
- Gateway URL configuration
- Saved credentials support
- Connection status feedback

### Overview Dashboard
- System health indicator
- Key metrics at a glance
- Quick action buttons
- Recent activity summary

### Functions List
- Searchable, filterable table
- Status badges for debug mode
- Replica counts
- Last updated timestamps
- Quick view actions

### Function Detail
- Configuration summary
- Replica management with scale controls
- Real-time replica status
- Inline function invocation
- Request/response viewer

### Create/Edit Function
- OpenFaaS-compatible form
- JSON environment variables
- JSON labels
- Comma-separated secrets
- Resource limits (CPU/memory)
- Advanced options (read-only FS, debug mode)
- Clear validation feedback

### Secrets Management
- Secure secret creation
- Update with password-style value input
- Delete with confirmation
- Names-only display for security

### Logs Viewer
- Function selector
- Tail length selector
- Monospace log output
- Manual refresh control

## Usage Examples

### 1. Connect to Gateway

1. Navigate to `http://localhost:8080/ui/`
2. Enter gateway URL: `http://localhost:8080`
3. Enter username: `admin`
4. Enter password: `admin`
5. Click "Connect"

### 2. Deploy a New Function

1. Click "Deploy New Function" or navigate to Functions → Create
2. Fill in the form:
   - **Function Name**: `hello-world`
   - **Image**: `ghcr.io/openfaas/alpine:latest`
   - **Environment Variables** (optional):
     ```json
     {"fprocess": "cat"}
     ```
   - **Labels** (optional):
     ```json
     {"app": "demo"}
     ```
3. Click "Deploy Function"

### 3. Scale a Function

1. Navigate to Functions
2. Click "View" on a function
3. In the "Replicas" section, adjust the scale value
4. Click "Apply"

### 4. Invoke a Function

1. View function details
2. Scroll to "Invoke Function" section
3. Select HTTP method (GET, POST, etc.)
4. Add headers (optional):
   ```json
   {"Content-Type": "application/json"}
   ```
5. Add request body (optional):
   ```
   {"message": "Hello from UI"}
   ```
6. Click "Send Request"
7. View response status, headers, body, and latency

### 5. Create a Secret

1. Navigate to Secrets
2. Click "Create Secret"
3. Enter secret name: `db-password`
4. Enter secret value: `my-secure-password`
5. Click "Save Secret"

### 6. Deploy Function with Secrets

1. Create function as usual
2. In "Secrets" field, enter: `db-password,api-key`
3. Deploy function
4. Secrets will be mounted read-only at `/var/openfaas/secrets/`

### 7. View Logs

1. Navigate to Logs
2. Select function from dropdown
3. Select tail length (e.g., 100 lines)
4. Click "Fetch Logs"

### 8. Enable Debug Mode

1. Edit or create a function
2. Check "Enable Debug Mode" in Advanced Options
3. Note the security warning
4. Deploy/update function
5. View debug ports in function details

## Configuration

The UI reads configuration from the login screen:

- **Gateway URL**: The base URL of your Docker FaaS gateway
- **Username**: Basic auth username (default: `admin`)
- **Password**: Basic auth password (default: `admin`)

These are stored in browser `localStorage` for persistence across sessions.

## Security Considerations

### Debug Mode Warning
When debug mode is enabled, the UI displays a warning:

> ⚠️ Debug mode exposes debugger ports. Check DEBUG_BIND_ADDRESS configuration.

This reminds operators that debug ports should be bound to `127.0.0.1` (localhost only) for security.

### Secrets Security
- Secret values are **never** displayed in the UI
- Only secret names are shown in the list
- Values are only entered during create/update
- Secrets are transmitted over HTTPS in production

### Authentication
- Basic auth credentials are required for all API calls
- Session data stored in browser localStorage
- Logout clears session immediately

## Auto-Refresh

The UI automatically refreshes data every 30 seconds when you're viewing:
- Overview dashboard
- Functions list
- Secrets list

Manual refresh is available via the refresh button in the header.

## Mobile Support

The UI is fully responsive and works on:
- Desktop browsers
- Tablets
- Mobile phones

Key mobile optimizations:
- Touch-friendly buttons and controls
- Responsive grid layouts
- Scrollable tables
- Hamburger menu navigation (auto)

## Browser Compatibility

Tested and supported on:
- Chrome/Edge (latest)
- Firefox (latest)
- Safari (latest)

Requires:
- ES6+ JavaScript support
- CSS Grid and Flexbox
- LocalStorage API

## Development

### File Structure

```
web/
└── static/
    ├── index.html          # Main HTML structure
    ├── css/
    │   └── styles.css      # Complete styling
    └── js/
        └── app.js          # Full application logic
```

### Customization

**Colors**: Edit CSS variables in `:root` selector:

```css
:root {
    --accent-primary: #3b82f6;  /* Primary blue */
    --accent-success: #10b981;  /* Success green */
    --accent-danger: #ef4444;   /* Danger red */
    /* ... */
}
```

**Auto-refresh interval**: Edit `app.js`:

```javascript
startAutoRefresh() {
    this.refreshInterval = setInterval(() => {
        this.refreshCurrentView();
    }, 30000); // Change to desired milliseconds
}
```

**Default gateway URL**: Edit `index.html`:

```html
<input type="text" id="gateway-url" value="http://localhost:8080" placeholder="http://localhost:8080">
```

## API Integration

The UI integrates with these gateway endpoints:

### System Endpoints
- `GET /system/info` - Gateway version and info
- `GET /system/functions` - List all functions
- `POST /system/functions` - Create function
- `PUT /system/functions` - Update function
- `DELETE /system/functions?functionName=X` - Delete function
- `POST /system/scale-function/{name}` - Scale function
- `GET /system/logs?name=X&tail=N` - Get logs

### Secret Endpoints
- `GET /system/secrets` - List secrets
- `POST /system/secrets` - Create secret
- `PUT /system/secrets` - Update secret
- `DELETE /system/secrets?name=X` - Delete secret

### Function Invocation
- `POST /function/{name}` - Invoke function (supports all HTTP methods)

All API calls include `Authorization: Basic <base64>` header with credentials.

## Troubleshooting

### Cannot Connect to Gateway

**Error**: Connection failed

**Solutions**:
- Verify gateway is running: `docker ps`
- Check gateway URL is correct
- Ensure auth credentials match gateway configuration
- Check network connectivity

### Functions Not Loading

**Error**: Failed to load functions

**Solutions**:
- Check auth credentials
- Verify gateway is healthy: `http://localhost:8080/healthz`
- Check browser console for errors
- Refresh page

### Debug Ports Not Showing

**Issue**: Debug ports not visible in function details

**Solutions**:
- Ensure debug mode is enabled on function
- Check function has at least one replica running
- Verify container inspection is working
- Check gateway logs for errors

### Logs Not Displaying

**Error**: No logs available

**Solutions**:
- Ensure function has run at least once
- Check function name is correct
- Verify gateway can access Docker daemon
- Try increasing tail length

## Production Deployment

### With Reverse Proxy (Recommended)

**nginx configuration**:

```nginx
server {
    listen 80;
    server_name faas.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Add TLS**:

```nginx
server {
    listen 443 ssl http2;
    server_name faas.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        # ... other headers
    }
}
```

### Environment Variables

Ensure these are set for production:

```bash
AUTH_ENABLED=true
AUTH_USER=your-username
AUTH_PASSWORD=strong-password
DEBUG_BIND_ADDRESS=127.0.0.1  # Localhost only for security
```

### Security Checklist

- [ ] Enable HTTPS/TLS
- [ ] Use strong auth password
- [ ] Set `DEBUG_BIND_ADDRESS=127.0.0.1`
- [ ] Enable rate limiting (via reverse proxy)
- [ ] Set up firewall rules
- [ ] Regular gateway updates
- [ ] Monitor access logs

## Future Enhancements

Potential improvements for future versions:

- **Real-time Updates**: WebSocket support for live function status
- **Metrics Dashboard**: Integration with Prometheus/Grafana
- **Multi-User**: Role-based access control
- **Function Templates**: Quick-deploy templates for common use cases
- **Batch Operations**: Deploy/delete multiple functions at once
- **Export/Import**: Configuration backup and restore
- **Dark/Light Theme Toggle**: User preference
- **Audit Logs**: Track all UI actions

## Support

For issues or questions:
- Check [GitHub Issues](https://github.com/docker-faas/docker-faas/issues)
- Review [API Documentation](API.md)
- Read [Getting Started Guide](GETTING_STARTED.md)

---

**Version**: 2.0.0
**Last Updated**: December 2024
