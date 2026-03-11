# Instagram Extractor API

FastAPI microservice for extracting Instagram profile data with pagination support.

## Quick Start

```bash
# Install dependencies
pip install -r requirements.txt

# Run server
python3 -m uvicorn app:app --host 0.0.0.0 --port 8000
```

## API Endpoints

### Get Profile with Pagination
```bash
GET /profile/{username}?offset=0&limit=20&proxy={proxy_url}&cookie={cookies_json}
```

**Parameters:**
- `offset`: Number of posts to skip (default: 0)
- `limit`: Posts per page (default: 12, max: 100)
- `proxy` (optional): Proxy URL
- `cookie` (optional): Instagram cookies as JSON string

**Example:**
```bash
curl "http://localhost:8000/profile/natgeo?limit=20"
```

### Extract Post/Reel
```bash
GET /extract?url={instagram_post_url}
```

## Docker

```bash
docker build -t insta-extractor .
docker run -p 8000:8000 insta-extractor
```

## Features

- Pagination with `has_more` flag
- Proxy & cookie authentication support
- Returns CDN URLs (no media download)
- CORS enabled
- Supports all post types (photos, videos, carousels)
