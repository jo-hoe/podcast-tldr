"""Transcribe-stage orchestration.

Iterates the manifest, transcribes each episode that has downloaded audio, writes
a transcript JSON file per episode, and appends the transcribe-owned fields to the
manifest before saving it back.

When ``max_parallel_episodes > 1``, episodes are transcribed concurrently using a
thread pool. Each worker thread gets its own Transcriber instance (CTranslate2 /
WhisperModel is not thread-safe when shared across threads).
"""

from __future__ import annotations

import json
import logging
import threading
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path
from typing import Callable

from transcribe import manifest as manifest_module
from transcribe.config import Config
from transcribe.manifest import Episode
from transcribe.transcriber import Transcriber, TranscriptionResult

logger = logging.getLogger(__name__)

# Type alias: a zero-argument callable that returns a fresh Transcriber.
TranscriberFactory = Callable[[], Transcriber]


class TranscribeService:
    """Runs the transcribe stage using an injected :class:`TranscriberFactory`."""

    def __init__(self, config: Config, transcriber_factory: TranscriberFactory) -> None:
        self._config = config
        self._factory = transcriber_factory

    def run(self) -> None:
        """Transcribe every eligible episode and write the manifest back.

        Per-episode failures are logged and skipped so one bad episode does not
        abort the whole run. When ``max_parallel_episodes > 1`` a thread pool is
        used; each worker owns its own Transcriber to avoid shared-state issues.
        """
        manifest = manifest_module.load(self._config.work_dir)
        episodes = list(manifest.iter_episodes())

        workers = max(1, self._config.max_parallel_episodes)
        if workers == 1:
            results = [self._process_episode_threadsafe(ep) for ep in episodes]
        else:
            logger.info("transcribing with %d parallel workers", workers)
            results = self._run_parallel(episodes, workers)

        transcribed = sum(1 for ok in results if ok)
        manifest_module.save(manifest, self._config.work_dir)
        logger.info(
            "transcribe stage complete: work_dir=%s transcribed=%d",
            self._config.work_dir,
            transcribed,
        )

    def _run_parallel(self, episodes: list[Episode], workers: int) -> list[bool]:
        """Transcribe episodes concurrently; each thread owns one Transcriber."""
        # Thread-local storage so each thread creates exactly one model instance.
        tls = threading.local()

        def worker(episode: Episode) -> bool:
            if not hasattr(tls, "transcriber"):
                tls.transcriber = self._factory()
            return self._process_episode(episode, tls.transcriber)

        results: dict[int, bool] = {}
        with ThreadPoolExecutor(max_workers=workers) as pool:
            futures = {pool.submit(worker, ep): i for i, ep in enumerate(episodes)}
            for future in as_completed(futures):
                idx = futures[future]
                try:
                    results[idx] = future.result()
                except Exception as exc:  # noqa: BLE001
                    logger.error("unexpected worker error: %s", exc)
                    results[idx] = False
        return [results.get(i, False) for i in range(len(episodes))]

    def _process_episode_threadsafe(self, episode: Episode) -> bool:
        """Single-threaded path: create one transcriber lazily and reuse it."""
        if not hasattr(self, "_transcriber"):
            self._transcriber = self._factory()
        return self._process_episode(episode, self._transcriber)

    def _process_episode(self, episode: Episode, transcriber: Transcriber) -> bool:
        """Transcribe one episode. Returns True if a transcript was produced."""
        if not episode.audio_file:
            logger.info("skipping episode without audio: id=%s", episode.id)
            return False

        audio_path = Path(self._config.work_dir) / _to_os_path(episode.audio_file)
        if not audio_path.exists():
            logger.error("audio file missing: id=%s path=%s", episode.id, audio_path)
            return False

        try:
            result = transcriber.transcribe(str(audio_path))
        except Exception as exc:  # noqa: BLE001 - isolate per-episode failures
            logger.error("transcription failed: id=%s err=%s", episode.id, exc)
            return False

        rel_transcript = episode.transcript_path("json")
        self._write_transcript(rel_transcript, result)

        episode.transcript_file = rel_transcript
        episode.language = result.language
        episode.transcribe_model = result.model
        logger.info("transcribed: id=%s file=%s", episode.id, rel_transcript)
        return True

    def _write_transcript(self, rel_path: str, result: TranscriptionResult) -> None:
        dest = Path(self._config.work_dir) / _to_os_path(rel_path)
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(
            json.dumps(_result_to_dict(result), ensure_ascii=False, indent=2),
            encoding="utf-8",
        )


def _result_to_dict(result: TranscriptionResult) -> dict:
    return {
        "text": result.text,
        "segments": [
            {"start": s.start, "end": s.end, "text": s.text} for s in result.segments
        ],
        "language": result.language,
        "model": result.model,
        "duration": result.duration,
    }


def _to_os_path(work_relative: str) -> Path:
    """Convert a forward-slash work-relative manifest path to an OS-native Path."""
    return Path(*work_relative.split("/"))
