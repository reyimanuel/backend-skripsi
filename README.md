# Letter Administration Backend

Backend service for a campus letter/correspondence administration system.

This project provides:
- Authentication (JWT, role-based access control)
- Student registration + verification flow
- Letter templates (DOCX) with `{{placeholder}}` detection and validation
- Letter lifecycle: draft → submitted → forwarded → approved/rejected
- PDF previews generated from DOCX using LibreOffice (headless)
- Attachment uploads and requirements per letter type
- Notifications via Firebase Cloud Messaging (FCM)

## Tech Stack

- Go (see [go.mod](go.mod))
- Gin (HTTP router)
- GORM + PostgreSQL
- JWT (RS256)
- LibreOffice (DOCX → PDF conversion)
- Firebase Admin SDK (FCM)

## Project Structure (high level)

- [cmd/server/main.go](cmd/server/main.go) — application entrypoint (server + CLI)
- [internal/server/server.go](internal/server/server.go) — HTTP server bootstrap
- [internal/api/](internal/api/) — route registrations + handlers/services per module
  - users, letters, correspondence, notifications
- [internal/infrastructures/config/](internal/infrastructures/config/) — configuration from environment variables
- [internal/infrastructures/database/](internal/infrastructures/database/) — DB connection
- [internal/migration/](internal/migration/) — AutoMigrate + seed utilities
- [docs/openapi.yaml](docs/openapi.yaml) — OpenAPI specification
- `public/` — served statically at `/public`
  - `public/generated/` — generated files (attachments, etc.)
  - `public/images/` — profile photos, signatures, student cards
  - `public/letter-template/` — example templates

## Requirements

- Go version matching [go.mod](go.mod)
- PostgreSQL
- LibreOffice
  - Required for PDF preview generation (DOCX → PDF)
  - On Windows you can install LibreOffice normally, or set `LIBREOFFICE_BIN` to the `soffice(.exe/.com)` path
- Firebase service account JSON (for FCM)
- RSA key pair (PEM) for JWT (RS256)

## Configuration (Environment Variables)

The service loads configuration from `.env` (if present) and the OS environment.

### Server

- `PORT` (default: `8080`)
- `IS_PRODUCTION` (`true|false`) — enables Gin release mode
- `BASE_URL` (default: `http://localhost:<PORT>`) — used to generate absolute URLs
- `FRONTEND_URL` (default: `http://localhost:3000`)

### Database (required)

- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PASS`
- `DB_NAME`
- `DB_TIME_ZONE`

### JWT keys (required)

- `PRIVATE_KEY` — path to private key PEM file (must end with `.pem`)
- `PUBLIC_KEY` — path to public key PEM file (must end with `.pem`)

### Token lifetime (optional)

- `ACCESS_TOKEN_LIFE_TIME` — seconds (default: `3600`)
- `REFRESH_TOKEN_LIFE_TIME` — seconds (default: `86400`)

### Email provider (optional)

- `EMAIL_PROVIDER` — defaults to `smtp`

If `EMAIL_PROVIDER=smtp`:
- `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`
- `SMTP_SENDER_EMAIL` (optional; defaults to `SMTP_USER`)
- `SMTP_SENDER_NAME` (optional)

If `EMAIL_PROVIDER=sendgrid` (or when using SendGrid in code):
- `SENDGRID_API_KEY`, `SENDGRID_SENDER_EMAIL`, `SENDGRID_SENDER_NAME`

### Notifications (required)

- `FIREBASE_CREDENTIALS` — path to Firebase service account JSON

### LibreOffice (optional)

- `LIBREOFFICE_BIN` — absolute path to LibreOffice `soffice` binary (overrides auto-detection)
- `LIBREOFFICE_PATH` — alias for `LIBREOFFICE_BIN`

### Example `.env`

```env
# Server
PORT=8080
IS_PRODUCTION=false
BASE_URL=http://localhost:8080
FRONTEND_URL=http://localhost:3000

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASS=postgres
DB_NAME=letter_administration
DB_TIME_ZONE=Asia/Makassar

# JWT keys (paths)
PRIVATE_KEY=./keys/private_key.pem
PUBLIC_KEY=./keys/public_key.pem

# Firebase
FIREBASE_CREDENTIALS=./keys/firebase.json

# Optional
ACCESS_TOKEN_LIFE_TIME=3600
REFRESH_TOKEN_LIFE_TIME=86400
EMAIL_PROVIDER=smtp
SMTP_HOST=smtp.example.com
SMTP_PORT=587
SMTP_USER=user@example.com
SMTP_PASS=your_password
```

## Running Locally

1) Install dependencies

```bash
go mod download
```

2) Prepare configuration
- Create `.env` (or export environment variables)
- Ensure `PRIVATE_KEY`, `PUBLIC_KEY`, and `FIREBASE_CREDENTIALS` point to real files

3) Run migrations (schema + seed)

```bash
go run ./cmd/server migrate
```

4) Start the HTTP server

```bash
go run ./cmd/server
```

The API is mounted under `/api`, and static files are served under `/public`.

## CLI Commands (Migrations / Seed / Reset)

The binary supports CLI mode (see [cmd/server/main.go](cmd/server/main.go)):

- Migrate + seed:
  ```bash
  go run ./cmd/server migrate
  ```

- Migrate only (no seeding):
  ```bash
  go run ./cmd/server migrate-only
  ```

- Seed only:
  ```bash
  go run ./cmd/server seed --only=users
  go run ./cmd/server seed --only=templates
  go run ./cmd/server seed --only=all
  ```

- Reset DB (drop + migrate + seed):
  ```bash
  go run ./cmd/server reset
  ```

### Safety notes
- `seed` and `reset` are blocked when `DB_HOST` is not local, unless you pass `--force`.
- `seed --truncate-all` will TRUNCATE multiple tables (dev-only, destructive).

## Default Seed Accounts (development)

When seeding runs, these accounts are ensured (password: `password`):

- Admin: `admin@kampus.ac.id`
- Dekan: `dekan@kampus.ac.id`
- Wakil Dekan: `wakildekan@kampus.ac.id`
- Mahasiswa: `mahasiswa@test.ac.id`

Do not use these credentials in production.

## Authentication & Authorization

- Use `Authorization: Bearer <access_token>` (the middleware also accepts a raw token for convenience).
- Roles used by the system:
  - `ADMIN`
  - `MAHASISWA`
  - `DEKAN`
  - `WAKIL_DEKAN`

Verification rules summary is documented in [docs/authorization-verification-rules.md](docs/authorization-verification-rules.md).

## Templates & Placeholders

Letter templates are uploaded as `.docx`. Placeholders use the format:

- `{{key}}`

During template upload, the backend analyzes placeholders and determines:
- Which keys can be auto-filled (e.g., student identity fields)
- Which keys must be provided in the request payload

At draft creation/update and submission time, required keys are validated. The system also performs a post-generation scan to ensure no `{{...}}` tokens remain in the generated document.

## API Documentation

- OpenAPI spec: [docs/openapi.yaml](docs/openapi.yaml)
- Base path: `/api`

Notes:
- Most endpoints return a JSON envelope: `{ "status_code": number, "message": string, "data"?: any }`
- Preview endpoints return `application/pdf`.

## Docker

This repository includes a production-oriented Docker image that bundles LibreOffice for DOCX → PDF conversion.

- [Dockerfile](Dockerfile) builds a static Go binary and installs LibreOffice in the runtime image.
- [deploy/entrypoint.sh](deploy/entrypoint.sh) can materialize secret files from environment variables:
  - `PRIVATE_KEY_PEM_B64` or `PRIVATE_KEY_PEM` → writes to `PRIVATE_KEY`
  - `PUBLIC_KEY_PEM_B64` or `PUBLIC_KEY_PEM` → writes to `PUBLIC_KEY`
  - `FIREBASE_CREDENTIALS_JSON_B64` or `FIREBASE_CREDENTIALS_JSON` → writes to `FIREBASE_CREDENTIALS`

Build:

```bash
docker build -t letter-administration-backend .
```

Run (example):

```bash
docker run --rm -p 8080:8080 \
  -e PORT=8080 \
  -e DB_HOST=host.docker.internal -e DB_PORT=5432 -e DB_USER=postgres -e DB_PASS=postgres -e DB_NAME=letter_administration -e DB_TIME_ZONE=Asia/Makassar \
  -e BASE_URL=http://localhost:8080 \
  -e PRIVATE_KEY=/app/keys/private_key.pem \
  -e PUBLIC_KEY=/app/keys/public_key.pem \
  -e FIREBASE_CREDENTIALS=/app/keys/firebase.json \
  -e PRIVATE_KEY_PEM_B64=... \
  -e PUBLIC_KEY_PEM_B64=... \
  -e FIREBASE_CREDENTIALS_JSON_B64=... \
  letter-administration-backend
```

If you want to run migrations inside the container, run the binary with CLI args, e.g.:

```bash
docker run --rm \
  -e ...same env... \
  letter-administration-backend /app/app migrate --force
```

## Development

Run tests:

```bash
go test ./...
```

## License

Not specified.
