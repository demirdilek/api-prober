# Stage 1: Build
FROM --platform=$BUILDPLATFORM golang:latest AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Automatic BuildKit architecture variables
ARG TARGETOS
ARG TARGETARCH

# Fallback to local host architecture if TARGETARCH is empty
RUN GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} CGO_ENABLED=0 go build -ldflags="-w -s" -o api-prober .

# Stage 2: Final minimal image
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/api-prober .

EXPOSE 8080

CMD ["./api-prober"]