#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

if ! command -v python3 >/dev/null 2>&1; then
  echo "python3 is required" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

SMOKE_TMP_ROOT="${SMOKE_TMP_ROOT:-$(mktemp -d /tmp/narratio-e2e-video-download.XXXXXX)}"
SMOKE_PORT="${SMOKE_PORT:-18081}"
SMOKE_DATABASE_DRIVER="${SMOKE_DATABASE_DRIVER:-sqlite}"
SMOKE_DATABASE_DSN="${SMOKE_DATABASE_DSN:-$SMOKE_TMP_ROOT/smoke.db}"
SMOKE_WORKSPACE_DIR="${SMOKE_WORKSPACE_DIR:-$SMOKE_TMP_ROOT/workspace}"
SMOKE_VOICE_ID="${SMOKE_VOICE_ID:-default}"
SMOKE_IMAGE_STYLE="${SMOKE_IMAGE_STYLE:-realistic}"
SMOKE_POLL_INTERVAL="${SMOKE_POLL_INTERVAL:-2}"
SMOKE_POLL_ATTEMPTS="${SMOKE_POLL_ATTEMPTS:-90}"
SMOKE_ARTICLE="${SMOKE_ARTICLE:-暴雨停后，林夏站在旧城巷口，想起父亲留下的最后一句话，随后走向巷子深处亮着灯的小书店。}"

BASE_URL="http://127.0.0.1:${SMOKE_PORT}/api/v1"
SERVER_LOG="${SMOKE_TMP_ROOT}/server.log"
HEALTH_JSON="${SMOKE_TMP_ROOT}/health.json"
JOB_JSON="${SMOKE_TMP_ROOT}/job.json"
DOWNLOAD_FILE="${SMOKE_TMP_ROOT}/downloaded.mp4"
DOWNLOAD_HEADERS="${SMOKE_TMP_ROOT}/download.headers"
RANGE_FILE="${SMOKE_TMP_ROOT}/downloaded.range.bin"
RANGE_HEADERS="${SMOKE_TMP_ROOT}/download.range.headers"

SERVER_PID=""

cleanup() {
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}

wait_for_health() {
  local attempt
  for attempt in $(seq 1 30); do
    if curl -fsS "${BASE_URL}/health" >"$HEALTH_JSON"; then
      return 0
    fi
    sleep 1
  done

  echo "backend did not become healthy" >&2
  if [[ -f "$SERVER_LOG" ]]; then
    echo "--- server.log ---" >&2
    tail -n 50 "$SERVER_LOG" >&2 || true
  fi
  return 1
}

trap cleanup EXIT

mkdir -p "$SMOKE_WORKSPACE_DIR"

echo "Starting e2e video download smoke server..."
echo "  tmp_root: $SMOKE_TMP_ROOT"
echo "  database: $SMOKE_DATABASE_DSN"
echo "  workspace: $SMOKE_WORKSPACE_DIR"
echo "  base_url: $BASE_URL"

(
  cd "$BACKEND_DIR"
  PORT="$SMOKE_PORT" \
  DATABASE_DRIVER="$SMOKE_DATABASE_DRIVER" \
  DATABASE_DSN="$SMOKE_DATABASE_DSN" \
  WORKSPACE_DIR="$SMOKE_WORKSPACE_DIR" \
  ENABLE_LIVE_TEXT_GENERATION=false \
  ENABLE_LIVE_IMAGE_GENERATION=false \
  ENABLE_LIVE_VIDEO_GENERATION=false \
  TTS_API_BASE_URL= \
  TTS_JWT_PRIVATE_KEY= \
  go run cmd/server/main.go
) >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

wait_for_health

echo "Health check:"
cat "$HEALTH_JSON"
echo

JOB_ID="$(
  SMOKE_ARTICLE="$SMOKE_ARTICLE" \
  SMOKE_VOICE_ID="$SMOKE_VOICE_ID" \
  SMOKE_IMAGE_STYLE="$SMOKE_IMAGE_STYLE" \
  BASE_URL="$BASE_URL" \
  python3 - <<'PY'
import json
import os
import urllib.request

payload = {
    "article": os.environ["SMOKE_ARTICLE"],
    "options": {
        "voice_id": os.environ["SMOKE_VOICE_ID"],
        "image_style": os.environ["SMOKE_IMAGE_STYLE"],
    },
}

req = urllib.request.Request(
    os.environ["BASE_URL"] + "/jobs",
    data=json.dumps(payload).encode("utf-8"),
    headers={"Content-Type": "application/json"},
)
with urllib.request.urlopen(req, timeout=30) as resp:
    body = json.load(resp)
print(body["data"]["job_id"])
PY
)"

echo "Created job: $JOB_ID"

BASE_URL="$BASE_URL" \
JOB_ID="$JOB_ID" \
SMOKE_POLL_ATTEMPTS="$SMOKE_POLL_ATTEMPTS" \
SMOKE_POLL_INTERVAL="$SMOKE_POLL_INTERVAL" \
JOB_JSON="$JOB_JSON" \
python3 - <<'PY'
import json
import os
import time
import urllib.request

base_url = os.environ["BASE_URL"]
job_id = os.environ["JOB_ID"]
attempts = int(os.environ["SMOKE_POLL_ATTEMPTS"])
interval = float(os.environ["SMOKE_POLL_INTERVAL"])
job_json_path = os.environ["JOB_JSON"]

for attempt in range(attempts):
    with urllib.request.urlopen(f"{base_url}/jobs/{job_id}", timeout=30) as resp:
        body = json.load(resp)
    with open(job_json_path, "w", encoding="utf-8") as handle:
        json.dump(body, handle, ensure_ascii=False, indent=2)
    data = body["data"]
    print(json.dumps({
        "attempt": attempt,
        "status": data["status"],
        "progress": data["progress"],
        "running_keys": data["task_state"]["running_keys"],
        "failed_keys": data["task_state"]["failed_keys"],
    }, ensure_ascii=False))
    if data["status"] == "completed":
        raise SystemExit(0)
    if data["status"] == "failed":
        raise SystemExit("job failed before download smoke completed")
    time.sleep(interval)

raise SystemExit("job did not complete within polling window")
PY

curl -fsS -D "$DOWNLOAD_HEADERS" "${BASE_URL}/jobs/${JOB_ID}/download" -o "$DOWNLOAD_FILE"
curl -fsS -H "Range: bytes=0-15" -D "$RANGE_HEADERS" "${BASE_URL}/jobs/${JOB_ID}/download" -o "$RANGE_FILE"

JOB_JSON="$JOB_JSON" \
DOWNLOAD_FILE="$DOWNLOAD_FILE" \
DOWNLOAD_HEADERS="$DOWNLOAD_HEADERS" \
RANGE_FILE="$RANGE_FILE" \
RANGE_HEADERS="$RANGE_HEADERS" \
python3 - <<'PY'
import json
import os
from pathlib import Path

job = json.loads(Path(os.environ["JOB_JSON"]).read_text(encoding="utf-8"))
download_file = Path(os.environ["DOWNLOAD_FILE"])
download_headers = Path(os.environ["DOWNLOAD_HEADERS"]).read_text(encoding="utf-8").lower()
range_file = Path(os.environ["RANGE_FILE"])
range_headers = Path(os.environ["RANGE_HEADERS"]).read_text(encoding="utf-8").lower()

size = download_file.stat().st_size
range_size = range_file.stat().st_size
result = job["data"]["result"]

summary = {
    "job_id": job["data"]["job_id"],
    "job_status": job["data"]["status"],
    "video_url": result["video_url"],
    "reported_file_size": result["file_size"],
    "downloaded_file_size": size,
    "content_type_ok": "content-type: video/mp4" in download_headers,
    "content_disposition_ok": "content-disposition: attachment;" in download_headers,
    "accept_ranges_ok": "accept-ranges: bytes" in range_headers,
    "range_status_ok": "206 partial content" in range_headers,
    "range_file_size": range_size,
}

print("Smoke summary:")
print(json.dumps(summary, ensure_ascii=False, indent=2))

errors = []
if job["data"]["status"] != "completed":
    errors.append("job did not complete")
if size <= 0:
    errors.append("downloaded file is empty")
if not summary["content_type_ok"]:
    errors.append("download content-type is not video/mp4")
if not summary["content_disposition_ok"]:
    errors.append("download content-disposition missing attachment")
if not summary["accept_ranges_ok"]:
    errors.append("range response missing accept-ranges")
if not summary["range_status_ok"]:
    errors.append("range response status is not 206")
if range_size <= 0:
    errors.append("range download is empty")

if errors:
    raise SystemExit("smoke verification failed: " + "; ".join(errors))
PY

echo
echo "Artifacts are kept for inspection under:"
echo "  $SMOKE_TMP_ROOT"
echo "Server log:"
echo "  $SERVER_LOG"
