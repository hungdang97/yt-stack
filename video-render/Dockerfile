# --- Build stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /src

# Cache deps first if go.sum exists; otherwise just copy go.mod.
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/video-render .

# --- Final stage ---
# Alpine's ffmpeg ships with libass + libx264 + AAC — đủ cho subtitle burn-in.
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/video-render /app/video-render
COPY index.html /app/index.html

EXPOSE 8501
CMD ["/app/video-render"]
