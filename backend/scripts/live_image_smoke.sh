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
  submit one minimal job, poll until completion, and print artifact paths.

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

image_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "image")
tts_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "tts")
video_task = next(item for item in tasks["data"]["tasks"] if item["key"] == "video")

image_output = image_task["output_ref"]
tts_output = tts_task["output_ref"]
video_output = video_task["output_ref"]

image_manifest = root / image_output["artifact_path"]
generated_image_path = root / image_output["images"][0]["file_path"]
subtitle_path = root / tts_output["subtitle_artifact_ref"]
audio_path = root / tts_output["audio_segment_paths"][0]

summary = {
    "job_id": job_id,
    "workspace_dir": str(root),
    "job_status": "completed",
    "generated_image_count": image_output.get("generated_image_count"),
    "fallback_image_count": image_output.get("fallback_image_count"),
    "image_manifest_path": str(image_manifest),
    "generated_image_path": str(generated_image_path),
    "generated_image_exists": generated_image_path.exists(),
    "generated_image_size": generated_image_path.stat().st_size if generated_image_path.exists() else None,
    "subtitle_path": str(subtitle_path),
    "subtitle_exists": subtitle_path.exists(),
    "audio_path": str(audio_path),
    "audio_exists": audio_path.exists(),
    "video_artifact_ref": video_output.get("artifact_path"),
}
print(json.dumps(summary, ensure_ascii=False, indent=2))
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
