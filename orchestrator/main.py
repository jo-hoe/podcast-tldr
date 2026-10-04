"""
podcast-tldr orchestrator: fan-out per-episode pipeline after download.

Reads episodes.yaml, then for each episode runs:
  transcribe -> summarize -> zip -> backup

in parallel (bounded by MAX_PARALLEL, default 4), each as a separate
`docker compose run` command so the per-stage images, volumes, and env vars
are all resolved from the compose files.

Environment:
  MAX_PARALLEL          max concurrent episode chains (default: 4)
  PODCAST_TLDR_BACKUP_TOKEN  git token for backup stage
  COMPOSE_FILES         space-separated compose files (default: auto-detected)
  WORK_DIR              path to episodes.yaml dir (default: /app/mount/work)
"""
from __future__ import annotations

import json
import logging
import os
import subprocess
import sys
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

import yaml

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(message)s",
)
log = logging.getLogger(__name__)

STAGES = ("transcribe", "summarize", "zip", "backup")


def scatter(work_dir: str) -> list[str]:
    path = Path(work_dir) / "episodes.yaml"
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    return [
        ep["id"]
        for pod in data.get("podcasts", [])
        for ep in pod.get("episodes", [])
        if ep.get("audioFile") and not ep.get("backedUp", False)
    ]


def compose_files() -> list[str]:
    files = os.environ.get("COMPOSE_FILES", "").split()
    if files:
        return files
    # Auto-detect from cwd
    candidates = [
        "docker-compose.yml",
        "docker-compose.local.yml",
        "docker-compose.run.yml",
    ]
    result = []
    for f in candidates:
        if Path(f).exists():
            result += ["-f", f]
    return result


def run_episode(ep_id: str, cf: list[str], env: dict) -> bool:
    for stage in STAGES:
        cmd = ["docker", "compose"] + cf + [
            "run", "--rm",
            "-e", f"EPISODE_ID={ep_id}",
            stage,
        ]
        log.info("[%s] starting %s", ep_id, stage)
        r = subprocess.run(cmd, env=env, capture_output=True, text=True)
        if r.returncode != 0:
            log.error("[%s] %s FAILED:\n%s", ep_id, stage, r.stderr[-1000:])
            return False
        log.info("[%s] %s done", ep_id, stage)
    log.info("[%s] complete ✓", ep_id)
    return True


def main() -> int:
    work_dir = os.environ.get("WORK_DIR", "/app/mount/work")
    max_parallel = int(os.environ.get("MAX_PARALLEL", "4"))

    cf = compose_files()
    if not cf:
        log.error("No compose files found. Run from repo root.")
        return 1

    env = os.environ.copy()

    log.info("Scattering episodes from %s", work_dir)
    episode_ids = scatter(work_dir)
    total = len(episode_ids)
    log.info("Fan-out: %d episodes, parallelism=%d", total, max_parallel)

    done = 0
    failed = 0
    lock = threading.Lock()

    with ThreadPoolExecutor(max_workers=max_parallel) as pool:
        futures = {pool.submit(run_episode, ep, cf, env): ep for ep in episode_ids}
        for future in as_completed(futures):
            ep = futures[future]
            ok = future.result()
            with lock:
                if ok:
                    done += 1
                else:
                    failed += 1
                log.info("Progress: %d/%d  done=%d failed=%d", done + failed, total, done, failed)

    log.info("=== Fan-out complete: %d succeeded, %d failed ===", done, failed)
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
