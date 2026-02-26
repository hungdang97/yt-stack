# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

# Copy source
COPY . .

# Tidy and build
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -o tik-downloader .

# Run stage - minimal image (no FFmpeg needed!)
FROM alpine:3.19

WORKDIR /app

# Install CA certificates for HTTPS requests
RUN apk add --no-cache ca-certificates wget

# Copy binary
COPY --from=builder /build/tik-downloader .

# Create storage directory
RUN mkdir -p /app/storage

EXPOSE 5002

CMD ["./tik-downloader"]
