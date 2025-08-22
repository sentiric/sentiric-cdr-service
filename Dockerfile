# --- İNŞA AŞAMASI (DEBIAN TABANLI) ---
FROM golang:1.24-bullseye AS builder

RUN apt-get update && apt-get install -y --no-install-recommends git build-essential

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

RUN go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/sentiric-cdr-service ./cmd/cdr-service

# --- ÇALIŞTIRMA AŞAMASI (ALPINE) ---
FROM alpine:latest

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /app/bin/sentiric-cdr-service .

ENTRYPOINT ["./sentiric-cdr-service"]