# Enclavr Server

The backend server for Enclavr, a self-hosted voice chat platform.

## Features

- User authentication with JWT
- Room-based voice chat
- WebSocket-based real-time communication
- PostgreSQL database
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

## License

MIT
