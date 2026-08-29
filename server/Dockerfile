# Build stage
FROM golang:1.25.13-alpine AS builder

RUN apk add --no-cache git ca-certificates "zlib>=1.3.2-r0"

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -mod=readonly -trimpath -ldflags="-w -s" -o /out/main main.go && \
    go clean -cache -modcache

# Runtime stage
FROM alpine:3.22

RUN apk upgrade --no-cache && \
    apk add --no-cache ca-certificates tzdata && \
    adduser -D -h /home/appuser appuser && \
    mkdir -p /app/logs /app/config /app/resources && \
    chown -R appuser:appuser /app

WORKDIR /app

COPY --from=builder --chown=appuser:appuser /out/main /app/main
COPY --from=builder --chown=appuser:appuser /app/config/ /app/config/
COPY --from=builder --chown=appuser:appuser /app/resources/ /app/resources/

USER appuser

CMD ["/app/main"]
