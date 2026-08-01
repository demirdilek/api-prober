# ---------------------------------------------------
# Stage 1: Build-Umgebung
# ---------------------------------------------------
FROM golang:1.26-alpine AS builder

WORKDIR /app

# CA-Zertifikate & Zeitzonen installieren (wichtig für HTTPS-Aufrufe & Zeit-Funktionen in Go)
RUN apk add --no-cache ca-certificates tzdata

# Dependencies cachen (wird nur neu ausgeführt, wenn sich go.mod/go.sum ändert)
COPY go.mod go.sum ./
RUN go mod download

# Quellcode kopieren
COPY . .

# Statisch kompilierte & gestrippte Binary bauen
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o api-prober .

# ---------------------------------------------------
# Stage 2: Minimales Final-Image
# ---------------------------------------------------
FROM scratch

WORKDIR /app

# Root-Zertifikate für HTTPS-Requests kopieren
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Nur die fertig gebaute Binary aus der Builder-Stage übernehmen
COPY --from=builder /app/api-prober /app/api-prober

# Container starten
ENTRYPOINT ["/app/api-prober"]