# Enclavr Server

The backend server for Enclavr, a self-hosted voice chat platform.

## IMPORTANT: CLI Commands

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

## Features

- User authentication with JWT
- Room-based voice chat
- WebSocket-based real-time communication
- PostgreSQL database
- Redis pub/sub for horizontal scaling
- Prometheus metrics
- gRPC API support
- Docker support

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
docker-compose up -d
```

### Manual Setup

1. Create a PostgreSQL database (local or Neon)
2. Set environment variables:
   - For local PostgreSQL: copy `.env.example` to `.env`
   - For Neon PostgreSQL 17 (free tier): copy `.env.neon` to `.env` and sign up at https://neon.tech
   ```
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=enclavr
   DB_PASSWORD=enclavr
   DB_NAME=enclavr
   DB_SSLMODE=disable  # use 'require' for Neon
   JWT_SECRET=your-secret-key
   ```
3. Run the server:
   ```bash
   go run ./cmd/server
   ```

### Neon PostgreSQL (Free Tier)

For testing without setting up local PostgreSQL:
1. Sign up at https://neon.tech (free: 0.5GB storage, 1 branch)
2. Copy `server/.env.neon` to `server/.env`
3. Run the server - migrations run automatically

Note: The server uses GORM with standard PostgreSQL driver, so it works with any PostgreSQL provider (Neon, Supabase, self-hosted, etc.).

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| SERVER_PORT | 8080 | Server port |
| DB_HOST | localhost | Database host |
| DB_PORT | 5432 | Database port |
| DB_USER | enclavr | Database user |
| DB_PASSWORD | enclavr | Database password |
| DB_NAME | enclavr | Database name |
| DB_SSLMODE | disable | SSL mode |
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

## Testing

Run tests with real SQLite in-memory database:

```bash
go test -v ./...
go test -v -run TestAuthHandler ./...
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
