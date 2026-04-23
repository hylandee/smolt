# Smolt 5x5 - Go Backend

A self-hosted Stronglifts 5x5 workout tracker with Go backend, SQLite database, and HTMX frontend.

## Quick Start

### Prerequisites
- Go 1.21+

### Build & Run

```bash
# Download dependencies
go mod download

# Run the server
go run ./cmd/stronglifts/main.go
```

Server will start on `http://localhost:3000`
>>>>>>> 5c2c635 (initial stuff)

### Tests

```bash
<<<<<<< HEAD
cargo test --features ssr
```

## Building for Production

```bash
cargo leptos build --release
```

Output:
- Binary:      `target/release/smolt`
- Static site: `target/site/`

Copy both to the server. The binary expects `target/site/` in the working
directory (or set `LEPTOS_SITE_ROOT`).

## Database

Migrations run automatically on startup.

```bash
DATABASE_URL=sqlite:/var/lib/smolt/smolt.db ./smolt
```

## Deployment (Nginx reverse proxy on Linode)

### systemd service `/etc/systemd/system/smolt.service`

```ini
[Unit]
Description=Smolt workout tracker
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/smolt
Environment=HOST=127.0.0.1
Environment=PORT=3000
Environment=DATABASE_URL=sqlite:/var/lib/smolt/smolt.db
Environment=RUST_LOG=smolt=warn
ExecStart=/opt/smolt/smolt
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now smolt
```

### Nginx config

```nginx
server {
    listen 443 ssl;
    server_name workouts.yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/workouts.yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/workouts.yourdomain.com/privkey.pem;

    location / {
        proxy_pass         http://127.0.0.1:3000;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_buffering    off;
    }
}
=======
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestRegisterWithValidData ./internal/auth
>>>>>>> 5c2c635 (initial stuff)
```

## Project Structure

```
<<<<<<< HEAD
smolt/
├── migrations/        SQLite migrations (run on startup)
├── public/            Static assets copied to target/site/
├── src/
│   ├── app.rs         Leptos App component + router
│   ├── components/    UI components (pages)
│   ├── server_fns/    Leptos server functions (DB access)
│   ├── models.rs      Shared data types
│   ├── fallback.rs    Static-file fallback handler
│   ├── lib.rs         Crate root + WASM hydration entry
│   └── main.rs        Axum server entry
├── style/main.scss    Dark-theme styles
└── Cargo.toml
```
=======
cmd/
  stronglifts/
    main.go          # Entry point
internal/
  auth/
    auth.go          # User registration/login logic
    session.go       # Session management
    middleware.go    # Auth middleware & context
    auth_test.go     # Auth integration tests
  db/
    db.go            # Database initialization & schema
  handlers/
    auth.go          # HTTP handlers for auth routes
migrations/          # SQL migrations (if needed)
```

## Architecture

### Phase 1: Auth & DB (Current)
- [x] User registration with validation
- [x] Login with bcrypt password verification  
- [x] Session-based authentication (30-day cookies)
- [x] Protected routes with middleware
- [x] SQLite schema (5 tables)
- [x] Integration tests (11 tests)

### Phase 2: Workout Engine (Next)
- [ ] Exercise definitions (A/B program alternation)
- [ ] Workout start/progress/finish endpoints
- [ ] Linear progression (+increment on success)
- [ ] Deload logic (-10% on 3 failures)
- [ ] Integration tests

### Phase 3-8: UI, History, Deployment
- [ ] Dashboard with HTMX
- [ ] Active workout UI (set bubbles, rest timer)
- [ ] History & charts
- [ ] Body metrics
- [ ] Settings UI
- [ ] Fat JAR build & systemd deployment

## Database Schema

- `users` - User accounts with bcrypt hashes
- `workout_sessions` - Logged workouts (A/B programs)
- `exercise_sets` - Individual sets within a session (5 sets per exercise)
- `lift_progress` - Current weight & progression state per exercise
- `body_metrics` - Weight tracking over time

## API Endpoints

### Auth
- `POST /register` - Register new account
- `GET /register` - Register form
- `POST /login` - Login
- `GET /login` - Login form
- `POST /logout` - Clear session

### Protected
- `GET /` - Dashboard (requires auth)
>>>>>>> 5c2c635 (initial stuff)
