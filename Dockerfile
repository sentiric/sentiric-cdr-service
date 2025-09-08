# --- İNŞA AŞAMASI (DEBIAN TABANLI) ---
FROM golang:1.24-bullseye AS builder

ARG GIT_COMMIT="unknown"
ARG BUILD_DATE="unknown"
ARG SERVICE_VERSION="0.0.0"

RUN apt-get update && apt-get install -y --no-install-recommends git build-essential

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE} -X main.ServiceVersion=${SERVICE_VERSION} -w -s" \
    -o /app/bin/sentiric-cdr-service ./cmd/cdr-service

# --- ÇALIŞTIRMA AŞAMASI (DEBIAN SLIM) ---
# DEĞİŞİKLİK: Alpine yerine Debian Slim kullanarak platform standardını sağlıyoruz.
FROM debian:bookworm-slim

# TLS doğrulaması için ca-certificates gerekli
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

# GÜVENLİK: Root olmayan bir kullanıcı oluştur
RUN addgroup --system --gid 1001 appgroup && \
    adduser --system --no-create-home --uid 1001 --ingroup appgroup appuser

WORKDIR /app

# Dosyaları kopyala ve sahipliği yeni kullanıcıya ver
COPY --from=builder /app/bin/sentiric-cdr-service .
RUN chown appuser:appgroup ./sentiric-cdr-service

# GÜVENLİK: Kullanıcıyı değiştir
USER appuser

ENTRYPOINT ["./sentiric-cdr-service"]