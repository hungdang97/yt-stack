# YouTube Extractor Android (NewPipe-based)

Fast YouTube metadata extractor using NewPipe library.

## Build

```bash
./gradlew clean build
```

## Run Locally

```bash
java -jar build/libs/youtube-api-1.0.0.jar
```

## Docker

This service is integrated into the main `yt-stack` docker-compose.yml.

Port: **8100**

## API

- `GET /api/youtube/video/{videoId}` - Extract video metadata
- `GET /api/youtube/health` - Health check
