# backend

The fieldops API: Go 1.23+, Gin, GORM, PostgreSQL. Read `docs/CONVENTIONS.md`,
`docs/BUSINESS_RULES.md` and `docs/ARCHITECTURE.md` at the repo root before
changing anything here.

## Run

```bash
cp ../.env.example ../.env       # once; every variable is documented there
docker compose -f ../docker-compose.yml up -d db
make -C .. backend               # migrates, seeds (SEED_ON_BOOT=true), serves :8080
```

The binary reads `.env` from the working directory or its parent, then the
process environment (which wins). `make seed` runs `go run ./... --seed-only`:
migrate, seed, exit. The seed is idempotent and builds its visits around
today's Europe/London day, so it is safe on every boot.

```bash
curl -s localhost:8080/api/health
curl -s -X POST localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@fieldops.local","password":"fieldops"}'
```

Seed logins: `admin@fieldops.local`, `dispatcher@fieldops.local`,
`tech1@fieldops.local` … `tech3@fieldops.local`, password `fieldops`.
`tech4@fieldops.local` (Dana Departed) is soft-deleted and cannot log in; her
visits and clock events from the last two days are still attributed to her.

## Test

```bash
make -C .. test-backend          # go test ./...
go test ./...                    # from this directory
TZ=Asia/Tokyo go test ./...      # the UK-day logic must not depend on the host zone
go vet ./... && gofmt -l .
```

Tests run against an in-memory SQLite database (`github.com/glebarez/sqlite`);
no PostgreSQL is needed. Handler tests go through the real router in
`internal/api/server.go`, so the wiring under test is the wiring in `main.go`.

## Layout

```
main.go, config.go        boot: .env + env → config, DB, migrate, seed, serve
internal/models           GORM structs + constants.go (roles, statuses, priorities, audit names)
internal/services         all business logic and queries
  ukday.go                Europe/London day helpers (the only place a "day" is decided)
  crypto.go               AES-GCM for users.licence_number
  dbscope.go              UnscopedUserAssoc / UnscopedSiteAssoc for attribution preloads
internal/api              handlers, views (wire shapes), routes.go (one route per line)
internal/middleware       JWT auth, role gate, request log, recovery, audit registry
internal/database         connect (postgres / sqlite for tests), migrate, seed
```

## Docker

```bash
docker build -t fieldops-backend .
docker run --rm -p 8080:8080 \
  -e DATABASE_URL=postgres://fieldops:fieldops@host.docker.internal:5432/fieldops?sslmode=disable \
  -e JWT_SECRET=change-me-32-chars-minimum-please \
  -e ENCRYPTION_KEY=MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY= \
  -e SEED_ON_BOOT=true fieldops-backend
```
