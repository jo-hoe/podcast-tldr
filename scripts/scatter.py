"""Scatter step: reads episodes.yaml from the work dir and prints a JSON array
of episode IDs that need processing (have audioFile but not yet backedUp).

Used as an Argo 'script' step after download to fan out the per-episode DAG.
Usage:
  python3 scatter.py <workDir>
Output (stdout):
  ["id-1", "id-2", ...]  (JSON array, one line)
"""
import json
import sys
from pathlib import Path

import yaml


def main() -> None:
    work_dir = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("/app/mount/work")
    manifest_path = work_dir / "episodes.yaml"
    data = yaml.safe_load(manifest_path.read_text(encoding="utf-8")) or {}

    ids = []
    for podcast in data.get("podcasts", []):
        for ep in podcast.get("episodes", []):
            # Fan out only episodes that have audio and are not yet backed up.
            if ep.get("audioFile") and not ep.get("backedUp", False):
                ids.append(ep["id"])

    print(json.dumps(ids))


if __name__ == "__main__":
    main()
