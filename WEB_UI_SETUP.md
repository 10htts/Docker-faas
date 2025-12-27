# Docker FaaS Web UI - Quick Setup Guide

## CORS Issue Fixed! ✅

The Web UI now includes CORS support, allowing it to communicate with the gateway API from any origin.

## Setup

### 1. Rebuild the Gateway (with CORS support)

```bash
cd c:\Users\10htts\Desktop\GIT\Docker-faas

# Rebuild
go build -o bin/gateway.exe ./cmd/gateway

# Or rebuild Docker image
docker-compose build
```

### 2. Start the Gateway

**Option A: Docker Compose (Recommended)**
```bash
docker-compose up -d
```

**Option B: Local Binary**
```bash
./bin/gateway.exe
```

### 3. Access the Web UI

Open your browser to:
```
http://localhost:8080/ui/
```

Or simply:
```
http://localhost:8080/
```
(Automatically redirects to `/ui/`)

### 4. Login

Default credentials:
- **Gateway URL**: `http://localhost:8080`
- **Username**: `admin`
- **Password**: `admin`

Click "Connect" and you're in!

## What Was Fixed

### CORS Middleware Added

Created [pkg/middleware/cors.go](pkg/middleware/cors.go):
- Handles preflight OPTIONS requests
- Sets proper CORS headers
- Allows all origins for development (`Access-Control-Allow-Origin: *`)
- Supports credentials

### Gateway Integration

Updated [cmd/gateway/main.go](cmd/gateway/main.go):
- Added CORS middleware to the chain
- Order: CORS → Logging → Auth → Routes
- CORS applied before authentication for preflight requests

### Docker Support

Updated:
- [Dockerfile](Dockerfile) - Copies web UI files into image
- [docker-compose.yml](docker-compose.yml) - Mounts web directory

## Testing the UI

### Quick Test Workflow

1. **Login**
   - Navigate to `http://localhost:8080/ui/`
   - Enter credentials
   - Click "Connect"

2. **View Overview**
   - Should see system health: "Healthy"
   - Statistics: 0 functions initially
   - Gateway version: v2.0

3. **Deploy a Function**
   - Click "Deploy New Function"
   - Function Name: `hello`
   - Image: `ghcr.io/openfaas/alpine:latest`
   - Click "Deploy Function"

4. **View Function**
   - Go to Functions tab
   - Click "View" on hello function
   - See configuration and replicas

5. **Invoke Function**
   - In function detail, scroll to "Invoke Function"
   - Method: POST
   - Body: `Hello from UI!`
   - Click "Send Request"
   - See response and latency

6. **Scale Function**
   - Set replicas to 3
   - Click "Apply"
   - See 3 replica cards appear

7. **Create Secret**
   - Go to Secrets tab
   - Click "Create Secret"
   - Name: `test-secret`
   - Value: `my-secret-value`
   - Click "Save Secret"

8. **View Logs**
   - Go to Logs tab
   - Select function: `hello`
   - Tail: 100 lines
   - Click "Fetch Logs"

## Troubleshooting

### CORS Errors

**Error**: Access-Control-Allow-Origin header missing

**Solution**: Ensure you're using the updated gateway with CORS middleware:
```bash
go build -o bin/gateway.exe ./cmd/gateway
```

### Gateway Not Reachable

**Error**: Connection failed

**Solutions**:
1. Check gateway is running:
   ```bash
   docker ps | grep docker-faas-gateway
   ```

2. Verify port 8080 is accessible:
   ```bash
   curl http://localhost:8080/healthz
   ```

3. Check firewall settings

### UI Not Loading

**Error**: 404 Not Found for `/ui/`

**Solutions**:
1. Verify web files exist:
   ```bash
   ls web/static/
   # Should see: index.html, css/, js/
   ```

2. Check Docker volume mount:
   ```bash
   docker inspect docker-faas-gateway | grep -A 5 Mounts
   ```

3. Rebuild Docker image:
   ```bash
   docker-compose build
   docker-compose up -d
   ```

### Functions Not Appearing

**Issue**: Functions list empty but you deployed functions via CLI

**Solutions**:
1. Click refresh button in header
2. Check auth credentials match
3. Try logging out and back in

## Production Deployment

### Security Considerations

1. **Change Default Password**:
   ```bash
   export AUTH_PASSWORD=strong-secure-password
   ```

2. **Restrict CORS Origins**:
   Edit [cmd/gateway/main.go](cmd/gateway/main.go):
   ```go
   corsMiddleware := middleware.NewCORSMiddleware([]string{
       "https://faas.yourdomain.com",
   })
   ```

3. **Enable HTTPS**:
   - Use reverse proxy (nginx/traefik)
   - Terminate TLS at proxy
   - Forward to gateway on localhost:8080

4. **Set Secure Debug Binding**:
   ```bash
   export DEBUG_BIND_ADDRESS=127.0.0.1
   ```

### Example nginx Config

```nginx
server {
    listen 443 ssl http2;
    server_name faas.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

## Features Summary

✅ **Complete UI Implementation**:
- Login & session management
- System overview dashboard
- Functions CRUD operations
- Function scaling
- Live function invocation
- Secrets management
- Logs viewer
- Debug mode controls

✅ **Modern Design**:
- Dark atmospheric theme
- Responsive mobile layout
- Smooth animations
- Toast notifications
- Real-time updates

✅ **Security**:
- Basic auth integration
- CORS support
- Session persistence
- Secret value protection
- Debug warnings

## Next Steps

1. **Explore the UI**:
   - Deploy various functions
   - Test scaling
   - Create secrets
   - View logs

2. **Integrate with CI/CD**:
   - Use UI for manual deployments
   - Use API/CLI for automation

3. **Monitor System**:
   - Check health status
   - Monitor replica counts
   - Review function metrics

4. **Customize**:
   - Edit colors in `web/static/css/styles.css`
   - Adjust auto-refresh in `web/static/js/app.js`
   - Add custom branding

## Support

For issues or questions:
- Review [Web UI Guide](docs/WEB_UI.md)
- Check [GitHub Issues](https://github.com/docker-faas/docker-faas/issues)
- Read [API Documentation](docs/API.md)

---

**Enjoy your new Docker FaaS Web UI! 🚀**
