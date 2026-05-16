# Deployment

> Single binary, no runtime dependencies. Deploy however you deploy Go binaries.

---

## Docker (Recommended)

```bash
cp .env.example .env    # edit with production values
docker compose up -d
```

The Dockerfile uses multi-stage build (golang-alpine → scratch). The image contains only the binary, templates, static assets, and articles.

## Binary + systemd

```bash
# Build
make build-release

# Copy to server
scp build/markgo-linux-amd64 server:/usr/local/bin/markgo

# Create service user
sudo useradd --system --no-create-home --shell /bin/false markgo
sudo mkdir -p /opt/markgo/{articles,web,logs}
sudo chown -R markgo:markgo /opt/markgo

# Install systemd unit
sudo cp deployments/etc/systemd/system/markgo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now markgo
```

### Production .env

```bash
sudo tee /opt/markgo/.env << 'EOF'
ENVIRONMENT=production
PORT=3000
BASE_URL=https://yourdomain.com
BLOG_TITLE=Your Blog
BLOG_AUTHOR=Your Name

ADMIN_USERNAME=your-admin
ADMIN_PASSWORD=a-strong-password

LOG_LEVEL=warn
LOG_OUTPUT=file
LOG_FILE=/opt/markgo/logs/markgo.log

CACHE_TTL=24h
CORS_ALLOWED_ORIGINS=https://yourdomain.com
EOF

sudo chown markgo:markgo /opt/markgo/.env
sudo chmod 600 /opt/markgo/.env
```

## Reverse Proxy

Put Nginx or Caddy in front for TLS termination, compression, and static asset caching.

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Cache static assets at the proxy level
    location /static/ {
        proxy_pass http://127.0.0.1:3000;
        proxy_cache_valid 200 7d;
        add_header Cache-Control "public, max-age=604800, immutable";
    }

    # Service Worker must be served from root
    location = /sw.js {
        proxy_pass http://127.0.0.1:3000;
        add_header Cache-Control "no-cache";
    }
}
```

### Caddy

```
yourdomain.com {
    reverse_proxy localhost:3000
}
```

Caddy handles TLS automatically via Let's Encrypt.

## Binary Deployment (Content-Only)

The MarkGo binary embeds all web assets. You only need three things on the server:

1. The `markgo` binary
2. An `articles/` directory with your content
3. A `.env` configuration file

```bash
# On the server
mkdir -p /opt/markgo/articles
# Copy your content
scp articles/*.md server:/opt/markgo/articles/
scp .env server:/opt/markgo/.env
# Copy the binary
scp build/markgo-linux-amd64 server:/opt/markgo/markgo
# Start
ssh server '/opt/markgo/markgo serve'
```

To customize templates or CSS, create `web/templates/` and `web/static/` directories on the server. Filesystem paths take precedence over embedded assets.

### Upload Storage

When compose upload is enabled, files are stored in `UPLOAD_PATH` (default `./uploads`) in slug-scoped subdirectories. Ensure this directory:
- Is writable by the markgo process
- Persists across container restarts (mount as a Docker volume)
- Is included in backup procedures

For large file uploads, consider increasing `SERVER_WRITE_TIMEOUT` (default 15s).

### SSL with Let's Encrypt

```bash
sudo certbot certonly --nginx -d yourdomain.com
```

---

## PWA Deployment Notes

The Service Worker (`sw.js`) must be served from the root path to control the full scope. The Nginx config above handles this. If using a CDN, ensure `/sw.js` is not cached aggressively — use `Cache-Control: no-cache`.

The PWA manifest is generated dynamically at `/manifest.json` from your `.env` configuration (blog title, description, icons).

---

## Health Checks

```bash
curl -f http://localhost:3000/health
```

Returns JSON with status, uptime, and version. Use this for load balancer health checks and monitoring.

## Monitoring

```bash
# Service status
systemctl status markgo

# Application logs
journalctl -u markgo -f

# Application health (public)
curl http://localhost:3000/health

# Performance metrics (admin-only — requires session cookie)
curl --cookie "_session=$ADMIN_SESSION_TOKEN" http://localhost:3000/admin/metrics
```

---

## Updates

```bash
sudo systemctl stop markgo
sudo cp /usr/local/bin/markgo /usr/local/bin/markgo.bak
sudo cp new-binary /usr/local/bin/markgo
sudo systemctl start markgo
```

With Docker: rebuild and restart the container.

---

## Troubleshooting

**Service won't start**: Check `journalctl -u markgo --no-pager`. Common causes: port in use, missing .env, file permissions.

**High memory**: Reduce `CACHE_MAX_SIZE` in .env. Typical usage is ~30MB.

**Articles not appearing**: Run `POST /admin/articles/reload` or restart the server. Check `ARTICLES_PATH` is correct.

**SSL errors**: `nginx -t` to test config. `openssl x509 -in cert.pem -text -noout` to verify certificate.

**Service Worker stale**: Users may have old SW cached. Increment the `CACHE_VERSION` in `sw.js` to force update on next visit.
