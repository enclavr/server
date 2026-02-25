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

1. Create a PostgreSQL database
2. Set environment variables:
   ```
   DB_HOST=localhost
   DB_PORT=5432
   DB_USER=enclavr
   DB_PASSWORD=enclavr
   DB_NAME=enclavr
   JWT_SECRET=your-secret-key
   ```
3. Run the server:
   ```bash
   go run ./cmd/server
   ```

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

## License

MIT
