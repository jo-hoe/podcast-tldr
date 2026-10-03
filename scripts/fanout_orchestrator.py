"""
fan-out orchestrator: runs the podcast-tldr pipeline per-episode in parallel.

Usage (from the repo root):
  docker run --rm \\
    -v "$(pwd)/volume/work:/app/mount/work" \\
    -v "$(pwd)/scripts:/scripts" \\
    -v "/var/run/docker.sock:/var/run/docker.sock" \\
    -e PODCAST_TLDR_BACKUP_TOKEN="..." \\
    -e MAX_PARALLEL=4 \\
    python:3.12-slim python3 /scripts/fanout_orchestrator.py

Or use the Makefile target: make run-fanout-compose
"""
import json
import os
import subprocess
import sys
import threading
import yaml
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path


def scatter(work_dir: str) -> list[str]:
    """Return episode IDs that need processing (have audioFile, not backedUp)."""
    manifest_path = Path(work_dir) / "episodes.yaml"
    data = yaml.safe_load(manifest_path.read_text(encoding="utf-8")) or {}
    return [
        ep["id"]
        for pod in data.get("podcasts", [])
        for ep in pod.get("episodes", [])
        if ep.get("audioFile") and not ep.get("backedUp", False)
    ]


def run_stage(stage: str, episode_id: str, compose_args: list[str], env: dict) -> bool:
    """Run one stage for one episode via docker compose run."""
    cmd = ["docker", "compose"] + compose_args + [
        "run", "--rm",
        "-e", f"EPISODE_ID={episode_id}",
        stage,
    ]
    result = subprocess.run(cmd, env=env, capture_output=True, text=True)
    if result.returncode != 0:
        print(f"[{episode_id}] {stage} FAILED: {result.stderr[-500:]}", flush=True)
        return False
    return True


def process_episode(episode_id: str, compose_args: list[str], env: dict) -> bool:
    """Run transcribe -> summarize -> zip -> backup for one episode."""
    for stage in ("transcribe", "summarize", "zip", "backup"):
        print(f"[{episode_id}] starting {stage}", flush=True)
        if not run_stage(stage, episode_id, compose_args, env):
            print(f"[{episode_id}] aborting at {stage}", flush=True)
            return False
        print(f"[{episode_id}] {stage} done", flush=True)
    print(f"[{episode_id}] complete", flush=True)
    return True


def main():
    work_dir = os.environ.get("WORK_DIR", "volume/work")
    max_parallel = int(os.environ.get("MAX_PARALLEL", "4"))
    backup_token = os.environ.get("PODCAST_TLDR_BACKUP_TOKEN", "")

    compose_files = ["-f", "docker-compose.yml"]
    if Path("docker-compose.local.yml").exists():
        compose_files += ["-f", "docker-compose.local.yml"]
    if Path("docker-compose.run.yml").exists():
        compose_files += ["-f", "docker-compose.run.yml"]

    env = os.environ.copy()
    env["PODCAST_TLDR_BACKUP_TOKEN"] = backup_token

    episode_ids = scatter(work_dir)
    total = len(episode_ids)
    print(f"=== {total} episodes to process (parallelism={max_parallel}) ===", flush=True)

    done = 0
    failed = 0
    lock = threading.Lock()

    with ThreadPoolExecutor(max_workers=max_parallel) as pool:
        futures = {pool.submit(process_episode, ep, compose_files, env): ep for ep in episode_ids}
        for future in as_completed(futures):
            ep = futures[future]
            ok = future.result()
            with lock:
                if ok:
                    done += 1
                else:
                    failed += 1
                print(f"Progress: {done+failed}/{total} done={done} failed={failed}", flush=True)

    print(f"=== Fan-out complete: {done} succeeded, {failed} failed ===", flush=True)
    sys.exit(0 if failed == 0 else 1)


if __name__ == "__main__":
    main()
