# ---------------------------------------------------
# Stage 1: Build environment
# ---------------------------------------------------
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install CA certificates for outgoing HTTPS requests (Pushover, webhooks)
RUN apk add --no-cache ca-certificates

# Cache dependencies (re-run only when go.mod or go.sum changes)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically compiled and stripped binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o api-prober .

# ---------------------------------------------------
# Stage 2: Minimal runtime image (~18 MB)
# ---------------------------------------------------
FROM scratch

WORKDIR /app

# Copy root CA certificates for TLS verification
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled binary
COPY --from=builder /app/api-prober /app/api-prober

# Run application
ENTRYPOINT ["/app/api-prober"]