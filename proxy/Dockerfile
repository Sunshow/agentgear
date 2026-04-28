# Build stage
FROM golang:alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o agentgear ./cmd/agentgear/

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates tzdata wget curl

# Create non-root user
RUN adduser -D -u 1000 agentgear

# Copy binary from builder
COPY --from=builder /app/agentgear .

# Copy default config
COPY --from=builder /app/configs/config.example.yaml ./configs/config.yaml

# Create directories
RUN mkdir -p /app/logs /app/data && chown -R agentgear:agentgear /app

# Switch to non-root user
USER agentgear

# Default environment variables
ENV AGENTGEAR_SERVER_PORT=9000 \
    AGENTGEAR_SERVER_HOST=0.0.0.0 \
    AGENTGEAR_SERVER_API_PORT=9001 \
    AGENTGEAR_SERVER_API_HOST=0.0.0.0 \
    AGENTGEAR_LOGGING_ENABLED=true \
    AGENTGEAR_LOGGING_DIR=/app/logs \
    AGENTGEAR_MEMORY_MAX_CONNECTIONS=1000 \
    AGENTGEAR_MEMORY_RETENTION_MINUTES=60

EXPOSE 9000 9001

ENTRYPOINT ["./agentgear"]
