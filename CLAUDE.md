# base-project-go

Base/skeleton Go project using Gin (HTTP) and GORM (ORM), organized by
**domain/feature** rather than by technical layer — no interfaces, no
repository abstraction. Business logic lives in a `service` per domain that
talks to `*gorm.DB` directly (GORM is deliberately coupled, not hidden behind
an interface — see Conventions).

## Stack

- Go 1.25
- Gin (`github.com/gin-gonic/gin`) — HTTP router/framework
- GORM (`gorm.io/gorm`) — ORM
- SQLite via `github.com/glebarez/sqlite` (pure Go, no CGO required)
- `github.com/joho/godotenv` — loads `.env` into environment variables

Module path: `zhu`

## Architecture

Package by domain, not by layer. The entry point lives under `cmd/api`
(community-standard layout), application code under `internal/`
(community-standard layout — prevents other modules from importing our
internals). Cross-cutting concerns (config, DB connection) get their own
package; each domain gets a single self-contained package with model,
service, handler, and routes:

```
cmd/
  api/
    main.go                  entry point: load config, connect DB, migrate, register routes, start server
internal/
  config/
    config.go                env loading (PORT, DB_PATH)
  database/
    database.go              gorm.DB connection + AutoMigrate
  health/
    handler.go                Handler struct { DB *gorm.DB } — GET /api/health, pings DB (infra check, no model/service)
    routes.go                 RegisterRoutes(rg *gin.RouterGroup, h *Handler)
  docs/
    routes.go                 RegisterRoutes(rg *gin.RouterGroup) — mounts Swagger UI at /api/docs/*any (infra, no model/service)
  apierror/
    apierror.go                AppError type + Respond(c, err) — shared error JSON envelope (infra, no model/service)
  user/                       (example of a real domain, not yet implemented)
    model.go                  User struct (gorm.Model)
    service.go                UserService struct { DB *gorm.DB } — business rules + gorm calls
    handler.go                UserHandler struct { Service *UserService } — HTTP only: bind, validate input, status codes
    routes.go                 RegisterRoutes(rg *gin.RouterGroup, h *UserHandler)
```

Conventions:
- Each domain lives in its own package under `internal/<domain>/`, containing
  its model, service, handler, and route registration together — not split
  across shared `models/`, `handlers/`, `routes/` folders.
- `health` is an exception: it's an infra check, not a business domain, so it
  has no `model.go`/`service.go` — the handler talks to `*gorm.DB` directly.
- `handler.go` is HTTP-only: bind/validate the request, call the service,
  translate the result into a status code + JSON response. It holds no
  business logic and no direct `*gorm.DB` access.
- `service.go` holds the business logic and calls `*gorm.DB` directly. GORM
  is intentionally coupled here — no repository interface. This isn't
  because we don't know the pattern; it's a deliberate choice for a base
  project. Only introduce an interface at this boundary if you actually hit
  a concrete need (swapping ORMs, or unit-testing the service against a
  mocked DB) — don't add it speculatively.
- Structs throughout (`UserService`, `UserHandler`) are concrete types, never
  interfaces, and are wired by hand in `cmd/api/main.go` — no DI container.
- Routes are registered on the `/api` `*gin.RouterGroup` created in
  `main.go`, not on the root engine.
- New resources follow the same pattern: create `internal/<domain>/` with
  `model.go`, `service.go`, `handler.go`, `routes.go`, then wire
  `RegisterRoutes` into `cmd/api/main.go`.
- Keep it simple — don't introduce interfaces, DI containers, or extra layers
  unless the project actually grows to need them.

## API documentation (Swagger)

`internal/docs/` holds the Swagger/OpenAPI wiring (see
`internal/docs/SPEC.md`) — same infra exception as `health`: no
model/service, just `routes.go` mounting the Swagger UI. Every handler
method in every domain must carry `swaggo/swag` annotations
(`@Summary`/`@Tags`/`@Success`/`@Failure`/`@Router`, plus `@Security
BearerAuth` on routes behind `AuthRequired`). After adding or changing a
route's annotations, regenerate the root-level `docs/` package before
opening a PR:

```
go tool swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

The generated `docs/` folder is committed (no global `swag` install
required — it's a Go 1.24+ tool dependency in `go.mod`, run via `go tool
swag`). UI is served at `/api/docs/index.html`.

## Error handling

`internal/apierror` is the shared, cross-cutting way handlers turn an error
into an HTTP response — same infra exception as `health`/`docs`: no
model/service, just the `AppError` type and a `Respond(c, err)` helper.

- Every JSON error response shares one envelope:
  `{"error": {"code": "...", "message": "..."}}` (`apierror.Body` /
  `apierror.Detail` — also what `@Failure` Swagger annotations should
  reference, not an ad-hoc inline shape).
- `service.go` stays HTTP-agnostic: it keeps returning plain Go errors
  (sentinel vars like `ErrInvalidCredentials`/`ErrEmailTaken`, or
  `gorm.ErrRecordNotFound` as-is). It never imports `apierror` or knows
  about status codes.
- `handler.go` is the only layer that knows the HTTP mapping. For cases the
  handler can distinguish (bad input, a known domain sentinel, an
  unauthorized request), construct the response explicitly:
  `apierror.BadRequest(code, msg)`, `apierror.Unauthorized(...)`,
  `apierror.NotFound(...)`, `apierror.Conflict(...)`, then
  `apierror.Respond(c, thatError)`. For everything else, just pass the raw
  service error straight to `apierror.Respond(c, err)` — it already
  recognizes `gorm.ErrRecordNotFound` (→ 404) and falls back to a generic
  500 for anything unrecognized.
- **Never expose a raw/unexpected error's `.Error()` text to the client.**
  `apierror.Respond`'s default branch logs the real error server-side
  (`log.Printf`) and returns a generic `"internal server error"` message —
  this is deliberate (avoids leaking driver/SQL/internal detail); don't
  bypass it by hand-rolling `c.JSON(500, gin.H{"error": err.Error()})` in a
  handler.
- Unique-constraint violations (e.g. duplicate email) must be translated to
  a domain sentinel in `service.go` and mapped to `apierror.Conflict` in the
  handler — not left to fall through to a generic 500. This requires
  `gorm.Config{TranslateError: true}` wherever a `*gorm.DB` is opened
  (`database.Connect`, and any test helper that opens `:memory:` directly),
  so `errors.Is(err, gorm.ErrDuplicatedKey)` works.

## Spec-first for new domains

[BASE_SPEC.md](./BASE_SPEC.md) is the spec template for any new
domain/task. Before implementing a new `internal/<domain>/`, copy
`BASE_SPEC.md` to `internal/<domain>/SPEC.md` and fill it in (objective,
scope, data model, business rules, endpoints, error cases, acceptance
criteria, explicit non-goals). The spec's acceptance criteria mirror this
file's conventions (model/service/handler/routes + `service_test.go`), so
writing it forces the same shape as the rest of the codebase.

## Testing

No 100% coverage target — chasing that number forces tests on trivial code
(`main.go` wiring, `routes.go`) that don't catch real bugs.

- **Every domain must ship with a `service_test.go`.** A new
  `internal/<domain>/` is not done until its service layer has tests — this
  is not optional, add it in the same change that adds the domain.
- Test `service.go` in each domain: business logic + GORM calls, using
  in-memory SQLite (`:memory:`) so tests run fast with no mocks. See
  `internal/user/service_test.go` for the reference pattern (a
  `newTestService(t)` helper that opens `:memory:` + `AutoMigrate`s the
  domain's model(s), then one test function per service method/behavior).
- `handler.go` / `routes.go` don't need dedicated unit tests; cover them only
  with a light `httptest`-based integration test if a domain's HTTP wiring
  is non-trivial (custom status codes, middleware, etc.) — see
  `internal/health/handler_test.go` for that pattern.
- Use coverage as a signal to spot untested business logic, not as a target
  to hit.

## Zhu node (`cmd/node`)

Além do skeleton Gin/GORM acima, o repo abriga o full node da Zhu —
um binário standalone **sem Gin nem GORM** (storage próprio em bbolt), com
build estático `CGO_ENABLED=0`. Plano e decisões: [PLAN.md](./PLAN.md);
guia de uso: [TUTORIAL.md](./TUTORIAL.md).

- Pacotes (dependência estrita `params ← core ← pow ← chain ← {mempool,
  wallet} ← {p2p, miner} ← node ← cmd/node`): cada um com `SPEC.md` próprio
  (seção "Conceito" didática — o usuário aprende blockchain pelo projeto).
- Consenso: PoW Argon2id memory-hard (ID do bloco = SHA-256d ≠ hash de PoW),
  UTXO, retarget estilo Bitcoin, MaxBlockSize 256 KiB como regra de consenso.
- **Todo node minera por padrão** (1 worker, ~64 MiB); opt-out `-mine=false`.
  A coinbase paga a wallet do datadir (criada no primeiro `run`).
- RPC JSON localhost-only em `/rpc` para a CLI (`info`/`balance`/`send`) —
  bbolt é single-writer, a CLI nunca abre o banco de um node em execução.
- Config: flags > arquivo `zhu.conf` (chave=valor, mesmas chaves dos
  flags) > env `NODE_*` > defaults.
- Testes seguem o padrão do repo (`service_test.go`-equivalente por pacote,
  `go test -race` verde); princípio de produto: "um node em cada casa" —
  todo trade-off se decide pela régua do node doméstico.

```
CGO_ENABLED=0 go build -o bin/zhu ./cmd/node
bin/zhu run -profile devnet          # sobe o node (minera por padrão)
bin/zhu info|balance|send            # CLI via RPC localhost
bin/zhu powdemo                      # bancada didática de mineração
```

## Commands

```
go run ./cmd/api       # start the server (reads .env, defaults PORT=8080)
go build ./...         # build
go vet ./...           # vet
go test ./...          # tests (none yet)
```

## Config

Copy `.env.example` to `.env` and adjust as needed:
- `PORT` — HTTP port (default `8080`)
- `DB_DRIVER` — `sqlite` (default) or `postgres`. `database.Connect(driver,
  path, dsn)` picks the GORM dialector based on this; add a new `case` there
  (and a new driver import) if another database is needed later.
- `DB_PATH` — SQLite file path (default `data/base_project.db`), used only
  when `DB_DRIVER=sqlite`. The parent directory is created automatically by
  `database.Connect` if it doesn't exist. The `data/` dir and any `*.db`
  file are gitignored — never commit the database file.
- `DB_DSN` — Postgres connection string, used only when `DB_DRIVER=postgres`
  (e.g. `host=localhost user=postgres password=postgres dbname=base_project
  port=5432 sslmode=disable`).
- `CORS_ALLOWED_ORIGINS` — `*` (default, any origin) or a comma-separated
  allowlist. Wired via `gin-contrib/cors` in `cmd/api/main.go`
  (`corsConfig` helper); `Authorization` is explicitly in `AllowHeaders`
  since the API is JWT bearer-token based.

## Migrations

`cmd/migrate` uses [`gormigrate`](https://github.com/go-gormigrate/gormigrate)
instead of a bare `AutoMigrate` call — a plain `AutoMigrate` has no history
and no rollback, so a bad schema change in production has no way back.
`internal/database/migrations/migrations.go` holds `All`, an ordered slice
of `*gormigrate.Migration`; gormigrate tracks applied IDs in a `migrations`
table it creates itself, so `go run ./cmd/migrate` only runs what's new and
is safe to run repeatedly (idempotent).

- **Adding a migration**: append a new entry to `All` with an ID following
  `YYYYMMDDHHMMSS_description` (never reuse or edit an ID that has already
  shipped — anyone who ran it has that ID recorded as applied). `Migrate`
  and `Rollback` each take a `*gorm.DB` (the transaction) and return an
  error. For a brand-new model, `Migrate` can just call
  `tx.AutoMigrate(&yourmodel.Model{})` — the safety gormigrate adds is
  *tracking + rollback*, not replacing `AutoMigrate` as the mechanism that
  creates the table.
- **Rolling back**: `go run ./cmd/migrate -rollback` undoes only the most
  recently applied migration (`RollbackLast`).
- Never edit a migration that already ran anywhere outside local dev — add
  a new one instead, same as any other migration tool.

## Docker

`Dockerfile` is a multi-stage build producing static (`CGO_ENABLED=0`)
`api`/`migrate`/`seed` binaries on top of `alpine`. `docker-compose.yml`
runs the API against a real `postgres` service (`DB_DRIVER=postgres`) —
`migrate` runs once as a one-shot service that `api` depends on completing
before it starts. See README's Docker section for the commands.
