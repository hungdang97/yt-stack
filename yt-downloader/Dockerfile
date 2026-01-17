# Multi-stage build for optimal image size
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build optimized binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o yt-downloader-go .

# Final stage - minimal runtime image
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies (yt-dlp, ffmpeg)
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    python3 \
    py3-pip \
    ffmpeg \
    wget \
    && pip3 install --break-system-packages --no-cache-dir yt-dlp \
    && rm -rf /var/cache/apk/*

# Copy binary from builder
COPY --from=builder /build/yt-downloader-go .

# Create storage directory
RUN mkdir -p /app/storage

# Expose port
EXPOSE 5001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5001/health || exit 1

# Run as non-root user
RUN adduser -D -u 1000 appuser && chown -R appuser:appuser /app
USER appuser

CMD ["./yt-downloader-go"]
