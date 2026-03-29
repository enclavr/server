# Build stage
FROM golang:1.25.7-alpine AS builder

# Metadata labels
LABEL org.opencontainers.image.title="Enclavr Server" \
      org.opencontainers.image.description="Go backend API server for Enclavr voice chat platform" \
      org.opencontainers.image.vendor="Enclavr" \
      org.opencontainers.image.licenses="Unlicense"

WORKDIR /app

# Copy dependency files first (for better layer caching)
COPY go.mod go.sum ./

# Copy source code
COPY . .

# Install build dependencies and build app
RUN apk add --no-cache --virtual .build-deps gcc musl-dev && \
    go mod download && \
    CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server && \
    apk del .build-deps

# Runtime stage
FROM alpine:3.21

# Metadata labels for runtime image
LABEL org.opencontainers.image.title="Enclavr Server Runtime" \
      org.opencontainers.image.description="Runtime environment for Enclavr server" \
      org.opencontainers.image.vendor="Enclavr" \
      org.opencontainers.image.licenses="Unlicense"

# Create non-root user with explicit UID/GID
RUN addgroup -g 1000 enclavr && \
    adduser -u 1000 -G enclavr -s /bin/sh -D enclavr && \
    mkdir -p /app/uploads

# Install only runtime dependencies (ca-certificates for TLS, tzdata for timezone support)
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /server /app/server

# Create uploads directory and set permissions
RUN mkdir -p /app/uploads && \
    chown -R enclavr:enclavr /app/uploads && \
    chmod 755 /app/uploads

# Switch to non-root user
USER enclavr

# Default environment variables (can be overridden by docker-compose)
ENV SERVER_PORT=8080
EXPOSE 8080

# Run the server
CMD ["./server"]