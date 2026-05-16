# --- Build stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/deepgram .

# --- Final stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/deepgram /app/deepgram

EXPOSE 8502
CMD ["/app/deepgram"]
