"""
podcast-tldr work-queue orchestrator.

Architecture:
  1. Scatter: read episodes.yaml → create queue/todo/<id> files (one per episode)
  2. Worker pool: N workers each atomically claim one episode (todo → in-progress),
     run the full chain (transcribe→summarize→zip→backup), then mark done.
  3. If any stage fails, the episode is moved to queue/failed/<id>.
  4. Resume-safe: already-done episodes (queue/done/<id> or backedUp in manifest)
     are skipped. Restart just re-runs from the queue.

Usage:
  python3 scripts/run_queue.py [--workers N] [--compose-files f1 f2 ...]

Environment:
  PODCAST_TLDR_BACKUP_TOKEN  git token for backup stage
  N_WORKERS                  number of parallel workers (default: 4)
"""
from __future__ import annotations

import argparse
import logging
import os
import subprocess
import sys
import threading
from pathlib import Path
from queue import Queue, Empty

import yaml

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s [%(threadName)s] %(message)s",
)
log = logging.getLogger(__name__)

STAGES = ("transcribe", "summarize", "zip", "backup")


def setup_queue(work_dir: str, compose_files: list[str], env: dict) -> list[str]:
    """
    Read episodes.yaml, create queue dirs, populate todo/ with episode IDs.
    Episodes already in done/ or backed-up in the manifest are skipped.
    Returns the list of episode IDs to process.
    """
    queue_dir = Path(work_dir) / "queue"
    todo_dir = queue_dir / "todo"
    done_dir = queue_dir / "done"
    failed_dir = queue_dir / "failed"
    in_progress_dir = queue_dir / "in-progress"

    for d in (todo_dir, done_dir, failed_dir, in_progress_dir):
        d.mkdir(parents=True, exist_ok=True)

    manifest_path = Path(work_dir) / "episodes.yaml"
    data = yaml.safe_load(manifest_path.read_text(encoding="utf-8")) or {}

    todo = []
    for pod in data.get("podcasts", []):
        for ep in pod.get("episodes", []):
            ep_id = ep["id"]
            if not ep.get("audioFile"):
                continue
            if ep.get("backedUp"):
                # Mark as done if not already
                (done_dir / ep_id).touch()
                continue
            if (done_dir / ep_id).exists():
                log.info("skipping already done: %s", ep_id)
                continue
            # Move out of in-progress if stuck from a previous run
            ip = in_progress_dir / ep_id
            if ip.exists():
                ip.unlink()
            # Create todo marker
            (todo_dir / ep_id).touch(exist_ok=True)
            todo.append(ep_id)

    log.info("Queue: %d episodes to process", len(todo))
    return todo


def claim_episode(todo_dir: Path, in_progress_dir: Path) -> str | None:
    """Atomically claim one episode: rename todo/<id> → in-progress/<id>."""
    for marker in sorted(todo_dir.iterdir()):
        ep_id = marker.name
        target = in_progress_dir / ep_id
        try:
            marker.rename(target)  # atomic on Linux/ext4/overlayfs
            return ep_id
        except (FileNotFoundError, FileExistsError):
            continue  # Another worker grabbed it first
    return None


def run_stage(stage: str, ep_id: str, cf: list[str], env: dict, log_path: Path) -> bool:
    cmd = ["docker", "compose"] + cf + [
        "run", "--rm", "--no-deps",
        "-e", f"EPISODE_ID={ep_id}",
        stage,
    ]
    with open(log_path, "a") as f:
        r = subprocess.run(cmd, env=env, stdin=subprocess.DEVNULL, stdout=f, stderr=f)
    return r.returncode == 0


def process_episode(
    ep_id: str,
    queue_dir: Path,
    cf: list[str],
    env: dict,
    worker_name: str,
) -> bool:
    in_progress_dir = queue_dir / "in-progress"
    done_dir = queue_dir / "done"
    failed_dir = queue_dir / "failed"
    log_path = queue_dir / "logs" / f"{ep_id}.log"
    log_path.parent.mkdir(exist_ok=True)

    # Create in-progress marker
    ip = in_progress_dir / ep_id
    ip.touch()

    log.info("[%s] processing: %s", worker_name, ep_id)
    for stage in STAGES:
        log.info("[%s] %s → %s", worker_name, ep_id, stage)
        if not run_stage(stage, ep_id, cf, env, log_path):
            log.error("[%s] FAILED %s at %s", worker_name, ep_id, stage)
            try:
                ip.rename(failed_dir / ep_id)
            except Exception:
                (failed_dir / ep_id).touch()
            return False

    try:
        ip.rename(done_dir / ep_id)
    except Exception:
        (done_dir / ep_id).touch()
    log.info("[%s] ✓ done: %s", worker_name, ep_id)
    return True


def worker(worker_id: int, queue: Queue, queue_dir: Path, cf: list[str], env: dict) -> None:
    name = f"worker-{worker_id}"
    while True:
        try:
            ep_id = queue.get(timeout=5)
        except Empty:
            break
        process_episode(ep_id, queue_dir, cf, env, name)
        queue.task_done()


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description="podcast-tldr work-queue orchestrator")
    p.add_argument("--workers", "-n", type=int, default=int(os.environ.get("N_WORKERS", "4")))
    p.add_argument("--work-dir", default=os.environ.get("WORK_DIR", "volume/work"))
    p.add_argument("--compose-files", nargs="*", default=[])
    return p.parse_args()


def main() -> int:
    args = parse_args()
    work_dir = str(Path(args.work_dir).resolve())
    n_workers = args.workers

    # Resolve compose files
    cf_files = args.compose_files or [
        f for f in ["docker-compose.yml", "docker-compose.local.yml", "docker-compose.run.yml"]
        if Path(f).exists()
    ]
    cf = []
    for f in cf_files:
        cf += ["-f", f]

    env = os.environ.copy()

    log.info("Starting orchestrator: workers=%d, compose=%s", n_workers, cf_files)

    # Step 1: download
    log.info("Running download stage...")
    r = subprocess.run(
        ["docker", "compose"] + cf + ["run", "--rm", "download"],
        env=env, stdin=subprocess.DEVNULL
    )
    if r.returncode != 0:
        log.error("Download failed")
        return 1

    # Step 2: setup queue
    queue_dir = Path(work_dir) / "queue"
    episode_ids = setup_queue(work_dir, cf, env)
    if not episode_ids:
        log.info("Nothing to process")
        return 0

    # Step 3: fill work queue and run workers
    q: Queue = Queue()
    for ep_id in episode_ids:
        q.put(ep_id)

    threads = []
    for i in range(min(n_workers, len(episode_ids))):
        t = threading.Thread(
            target=worker,
            args=(i + 1, q, queue_dir, cf, env),
            name=f"worker-{i+1}",
            daemon=True,
        )
        t.start()
        threads.append(t)

    q.join()
    for t in threads:
        t.join(timeout=5)

    done = len(list((queue_dir / "done").iterdir()))
    failed = len(list((queue_dir / "failed").iterdir()))
    log.info("=== Complete: %d done, %d failed ===", done, failed)
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
