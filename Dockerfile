# --- İNŞA AŞAMASI (DEBIAN TABANLI) ---
FROM golang:1.24-bullseye AS builder

# YENİ: Build argümanlarını build aşamasında kullanılabilir yap
ARG GIT_COMMIT="unknown"
ARG BUILD_DATE="unknown"
ARG SERVICE_VERSION="0.0.0"

RUN apt-get update && apt-get install -y --no-install-recommends git build-essential

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . .

# GÜNCELLEME: ldflags ile build-time değişkenlerini Go binary'sine göm
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE} -X main.ServiceVersion=${SERVICE_VERSION} -w -s" \
    -o /app/bin/sentiric-cdr-service ./cmd/cdr-service

# --- ÇALIŞTIRMA AŞAMASI (ALPINE) ---
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/sentiric-cdr-service .

ENTRYPOINT ["./sentiric-cdr-service"]