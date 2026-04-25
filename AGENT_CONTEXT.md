# Smolt Agentic Development Context

Updated: 2026-04-24
Commit snapshot: 7d38363

## Project Status

Smolt is a Go + SQLite workout tracker with server-rendered templates.
Key focus in this iteration was UX polish, deployment ergonomics, route/base-path behavior, theme system, and progression correctness.

## Major Decisions and Changes

### 1) Workout dashboard route moved to /workouts
- Dashboard is now accessible at /workouts for compatibility with a static homepage at /.
- Redirects and links that previously pointed to / for app-home flows were updated to /workouts.
- Pagination and post-finish navigation were fixed to avoid leaking users back to static-site root.

### 2) Deployment and service setup
- scripts/setup_service.sh exists for systemd setup and restart from local code.
- restart mode supports manual git pull workflow and rebuild/restart without auto-pull.
- Nginx generation was removed from script to keep it lean for existing reverse proxy setups.

### 3) Theme system: server-side persistence + expanded options
- Theme preference moved from local-only behavior to server-backed user preference.
- users.theme_pref persisted and normalized in DB setup/migration path.
- Profile page controls theme selection (mobile friendly).
- Added multiple themes, including Dark High Contrast.

### 4) Vim colorscheme easter egg
- Profile has a hidden Vim theme section.
- Trigger: press ':' then Esc on Profile.
- Unlock shows Vim colorscheme options and toast: "Vim mode unlocked."
- Vim colorscheme values are accepted server-side and normalized in DB.

### 5) Keep screen awake feature
- Added users.keep_awake preference (default on).
- Profile includes opt-out checkbox (persisted server-side).
- Base template uses Screen Wake Lock API when available.
- Wake lock reacquires on visibility return and releases when hidden/disabled.
- Behavior degrades safely on unsupported browsers.

### 6) Progression logic fix for manual weight edits
- Progression now uses performed session weight (last set weight per exercise) as baseline.
- FinishSession no longer assumes lift_progress.current_weight is the baseline when manual edits occurred.
- This allows both load-ups and load-downs to influence next progression correctly.

## Important Files to Re-open First

- cmd/stronglifts/main.go
- internal/handlers/auth.go
- internal/handlers/workout.go
- internal/workout/progression.go
- internal/workout/workout_test.go
- internal/auth/auth.go
- internal/db/db.go
- internal/templates/base.html
- internal/templates/profile.html
- scripts/setup_service.sh

## Testing Notes

Primary validation command:

go test ./...

Regression coverage now includes:
- theme preference persistence
- keep_awake opt-out persistence
- manual weight edits affecting progression baseline

## Deployment Notes (current workflow)

Server workflow:
1. git pull manually
2. sudo bash scripts/setup_service.sh --restart-latest
3. verify with systemctl status smolt and curl on app routes

## Open Follow-ups (optional)

- Add a profile setting for progression baseline strategy (last set vs first set vs average).
- Add server-side health endpoint for easier service probes.
- Add docs page for theme catalog and Vim easter egg.
- Add integration test for wake lock preference propagation in rendered HTML.

## Quick Resume Prompt (for future agent session)

"Continue from AGENT_CONTEXT.md in this repo. Start by running go test ./..., then inspect internal/workout/progression.go and internal/templates/profile.html. Keep /workouts as app root and preserve static-site compatibility at /."
