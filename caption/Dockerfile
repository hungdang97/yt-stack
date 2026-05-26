# --- Build stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
RUN go mod download 2>/dev/null || true
COPY *.go ./
RUN go mod tidy && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/caption .

# --- Final stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app
COPY --from=builder /out/caption /app/caption

# Cache dir (mountable as volume for persistence across restarts)
RUN mkdir -p /data/cache
ENV CAPTION_CACHE_DIR=/data/cache

EXPOSE 8505
CMD ["/app/caption"]
