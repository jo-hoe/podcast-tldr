"""Transcribe-stage orchestration.

Iterates the manifest, transcribes each episode that has downloaded audio, writes
a transcript JSON file per episode, and appends the transcribe-owned fields to the
manifest before saving it back.
"""

from __future__ import annotations

import json
import logging
from pathlib import Path

from transcribe import manifest as manifest_module
from transcribe.config import Config
from transcribe.manifest import Episode
from transcribe.transcriber import Transcriber, TranscriptionResult

logger = logging.getLogger(__name__)


class TranscribeService:
    """Runs the transcribe stage using an injected :class:`Transcriber`."""

    def __init__(self, config: Config, transcriber: Transcriber) -> None:
        self._config = config
        self._transcriber = transcriber

    def run(self) -> None:
        """Transcribe every eligible episode and write the manifest back.

        Per-episode failures are logged and skipped so one bad episode does not
        abort the whole run — matching the download stage's behaviour.
        """
        manifest = manifest_module.load(self._config.work_dir)

        transcribed = 0
        for episode in manifest.iter_episodes():
            if self._process_episode(episode):
                transcribed += 1

        manifest_module.save(manifest, self._config.work_dir)
        logger.info(
            "transcribe stage complete: work_dir=%s transcribed=%d",
            self._config.work_dir,
            transcribed,
        )

    def _process_episode(self, episode: Episode) -> bool:
        """Transcribe one episode. Returns True if a transcript was produced."""
        if not episode.audio_file:
            logger.info("skipping episode without audio: id=%s", episode.id)
            return False

        audio_path = Path(self._config.work_dir) / _to_os_path(episode.audio_file)
        if not audio_path.exists():
            logger.error("audio file missing: id=%s path=%s", episode.id, audio_path)
            return False

        try:
            result = self._transcriber.transcribe(str(audio_path))
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
