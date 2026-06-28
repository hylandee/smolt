# smolt

Self-hosted StrongLifts 5x5 workout tracker. Go + SQLite + chi + HTMX. Mobile-first.

## Features

- SL5x5 A/B program with automatic linear progression
- Warm-up set display with IPF/IWF plate calculator
- Rest timer (elapsed, frosted glass pill)
- Standalone workout editor for custom sessions
- Body weight logging with progress chart
- Cardio tracking (treadmill, bike, staircase, elliptical)
- Progress charts with time range filtering
- Multiple themes including dark/high-contrast
- Backup and restore (JSON export/import)

## Quick Start

```bash
# Run (needs .env or env vars)
go run ./cmd/stronglifts

# Build
go build ./...

# Test
go test ./...
```

### Environment variables

```
PORT=3000
DB_PATH=stronglifts.db
SECURE_COOKIES=true   # set true when behind a TLS-terminating reverse proxy
```

Migrations run automatically on startup via goose.

## Project Structure

```
cmd/stronglifts/        Entry point
internal/
  auth/                 Registration, login, sessions, user prefs
  db/                   DB wrapper (WAL, single connection, WithTx)
  handlers/             chi HTTP handlers
  templates/            html/template files
  workout/              SL5x5 domain: progression, warmup, plates
migrations/             Goose SQL migrations
scripts/
  setup_service.sh      Systemd service install / restart helper
```

## Deployment (Linode + systemd)

The service is managed by a helper script. SSH in, then:

```bash
cd /root/dev/smolt
git pull
sudo bash scripts/setup_service.sh --restart-latest
```

First-time setup (creates and enables the systemd unit):

```bash
sudo bash scripts/setup_service.sh
```

View logs:

```bash
journalctl -u smolt -f
```

### Nginx (reverse proxy)

The app runs on port 3000, behind nginx which terminates TLS. Key points:

- `SECURE_COOKIES=true` must be set in the systemd service (`/etc/systemd/system/smolt.service`)
- Port 80 redirects to HTTPS; `/.well-known/acme-challenge/` is served from the static root for certbot renewals
- The default server block silently drops (`444`) requests for unmatched hostnames to suppress scanner noise
- Certbot auto-renews via systemd timer; test with `certbot renew --dry-run`

### Journal log size

journald is capped at 200MB (`/etc/systemd/journald.conf`):

```ini
[Journal]
SystemMaxUse=200M
SystemKeepFree=1G
```
