# --- İNŞA AŞAMASI ---
FROM golang:1.24.5-alpine AS builder

RUN apk add --no-cache git build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

COPY . .

ARG SERVICE_NAME=sentiric-cdr-service
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/bin/${SERVICE_NAME} ./cmd/cdr-service

# --- ÇALIŞTIRMA AŞAMASI ---
FROM alpine:latest

# TLS doğrulaması için ca-certificates gerekli
RUN apk add --no-cache ca-certificates

ARG SERVICE_NAME=sentiric-cdr-service
WORKDIR /app

COPY --from=builder /app/bin/${SERVICE_NAME} .

ENTRYPOINT ["./sentiric-cdr-service"]