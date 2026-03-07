FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o server ./cmd/server

FROM alpine:latest

# Create non-root user for security
RUN addgroup -g 1000 enclavr && adduser -u 1000 -G enclavr -s /bin/sh -D enclavr

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary and config
COPY --from=builder /app/server .
COPY --from=builder /app/internal/config/config.go ./internal/config/

# Create data directory for uploads
RUN mkdir -p uploads && chown -R enclavr:enclavr /app/uploads

USER enclavr

ENV SERVER_PORT=8080
ENV DB_HOST=postgres
ENV DB_PORT=5432
ENV DB_USER=enclavr
ENV DB_PASSWORD=enclavr
ENV DB_NAME=enclavr

EXPOSE 8080

CMD ["./server"]
