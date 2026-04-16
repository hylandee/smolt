# Smolt — StrongLifts 5×5 Workout Tracker

A Rust full-stack web app built with [Leptos](https://leptos.dev/) (SSR),
[Axum](https://docs.rs/axum), [SQLite](https://sqlite.org/) (via sqlx), and Serde.

## Features

- **Workout A/B** (StrongLifts 5×5 templates)
  - A: Squat 5×5, Bench Press 5×5, Barbell Row 5×5
  - B: Squat 5×5, Overhead Press 5×5, Deadlift 1×5
- Log working weight per lift, mark sets complete/failed
- Body metrics log (bodyweight + notes)
- Progress charts rendered as inline SVG (no JS charting library)
- Server-side rendering with Leptos, hydrated in the browser

## Environment Variables

| Variable       | Default          | Description                                      |
|----------------|------------------|--------------------------------------------------|
| `HOST`         | `127.0.0.1`      | Bind address                                     |
| `PORT`         | `3000`           | Bind port                                        |
| `DATABASE_URL` | `sqlite:smolt.db`| SQLite URL. `DB_PATH` also accepted (path only). |
| `RUST_LOG`     | `smolt=info`     | Log filter (tracing-subscriber env-filter)       |

Create `.env` in the project root (loaded automatically on startup):

```env
HOST=127.0.0.1
PORT=3000
DATABASE_URL=sqlite:/var/data/smolt.db
RUST_LOG=smolt=info,axum=info
```

## Running Locally

### Prerequisites

```bash
rustup target add wasm32-unknown-unknown
cargo install cargo-leptos
```

### Development (hot-reload)

```bash
cargo leptos watch
# open http://127.0.0.1:3000
```

### Tests

```bash
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
```

## Project Structure

```
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
