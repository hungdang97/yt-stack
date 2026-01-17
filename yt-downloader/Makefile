.PHONY: build run clean dev

# Binary name
BINARY=yt-downloader-go

# Build the application
build:
	go build -o $(BINARY) .

# Build optimized for production
build-prod:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) .

# Run the application
run: build
	./$(BINARY)

# Development mode with auto-reload (requires air)
dev:
	air

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -rf storage/*

# Download dependencies
deps:
	go mod tidy
	go mod download

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o $(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY)-windows-amd64.exe .
