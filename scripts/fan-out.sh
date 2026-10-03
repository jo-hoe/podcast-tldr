#!/usr/bin/env bash
# fan-out.sh — run the pipeline in fan-out mode locally via docker compose.
#
# After download, reads the manifest to get episode IDs, then runs
# transcribe->summarize->zip->backup for each episode in parallel (bounded
# by MAX_PARALLEL, default 4). Episodes already backed up are skipped.
#
# Usage:
#   export PODCAST_TLDR_BACKUP_TOKEN="$(gh auth token)"
#   bash scripts/fan-out.sh [MAX_PARALLEL]
#
# Example (8 parallel episode chains):
#   bash scripts/fan-out.sh 8
#
# The per-episode containers use locally built images when docker-compose.local.yml
# is present alongside docker-compose.run.yml.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$SCRIPT_DIR/.."
cd "$ROOT"

MAX_PARALLEL="${1:-4}"

COMPOSE_FILES="-f docker-compose.yml"
[ -f docker-compose.local.yml ] && COMPOSE_FILES="$COMPOSE_FILES -f docker-compose.local.yml"
[ -f docker-compose.run.yml ]   && COMPOSE_FILES="$COMPOSE_FILES -f docker-compose.run.yml"

echo "=== Step 1: Download ==="
docker compose $COMPOSE_FILES run --rm download

echo "=== Step 2: Scatter — read episode IDs from manifest ==="
EPISODES=$(python3 scripts/scatter.py volume/work 2>/dev/null || \
  docker run --rm -v "$(pwd -W)/volume/work://app/mount/work" \
    -v "$(pwd -W)/scripts/scatter.py://app/scatter.py" \
    python:3.12-slim python3 //app/scatter.py //app/mount/work)

EPISODE_IDS=$(echo "$EPISODES" | python3 -c "import json,sys; ids=json.load(sys.stdin); [print(i) for i in ids]")
TOTAL=$(echo "$EPISODE_IDS" | wc -l | tr -d ' ')
echo "=== $TOTAL episodes to process (parallelism=$MAX_PARALLEL) ==="

# Process episodes in parallel batches
run_episode() {
  local ep_id="$1"
  echo "[episode] starting: $ep_id"
  EPISODE_ID="$ep_id" docker compose $COMPOSE_FILES \
    run --rm -e EPISODE_ID="$ep_id" transcribe 2>&1 | grep -E "(INFO|ERROR|transcribed|failed)" | sed "s/^/[$ep_id] /" || true
  EPISODE_ID="$ep_id" docker compose $COMPOSE_FILES \
    run --rm -e EPISODE_ID="$ep_id" summarize  2>&1 | grep -E "(INFO|ERROR|summarized|failed)" | sed "s/^/[$ep_id] /" || true
  EPISODE_ID="$ep_id" docker compose $COMPOSE_FILES \
    run --rm -e EPISODE_ID="$ep_id" zip       2>&1 | grep -E "(INFO|ERROR|bundled|failed)"    | sed "s/^/[$ep_id] /" || true
  EPISODE_ID="$ep_id" docker compose $COMPOSE_FILES \
    run --rm -e EPISODE_ID="$ep_id" backup    2>&1 | grep -E "(INFO|ERROR|backed|failed)"     | sed "s/^/[$ep_id] /" || true
  echo "[episode] done: $ep_id"
}

export -f run_episode
export COMPOSE_FILES

echo "$EPISODE_IDS" | xargs -P "$MAX_PARALLEL" -I{} bash -c 'run_episode "$@"' _ {}

echo "=== Fan-out complete ==="
