# --- Build stage ---
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/translate .

# --- Final stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/translate /app/translate

EXPOSE 8503
CMD ["/app/translate"]
