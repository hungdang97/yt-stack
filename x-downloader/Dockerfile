# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true

# Copy source
COPY . .

# Tidy and build
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -o x-downloader .

# Run stage - includes FFmpeg for audio extraction
FROM alpine:3.19

WORKDIR /app

# Install CA certificates, wget for healthcheck, and FFmpeg for audio extraction
RUN apk add --no-cache ca-certificates wget ffmpeg

# Copy binary
COPY --from=builder /build/x-downloader .

# Create storage directory
RUN mkdir -p /app/storage

EXPOSE 5005

CMD ["./x-downloader"]
