# Contributing to Enclavr Server

Thank you for your interest in contributing to Enclavr Server.

## Getting Started

1. Fork and clone the repository
2. Install Go (see `go.mod` for required version)
3. Set up PostgreSQL (see `README.md` for setup instructions)
4. Run `go mod download` to install dependencies

## Development Guidelines

- Follow standard Go conventions and idioms
- Use `gofmt` or `goimports` to format your code
- Write tests for new functionality
- Keep functions focused and small
- Use meaningful variable and function names

## Before Submitting a PR

Run the following commands and ensure they pass:

```bash
golangci-lint run ./...
go test -v ./...
```

Fix any lint errors or test failures before opening a pull request.

## Code Style

- Follow existing code patterns in the repository
- Use Go standard project layout conventions
- Add comments for exported types and functions
- Handle errors explicitly; don't discard them

## Pull Requests

- Keep PRs focused on a single change
- Write a clear description of what the PR does and why
- Reference any related issues
- Ensure CI passes before requesting review

## Reporting Issues

Use the GitHub issue templates to report bugs or request features.

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.
