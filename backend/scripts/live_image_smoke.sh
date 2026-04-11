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

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  cat <<'EOF'
Usage:
  ./backend/scripts/live_image_smoke.sh

Purpose:
  Start a temporary backend with ENABLE_LIVE_IMAGE_GENERATION=true,
  submit one minimal job, poll until completion, and verify:
    1. character_image artifacts are written
    2. image shot_images contain the required fields
    3. live image source_image_url fields are surfaced when available

Note:
  This smoke explicitly disables live TTS so unrelated TTS failures do not
  block character_image/image verification.

Useful overrides:
  SMOKE_PORT
  SMOKE_TMP_ROOT
  SMOKE_DATABASE_DSN
  SMOKE_WORKSPACE_DIR
  SMOKE_ARTICLE
  SMOKE_VOICE_ID
  SMOKE_IMAGE_STYLE
  SMOKE_POLL_INTERVAL
  SMOKE_POLL_ATTEMPTS
EOF
  exit 0
fi

SMOKE_TMP_ROOT="${SMOKE_TMP_ROOT:-$(mktemp -d /tmp/narratio-live-smoke.XXXXXX)}"
SMOKE_PORT="${SMOKE_PORT:-18080}"
SMOKE_DATABASE_DRIVER="${SMOKE_DATABASE_DRIVER:-sqlite}"
SMOKE_DATABASE_DSN="${SMOKE_DATABASE_DSN:-$SMOKE_TMP_ROOT/smoke.db}"
SMOKE_WORKSPACE_DIR="${SMOKE_WORKSPACE_DIR:-$SMOKE_TMP_ROOT/workspace}"
SMOKE_VOICE_ID="${SMOKE_VOICE_ID:-default}"
SMOKE_IMAGE_STYLE="${SMOKE_IMAGE_STYLE:-realistic}"
SMOKE_POLL_INTERVAL="${SMOKE_POLL_INTERVAL:-2}"
SMOKE_POLL_ATTEMPTS="${SMOKE_POLL_ATTEMPTS:-90}"
SMOKE_ARTICLE="${SMOKE_ARTICLE:-A woman pauses beneath a black umbrella in a rain-soaked alley, remembers a final promise from her father, takes a breath, and steps into the warm light of a small bookstore.}"

BASE_URL="http://127.0.0.1:${SMOKE_PORT}/api/v1"
SERVER_LOG="${SMOKE_TMP_ROOT}/server.log"
HEALTH_JSON="${SMOKE_TMP_ROOT}/health.json"
JOB_JSON="${SMOKE_TMP_ROOT}/job.json"
TASKS_JSON="${SMOKE_TMP_ROOT}/tasks.json"

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

print_summary() {
  JOB_ID="$1" python3 - <<'PY'
import json
import os
from pathlib import Path

root = Path(os.environ["SMOKE_WORKSPACE_DIR"])
job_id = os.environ["JOB_ID"]
tasks = json.loads(Path(os.environ["TASKS_JSON"]).read_text())

character_image_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "character_image")
image_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "image")
tts_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "tts")
video_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "video")

character_image_output = character_image_task["output_ref"]
image_output = image_task["output_ref"]
tts_output = tts_task["output_ref"]
video_output = video_task["output_ref"]

character_manifest = root / character_image_output["artifact_path"]
image_manifest = root / image_output["artifact_path"]
character_data = json.loads(character_manifest.read_text())
image_data = json.loads(image_manifest.read_text())

character_images = character_data.get("images", [])
shot_images = image_data.get("shot_images", [])
segment_images = image_output.get("images", [])
audio_path = root / tts_output["audio_segment_paths"][0]

missing_character_files = [
    item.get("file_path", "")
    for item in character_images
    if not (root / item.get("file_path", "")).exists()
]

required_shot_fields = ("segment_index", "shot_index", "file_path", "prompt")
invalid_shot_entries = []
for item in shot_images:
    missing = [field for field in required_shot_fields if not item.get(field) and item.get(field) != 0]
    if missing:
        invalid_shot_entries.append(
            {
                "segment_index": item.get("segment_index"),
                "shot_index": item.get("shot_index"),
                "missing_fields": missing,
            }
        )

missing_shot_files = [
    item.get("file_path", "")
    for item in shot_images
    if not (root / item.get("file_path", "")).exists()
]

generated_character_count = sum(1 for item in character_images if not item.get("is_fallback", False))
generated_shot_count = sum(1 for item in shot_images if not item.get("is_fallback", False))
character_source_url_count = sum(1 for item in character_images if item.get("source_image_url"))
shot_source_url_count = sum(1 for item in shot_images if item.get("source_image_url"))

segment_image_path = None
if segment_images:
    segment_image_path = root / segment_images[0]["file_path"]

summary = {
    "job_id": job_id,
    "workspace_dir": str(root),
    "job_status": "completed",
    "character_image_manifest_path": str(character_manifest),
    "character_image_count": len(character_images),
    "generated_character_image_count": generated_character_count,
    "character_source_image_url_count": character_source_url_count,
    "missing_character_image_files": missing_character_files,
    "generated_image_count": image_output.get("generated_image_count"),
    "fallback_image_count": image_output.get("fallback_image_count"),
    "image_manifest_path": str(image_manifest),
    "shot_image_count": len(shot_images),
    "generated_shot_image_count": generated_shot_count,
    "shot_source_image_url_count": shot_source_url_count,
    "invalid_shot_entries": invalid_shot_entries,
    "missing_shot_image_files": missing_shot_files,
    "segment_summary_image_path": str(segment_image_path) if segment_image_path else None,
    "segment_summary_image_exists": segment_image_path.exists() if segment_image_path else False,
    "audio_path": str(audio_path),
    "audio_exists": audio_path.exists(),
    "video_artifact_ref": video_output.get("artifact_path"),
}

errors = []
if not character_images:
    errors.append("character_images manifest is empty")
if missing_character_files:
    errors.append("some character image files are missing")
if not shot_images:
    errors.append("image shot_images manifest is empty")
if invalid_shot_entries:
    errors.append("some shot_images entries are missing required fields")
if missing_shot_files:
    errors.append("some shot image files are missing")

print(json.dumps(summary, ensure_ascii=False, indent=2))
if errors:
    raise SystemExit("smoke verification failed: " + "; ".join(errors))
PY
}

trap cleanup EXIT

mkdir -p "$SMOKE_WORKSPACE_DIR"

echo "Starting live image smoke server..."
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
  ENABLE_LIVE_IMAGE_GENERATION=true \
  ENABLE_LIVE_TEXT_GENERATION=false \
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
TASKS_JSON="$TASKS_JSON" \
python3 - <<'PY'
import json
import os
import time
import urllib.request
from pathlib import Path

base_url = os.environ["BASE_URL"]
job_id = os.environ["JOB_ID"]
poll_attempts = int(os.environ["SMOKE_POLL_ATTEMPTS"])
poll_interval = float(os.environ["SMOKE_POLL_INTERVAL"])
job_json = Path(os.environ["JOB_JSON"])
tasks_json = Path(os.environ["TASKS_JSON"])

terminal = {"completed", "failed", "cancelled"}
last = None

for attempt in range(poll_attempts):
    with urllib.request.urlopen(f"{base_url}/jobs/{job_id}", timeout=20) as resp:
        payload = json.load(resp)
    job_json.write_text(json.dumps(payload, ensure_ascii=False, indent=2))
    data = payload["data"]
    current = (
        data["status"],
        tuple(data.get("task_state", {}).get("running_keys", [])),
        tuple(data.get("task_state", {}).get("failed_keys", [])),
        data.get("progress"),
    )
    if current != last:
        print(
            json.dumps(
                {
                    "attempt": attempt,
                    "status": data["status"],
                    "progress": data.get("progress"),
                    "running_keys": data.get("task_state", {}).get("running_keys", []),
                    "failed_keys": data.get("task_state", {}).get("failed_keys", []),
                },
                ensure_ascii=False,
            )
        )
        last = current
    if data["status"] in terminal:
        with urllib.request.urlopen(f"{base_url}/jobs/{job_id}/tasks", timeout=20) as resp:
            tasks = json.load(resp)
        tasks_json.write_text(json.dumps(tasks, ensure_ascii=False, indent=2))
        if data["status"] != "completed":
            raise SystemExit(f"job ended with status={data['status']}")
        break
    time.sleep(poll_interval)
else:
    raise SystemExit("job polling timed out")
PY

echo "Smoke summary:"
SMOKE_WORKSPACE_DIR="$SMOKE_WORKSPACE_DIR" TASKS_JSON="$TASKS_JSON" print_summary "$JOB_ID"

echo
echo "Artifacts are kept for inspection under:"
echo "  $SMOKE_TMP_ROOT"
echo "Server log:"
echo "  $SERVER_LOG"
