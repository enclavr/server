# Enclavr Server - Agent Instructions

## Build & Test

```bash
go run ./cmd/server                           # Run server
go test -v ./...                              # Run tests
go test -v -coverprofile=coverage.out ./...   # Run with coverage
golangci-lint run ./...                       # Lint code
go build -o bin/server ./cmd/server           # Build binary
go fmt ./...                                  # Format code
go vet ./...                                  # Vet code
go mod tidy                                   # Tidy dependencies
```

## Code Style

### Go
- Use `go fmt` before committing
- Use `golangci-lint` to catch issues
- Keep functions under 50 lines
- Use meaningful variable names
- Group imports: stdlib → external → internal
- Use `errors.Wrap` or `errors.Join` for error chains
- Prefix boolean variables with `is`, `has`, `can`, `should`
- Use `const` for magic numbers
- Return early to avoid nested conditionals

### Naming Conventions
- Use camelCase for variables, functions
- Use PascalCase for types, exported functions
- Use SCREAMING_SNAKE_CASE for constants
- Prefix interfaces with "er" (e.g., `Reader`, `Writer`)

### Database
- Use GORM for ORM
- Use database transactions for multi-step operations
- Use prepared statements for repeated queries
- Always close database connections

### Security
- Never log sensitive data (passwords, tokens)
- Use constant-time comparison for secrets
- Validate all input
- Use parameterized queries

## Testing

- Use **Go's built-in testing package** (`testing`)
- **NEVER mock database** - use SQLite in-memory for tests
- **NEVER mock external services** - use real implementations or test servers
- Test with real data and real responses
- Test edge cases with actual edge case data
- Place test files next to source files (`handler.go` and `handler_test.go`)
- Use table-driven tests
- Name test functions: `Test<FunctionName>_<Scenario>`

## Directory Structure

```
cmd/server/          # Entry point
internal/
  handlers/          # HTTP handlers
  models/            # Data models
  services/          # Business logic
  config/            # Configuration
  database/          # Database connection
  websocket/         # WebSocket hub
  auth/              # Authentication
  metrics/           # Prometheus metrics
  grpc/              # gRPC server
pkg/
  middleware/        # HTTP middleware
  logger/            # Logging utilities
```

## API Design

- RESTful URLs: `/api/v1/resource`
- Use proper HTTP methods: GET, POST, PUT, DELETE
- Return JSON for all responses
- Use status codes correctly
- Version APIs: `/api/v1/`, `/api/v2/`

## Important Notes

- Keep files under 500 lines
- Use barrel patterns for packages
- Export only what's needed
- Document exported functions
- Use Go doc comments
- This project uses Go 1.25.7 (as specified in go.mod)
