# Enclavr Server

The backend server for Enclavr, a self-hosted voice chat platform.

## Error Monitoring (Sentry)

The Go server is integrated with Sentry for production error tracking and performance monitoring.

### Configuration

Set the `SENTRY_DSN` environment variable to enable Sentry:

```bash
# In your .env file
SENTRY_DSN=https://[email]@sentry.io/[project-id]
```

### Features

- User authentication with JWT
- Room-based voice chat
- WebSocket-based real-time communication
- PostgreSQL database
- Redis pub/sub for horizontal scaling
- Prometheus metrics
- gRPC API support
- Docker support
- Sentry error tracking and performance monitoring

## API Endpoints

### Authentication
- `POST /api/auth/register` - Register a new user
- `POST /api/auth/login` - Login
- `POST /api/auth/refresh` - Refresh token
- `GET /api/auth/me` - Get current user

### Rooms
- `GET /api/rooms` - List all rooms
- `POST /api/room/create` - Create a room
- `GET /api/room?id=<room_id>` - Get room details
- `POST /api/room/join` - Join a room
- `POST /api/room/leave` - Leave a room

### Voice
- `GET /api/voice/ws?room_id=<room_id>` - WebSocket for voice
- `GET /api/voice/ice` - Get ICE server configuration

## Quick Start

### Using Docker Compose

```bash
cd infra
docker compose up -d
```

### Manual Setup

1. Copy `.env.example` to `.env`
2. Configure database (Neon is default, or use self-hosted PostgreSQL):

**Option 1: Neon PostgreSQL (default)** - Sign up at https://neon.tech
```bash
NEON_CONNECTION_STRING=postgres://user:password@host.neon.tech/neondb?sslmode=require
```

**Option 2: Self-hosted PostgreSQL**
```bash
NEON_CONNECTION_STRING=  # Empty to use self-hosted
DB_HOST=localhost
DB_PORT=5432
DB_USER=enclavr
DB_PASSWORD=enclavr
DB_NAME=enclavr
DB_SSLMODE=disable
```

3. Run the server:
   ```bash
   go run ./cmd/server
   ```

Note: The server uses GORM with standard PostgreSQL driver, so it works with any PostgreSQL provider (Neon, Supabase, self-hosted, etc.).

## CLI Commands

This project uses Go's standard tooling. Use these commands:

```bash
# Install dependencies
go mod tidy

# Run server
go run ./cmd/server

# Run tests
go test -v ./...

# Run with coverage
go test -v -coverprofile=coverage.out ./...

# Lint code
golangci-lint run ./...

# Build binary
go build -o bin/server ./cmd/server

# Format code
go fmt ./...
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_PORT | 8080 | Server port |
| NEON_CONNECTION_STRING | - | Neon PostgreSQL connection string (when set, takes priority) |
| DB_HOST | localhost | Database host (used when NEON_CONNECTION_STRING is empty) |
| DB_PORT | 5432 | Database port |
| DB_USER | enclavr | Database user |
| DB_PASSWORD | enclavr | Database password |
| DB_NAME | enclavr | Database name |
| DB_SSLMODE | disable | SSL mode (use 'require' for Neon) |
| JWT_SECRET | - | JWT signing secret |
| JWT_EXPIRATION | 24h | Token expiration |
| REFRESH_EXPIRATION | 7d | Refresh token expiration |
| STUN_SERVER | stun:stun.l.google.com:19302 | STUN server |
| TURN_SERVER | - | TURN server |
| TURN_USER | - | TURN username |
| TURN_PASS | - | TURN password |
| REDIS_HOST | localhost | Redis host |
| REDIS_PORT | 6379 | Redis port |
| REDIS_PASSWORD | - | Redis password |
| REDIS_DB | 0 | Redis database |
| SENTRY_DSN | - | Sentry DSN for error tracking |

### Admin User Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| ADMIN_USERNAME | admin | Admin username |
| ADMIN_PASSWORD | - | Admin password (if set, creates default admin) |
| ADMIN_EMAIL | admin@enclavr.local | Admin email |
| FIRST_USER_IS_ADMIN | true | First registered user becomes admin |

**Setup:**
```bash
# Option 1: Set admin password to create default admin on startup
ADMIN_PASSWORD=your-secure-password

# Option 2: First user to register becomes admin (default enabled)
# Just start the server and register a user
```

## Testing

Run tests (defaults to SQLite for local, uses Neon in CI):

```bash
# Run all tests
go test -v ./...

# Run specific test
go test -v -run TestAuthHandler ./...

# Run with Neon PostgreSQL
NEON_CONNECTION_STRING=postgres://... go test -v ./...
```

Run with coverage:

```bash
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Linting

```bash
golangci-lint run ./...
```

## Refactoring

When the codebase grows too large (>50 files in a module), refactor into smaller sub-modules:
- Create new Go modules using `go mod init` and `go mod tidy`
- Maintain clean boundaries between sub-modules
- Push new sub-module repositories to GitHub and link them in parent repo
- Update this README with new sub-modules

## License

MIT
