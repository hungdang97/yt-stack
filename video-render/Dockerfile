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
# Plus 27 Google Fonts hay dùng trong template subtitle (sans, serif,
# display, condensed, script). Lấy qua git sparse-checkout từ
# github.com/google/fonts — chỉ pull đúng folder cần, không clone full
# repo (~30GB). Install xong xoá git để image gọn.
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
    && apk add --no-cache --virtual .build-deps git \
    && git clone --depth 1 --filter=blob:none --no-checkout \
        https://github.com/google/fonts.git /tmp/gfonts \
    && cd /tmp/gfonts \
    && git sparse-checkout init --no-cone \
    && git sparse-checkout set \
        ofl/inter \
        ofl/montserrat \
        ofl/poppins \
        ofl/nunito \
        ofl/outfit \
        ofl/dmsans \
        ofl/plusjakartasans \
        ofl/raleway \
        ofl/bebasneue \
        ofl/anton \
        ofl/oswald \
        ofl/barlowcondensed \
        ofl/russoone \
        ofl/blackopsone \
        ofl/lilitaone \
        ofl/righteous \
        ofl/bangers \
        apache/permanentmarker \
        ofl/paytoneone \
        ofl/boogaloo \
        ofl/fredoka \
        ofl/squadaone \
        ofl/playfairdisplay \
        ofl/cormorant \
        ofl/librebaskerville \
        ofl/lora \
        ofl/bevietnampro \
    && git checkout \
    && mkdir -p /usr/share/fonts/google \
    && find /tmp/gfonts/ofl /tmp/gfonts/apache -type f \( -name "*.ttf" -o -name "*.otf" \) \
        -exec cp {} /usr/share/fonts/google/ \; 2>/dev/null \
    && rm -rf /tmp/gfonts \
    && apk del .build-deps \
    && fc-cache -fv \
    && fc-list | grep -iE "inter|montserrat|poppins|bebas|be vietnam" | head -10

WORKDIR /app
COPY --from=builder /out/video-render /app/video-render
COPY index.html /app/index.html

EXPOSE 8501
CMD ["/app/video-render"]
