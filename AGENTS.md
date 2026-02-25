# Server Agent

## Role
Manages the Go backend server for Enclavr voice chat platform.

## Tasks
- Implement API endpoints
- Maintain database models
- Handle authentication
- Manage WebSocket connections for voice
- Ensure code quality and testing

## IMPORTANT: CLI Tools

This project uses specific CLI tools. Use ONLY these:

| Task | Command |
|------|---------|
| Run server | `go run ./cmd/server` |
| Run tests | `go test -v ./...` |
| Run tests with coverage | `go test -v -coverprofile=coverage.out ./...` |
| Lint code | `golangci-lint run ./...` |
| Build binary | `go build -o bin/server ./cmd/server` |
| Format code | `go fmt ./...` |
| Vet code | `go vet ./...` |
| Update dependencies | `go get -u ./...` |
| Tidy dependencies | `go mod tidy` |

**NEVER use other build tools or test frameworks. Use Go's built-in testing only.**

## Guidelines

### Go
- Use Go 1.25.7 (as specified in go.mod)
- Use standard library where possible
- Use structs for request/response bodies
- Use context.Context for request-scoped values
- Return proper HTTP status codes

### Error Handling
- Always handle errors explicitly
- Log errors with appropriate level
- Return user-friendly error messages
- Use custom error types for domain errors

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

## Coding Style (Strict)

### TypeScript (N/A - Go only)
- N/A

### Go
- Use `go fmt` before committing
- Use `golangci-lint` to catch issues
- Keep functions small (< 50 lines)
- Use meaningful variable names
- Group imports: stdlib → external → internal
- Use `errors.Wrap` or `errors.Join` for error chains
- Prefix boolean variables with `is`, `has`, `can`, `should`
- Use `const` for magic numbers
- Return early to avoid nested conditionals

### Naming
- Use camelCase for variables, functions
- Use PascalCase for types, exported functions
- Use SCREAMING_SNAKE_CASE for constants
- Prefix interfaces with "er" (e.g., `Reader`, `Writer`)

### Components/Modules
- Handlers in `internal/handlers/`
- Models in `internal/models/`
- Services in `internal/services/`
- Configuration in `internal/config/`
- Middleware in `pkg/middleware/`
- Database in `internal/database/`
- WebSocket in `internal/websocket/`

### Testing
- Use **Go's built-in testing package** (`testing`)
- **NEVER mock database** - use SQLite in-memory for tests
- **NEVER mock external services** - use real implementations or test servers
- Test with real data and real responses
- Test edge cases with actual edge case data
- Place test files next to source files (`handler.go` and `handler_test.go`)
- Use table-driven tests
- Name test functions: `Test<FunctionName>_<Scenario>`

## Testing

### Running Tests
```bash
go test -v ./...
go test -v -run TestAuthHandler ./...
go test -v -coverprofile=coverage.out ./...
```

### Linting
```bash
golangci-lint run ./...
golangci-lint run --fix ./...  # auto-fix issues
```

### Test Structure
```go
func TestHandler_Method(t *testing.T) {
    tests := []struct {
        name           string
        input          InputType
        expectedOutput OutputType
        expectedError  error
    }{
        // table-driven tests
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // actual test logic
        })
    }
}
```

## Conventions

### Directory Structure
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

### API Design
- RESTful URLs: `/api/v1/resource`
- Use proper HTTP methods: GET, POST, PUT, DELETE
- Return JSON for all responses
- Use status codes correctly
- Version APIs: `/api/v1/`, `/api/v2/`

## Code Organization

- Keep files under 500 lines
- Use barrel patterns for packages
- Export only what's needed
- Document exported functions
- Use Go doc comments
