# Security Policy - Server

This document covers the security policy for the Enclavr server (Go backend).

## Reporting a Vulnerability

**Do NOT report security vulnerabilities through public GitHub issues.**

- **GitHub Private Vulnerability Reporting:** Use the ["Report a vulnerability"](https://github.com/enclavr/server/security/advisories/new) button in the Security tab.
- **Email:** Send details to `enclavr.dev@gmail.com`

For cross-cutting concerns affecting multiple components, report to the [root repository](https://github.com/enclavr/enclavr/security/advisories/new).

### What to Include

- Description of the vulnerability
- Affected endpoints, handlers, or services
- Steps to reproduce with request/response examples
- Potential impact (data exposure, privilege escalation, injection, DoS, etc.)
- Suggested fix (if any)

### Response Timeline

| Stage | Timeline |
|-------|----------|
| Acknowledgement | Within 48 hours |
| Initial triage | Within 5 business days |
| Fix/patch target | 30 days (varies by severity) |

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Previous release | Security fixes only |
| Older releases | No |

## Security Considerations

The server is the primary attack surface and handles:

- **Authentication & Authorization:** JWT with configurable expiry, bcrypt password hashing, TOTP 2FA with AES-256-GCM encrypted secrets, OIDC/WebAuthn support, refresh token rotation with family detection
- **Injection Attacks:** Parameterized queries via GORM ORM, input validation on all endpoints, prepared statements for raw SQL
- **Data Exposure:** Field-level encryption for sensitive data (2FA secrets), minimal data in API responses, no sensitive data in logs
- **Rate Limiting:** Per-user, per-IP, per-endpoint rate limiting using sliding window algorithm with Redis-backed distributed counters
- **WebSocket Security:** Authenticated WebSocket connections, message validation, connection limits per user
- **Denial of Service:** Request size limits, connection timeouts, circuit breakers, request deduplication
- **API Security:** CORS configuration, security headers (CSP, HSTS, X-Frame-Options), API versioning
- **Database Security:** Connection pooling with TLS, parameterized queries, migration versioning
- **Encryption at Rest:** AES-256-GCM for sensitive fields, secure key management via environment variables
- **Webhook Security:** HMAC-SHA256 signature verification for outgoing webhooks

## Security Features

- Security headers middleware (CSP, HSTS, X-Frame-Options, X-XSS-Protection, Referrer-Policy, Permissions-Policy)
- Distributed rate limiting with Redis
- Password policy enforcement (strength, history, expiry, lockout)
- Account lockout after failed login attempts
- Token rotation with refresh token family detection
- Prometheus metrics for security events monitoring
- Structured logging without sensitive data exposure
- gRPC with authentication middleware

## Dependency Security

- Automated dependency updates via Dependabot
- Go module checksum verification
- Regular security audits of dependencies

## Disclosure Policy

See the [root repository SECURITY.md](https://github.com/enclavr/enclavr/blob/main/SECURITY.md) for the full disclosure policy.

## Safe Harbor

We support safe harbor for security researchers who follow responsible disclosure practices. See the [root repository SECURITY.md](https://github.com/enclavr/enclavr/blob/main/SECURITY.md) for details.
