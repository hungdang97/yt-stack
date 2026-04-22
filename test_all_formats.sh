#!/bin/bash
# ============================================================
# Test ALL download formats (6 video + 8 audio)
# Creates jobs ONE BY ONE to avoid race conditions
# Downloads first 5MB of each stream + full file for merged
# ============================================================

API="https://vps-3bcc7e9d.ytconvert.org"
TOKEN="1234567890987654321234567890987654321"
VIDEO_URL="https://www.youtube.com/watch?v=q2zibGwG6Zo"  # 49 min video

OUTPUT_DIR="/tmp/format-test-$(date +%Y%m%d_%H%M%S)"
mkdir -p "$OUTPUT_DIR"

VIDEO_FORMATS=("mp4" "webm" "mkv" "avi" "flv" "mov")
AUDIO_FORMATS=("mp3" "m4a" "wav" "opus" "flac" "ogg" "aac" "alac")

echo "============================================"
echo "  Format Test - $(date)"
echo "  Output: $OUTPUT_DIR"
echo "============================================"
echo ""

create_job() {
    local type=$1
    local format=$2
    local resp
    resp=$(curl -s "$API/api/download" \
        -H "Content-Type: application/json" \
        -H "X-Hub-Token: $TOKEN" \
        -d "{\"url\":\"$VIDEO_URL\",\"output\":{\"type\":\"$type\",\"format\":\"$format\"}}")

    local status_url
    status_url=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['statusUrl'])" 2>/dev/null)

    if [ -z "$status_url" ]; then
        echo "FAIL: Could not create job. Response: $resp"
        return 1
    fi
    echo "$status_url"
}

wait_for_job() {
    local status_url=$1
    local format=$2
    local max_wait=120
    local waited=0

    while [ $waited -lt $max_wait ]; do
        local resp
        resp=$(curl -s "$status_url")
        local status
        status=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d['status'])" 2>/dev/null)

        if [ "$status" = "completed" ]; then
            local dl_url
            dl_url=$(echo "$resp" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('downloadUrl',''))" 2>/dev/null)
            echo "$dl_url"
            return 0
        elif [ "$status" = "error" ]; then
            echo "ERROR"
            return 1
        fi

        sleep 3
        waited=$((waited + 3))
        printf "\r  ⏳ %s waiting... %ds" "$format" "$waited" >&2
    done
    echo "TIMEOUT"
    return 1
}

download_file() {
    local url=$1
    local output=$2
    local max_size=$3  # in bytes, 0 = full download

    if [ "$max_size" -gt 0 ] 2>/dev/null; then
        # Download with timeout (for streams, download ~max_size then stop)
        curl -s --max-time 30 -o "$output" "$url" 2>/dev/null &
        local pid=$!

        # Wait until file reaches target size or curl finishes
        while kill -0 $pid 2>/dev/null; do
            local size
            size=$(stat -f%z "$output" 2>/dev/null || echo 0)
            if [ "$size" -ge "$max_size" ]; then
                kill $pid 2>/dev/null
                wait $pid 2>/dev/null
                break
            fi
            sleep 0.5
        done
        wait $pid 2>/dev/null
    else
        curl -s -o "$output" "$url"
    fi
}

verify_file() {
    local file=$1
    local expected_type=$2  # "video" or "audio"
    local expected_format=$3

    local size
    size=$(stat -f%z "$file" 2>/dev/null || echo 0)
    local size_human
    size_human=$(du -h "$file" | cut -f1)

    if [ "$size" -eq 0 ]; then
        echo "  ❌ FAIL: 0 bytes"
        return 1
    fi

    local file_type
    file_type=$(file "$file" | cut -d: -f2-)

    local ffprobe_out
    ffprobe_out=$(ffprobe -hide_banner -i "$file" 2>&1)

    local has_audio has_video duration
    has_audio=$(echo "$ffprobe_out" | grep -c "Audio:")
    has_video=$(echo "$ffprobe_out" | grep -c "Video:")
    duration=$(echo "$ffprobe_out" | grep -oE "Duration: [0-9:]+" | head -1)

    local codec_info
    codec_info=$(echo "$ffprobe_out" | grep "Stream #0" | head -2)

    echo "  Size: $size_human"
    echo "  File: $file_type"
    echo "  $codec_info"

    if [ "$expected_type" = "video" ]; then
        if [ "$has_video" -eq 0 ]; then
            echo "  ❌ FAIL: No video stream found"
            return 1
        fi
        if [ "$has_audio" -eq 0 ]; then
            echo "  ❌ FAIL: No audio stream found"
            return 1
        fi
        echo "  ✅ OK (video+audio, $duration)"
    else
        if [ "$has_audio" -eq 0 ]; then
            echo "  ❌ FAIL: No audio stream found"
            return 1
        fi
        echo "  ✅ OK (audio, $duration)"
    fi
    return 0
}

check_content_type() {
    local url=$1
    local expected_format=$2

    local headers
    headers=$(curl -sI "$url" 2>&1)
    local ct
    ct=$(echo "$headers" | grep -i "content-type:" | tr -d '\r')
    local cd
    cd=$(echo "$headers" | grep -i "content-disposition:" | tr -d '\r' | grep -oE 'filename="[^"]+"' | head -1)

    echo "  Content-Type: $ct"
    echo "  Filename: $cd"

    # Check if filename extension matches expected format
    if echo "$cd" | grep -qi "\.${expected_format}\""; then
        return 0
    else
        echo "  ⚠️  WARNING: Filename extension doesn't match expected .$expected_format"
        return 1
    fi
}

PASS=0
FAIL=0
RESULTS=()

# ============ VIDEO FORMATS ============
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  VIDEO FORMATS (${#VIDEO_FORMATS[@]})"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for fmt in "${VIDEO_FORMATS[@]}"; do
    echo ""
    echo "▶ $fmt"
    echo "  Creating job..."

    status_url=$(create_job "video" "$fmt")
    if [ $? -ne 0 ]; then
        echo "  ❌ FAIL: Job creation failed"
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ video/$fmt - job creation failed")
        continue
    fi

    dl_url=$(wait_for_job "$status_url" "$fmt")
    printf "\r                              \r" >&2

    if [ "$dl_url" = "ERROR" ] || [ "$dl_url" = "TIMEOUT" ] || [ -z "$dl_url" ]; then
        echo "  ❌ FAIL: Job failed ($dl_url)"
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ video/$fmt - job failed")
        continue
    fi

    echo "  Checking headers..."
    check_content_type "$dl_url" "$fmt"

    echo "  Downloading..."
    outfile="$OUTPUT_DIR/video_${fmt}.${fmt}"
    download_file "$dl_url" "$outfile" 10485760  # 10MB for video

    echo "  Verifying..."
    if verify_file "$outfile" "video" "$fmt"; then
        PASS=$((PASS + 1))
        RESULTS+=("✅ video/$fmt")
    else
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ video/$fmt")
    fi
done

# ============ AUDIO FORMATS ============
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  AUDIO FORMATS (${#AUDIO_FORMATS[@]})"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

for fmt in "${AUDIO_FORMATS[@]}"; do
    echo ""
    echo "▶ $fmt"
    echo "  Creating job..."

    status_url=$(create_job "audio" "$fmt")
    if [ $? -ne 0 ]; then
        echo "  ❌ FAIL: Job creation failed"
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ audio/$fmt - job creation failed")
        continue
    fi

    dl_url=$(wait_for_job "$status_url" "$fmt")
    printf "\r                              \r" >&2

    if [ "$dl_url" = "ERROR" ] || [ "$dl_url" = "TIMEOUT" ] || [ -z "$dl_url" ]; then
        echo "  ❌ FAIL: Job failed ($dl_url)"
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ audio/$fmt - job failed")
        continue
    fi

    echo "  Checking headers..."
    check_content_type "$dl_url" "$fmt"

    echo "  Downloading..."
    outfile="$OUTPUT_DIR/audio_${fmt}.${fmt}"
    download_file "$dl_url" "$outfile" 5242880  # 5MB for audio

    echo "  Verifying..."
    if verify_file "$outfile" "audio" "$fmt"; then
        PASS=$((PASS + 1))
        RESULTS+=("✅ audio/$fmt")
    else
        FAIL=$((FAIL + 1))
        RESULTS+=("❌ audio/$fmt")
    fi
done

# ============ SUMMARY ============
echo ""
echo ""
echo "============================================"
echo "  SUMMARY: $PASS passed, $FAIL failed"
echo "============================================"
for r in "${RESULTS[@]}"; do
    echo "  $r"
done
echo ""
echo "Files saved to: $OUTPUT_DIR"
echo ""

# Quick listing
echo "File sizes:"
ls -lhS "$OUTPUT_DIR"/ | tail -n +2 | awk '{print "  " $5 "\t" $NF}'
