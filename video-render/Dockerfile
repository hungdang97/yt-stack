# --- Build stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /src

# Cache deps first if go.sum exists; otherwise just copy go.mod.
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/video-render .

# --- Final stage ---
# Alpine's ffmpeg ships with libass + libx264 + AAC, but the image has NO
# fonts by default. libass needs real font files on disk: without them,
# ASS subtitles render blank or as tofu boxes (Vietnamese diacritics +
# emoji get hit hardest). We install:
#   - ttf-liberation : Arial-metric-compatible (Liberation Sans), so ASS
#                      styles that ask for "Arial" find a working face.
#   - font-noto      : Noto Sans for broad Latin + Vietnamese coverage.
#   - font-noto-cjk  : Chinese / Japanese / Korean glyphs.
#   - font-noto-emoji: Color emoji (🎬, 🙏, 🎉 ...) used in subtitle text.
# fc-cache rebuilds fontconfig's index so libass can find them.
FROM alpine:3.20
RUN apk add --no-cache \
        ffmpeg \
        ca-certificates \
        tzdata \
        fontconfig \
        ttf-liberation \
        font-noto \
        font-noto-cjk \
        font-noto-emoji \
    && fc-cache -f

WORKDIR /app
COPY --from=builder /out/video-render /app/video-render
COPY index.html /app/index.html

EXPOSE 8501
CMD ["/app/video-render"]
