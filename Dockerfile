# =============================================================================
# Build stage
# =============================================================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies first to leverage Docker layer cache.
COPY go.mod go.sum ./
RUN go mod download

# Copy source code and build the binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /auth-service ./cmd/server/

# =============================================================================
# Runtime stage
# =============================================================================
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy only the compiled binary and required runtime files.
COPY --from=builder /auth-service .
COPY config.yaml .
COPY migrations/ ./migrations/

# RSA keys are never baked into the image. They are mounted at runtime via
# the Docker volume ./keys:/app/keys:ro (see docker-compose.yml).
# The /app/keys directory is created here as an empty placeholder.
RUN mkdir -p /app/keys

EXPOSE 8080

ENTRYPOINT ["./auth-service"]
