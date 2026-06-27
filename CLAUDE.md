# smolt

Self-hosted StrongLifts 5x5 tracker. Go + SQLite + chi + HTMX. Mobile-first.

## Commands

```bash
go build ./...          # build
go test ./...           # all tests
go run ./cmd/stronglifts  # run (needs .env or env vars)
```

## Env vars

```
PORT=3000
DB_PATH=stronglifts.db
SECURE_COOKIES=false
```

## Key constraints

- `SetMaxOpenConns(1)` — never query inside a `rows.Next()` loop; collect rows into a slice first
- Migrations via goose (`migrations/` dir); run automatically on startup
- All handlers receive `*db.DB`, never raw `*sql.DB`
- Weight unit stored in user prefs (`lb_in` or `kg_cm`); plate calculator and warmup ramp are pure functions in `internal/workout/`
- 500 lb / 220 kg weight cap enforced client-side (blur clamp + JS calc) and server-side
