# Server Agent

## Role
Manages the Go backend server for Enclavr voice chat platform.

## Tasks
- Implement API endpoints
- Maintain database models
- Handle authentication
- Manage WebSocket connections for voice

## Guidelines
- Use clean architecture
- Follow Go best practices
- Write unit tests for handlers
- Use golangci-lint for linting

## Conventions
- Handlers in `internal/handlers/`
- Models in `internal/models/`
- Services in `internal/services/`
- Configuration in `internal/config/`

## Testing
Run tests with: `go test ./...`
Run linter with: `golangci-lint run`
