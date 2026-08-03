# ---------------------------------------------------
# Stage 1: Build environment
# ---------------------------------------------------
FROM golang:alpine AS builder

WORKDIR /app

# Install CA certificates for outgoing HTTPS requests
RUN apk add --no-cache ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically compiled and stripped binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o kube-prober .

# ---------------------------------------------------
# Stage 2: Minimal runtime image (~18 MB)
# ---------------------------------------------------
FROM scratch

WORKDIR /app

# Copy root CA certificates for TLS verification
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled binary
COPY --from=builder /app/kube-prober /app/kube-prober

# Run application
ENTRYPOINT ["/app/kube-prober"]