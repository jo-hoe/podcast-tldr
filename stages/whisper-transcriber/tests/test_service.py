"""Tests for the TranscribeService orchestration using a fake transcriber."""

from __future__ import annotations

import json
from pathlib import Path

import yaml
from conftest import FailingTranscriber, FakeTranscriber

from transcribe import manifest as manifest_module
from transcribe.config import Config
from transcribe.service import TranscribeService


def _seed(work_dir: Path, episodes: list[dict]) -> None:
    (work_dir / manifest_module.MANIFEST_FILENAME).write_text(
        yaml.safe_dump({"podcasts": [{"showTitle": "Show", "episodes": episodes}]}),
        encoding="utf-8",
    )


def _make_audio(work_dir: Path, rel: str) -> None:
    path = work_dir / Path(*rel.split("/"))
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(b"fake-audio")


def _svc(work_dir: Path, fake, *, workers: int = 1) -> TranscribeService:
    """Helper: build a TranscribeService with a factory that always returns ``fake``."""
    return TranscribeService(
        Config(work_dir=str(work_dir), max_parallel_episodes=workers),
        lambda: fake,
    )


def test_run_transcribes_and_updates_manifest(tmp_path: Path) -> None:
    _seed(
        tmp_path,
        [
            {"id": "ep-1", "title": "One", "audioFile": "audio/ep-1.mp3"},
            {"id": "ep-2", "title": "Two", "audioFile": "audio/ep-2.mp3"},
        ],
    )
    _make_audio(tmp_path, "audio/ep-1.mp3")
    _make_audio(tmp_path, "audio/ep-2.mp3")

    fake = FakeTranscriber()
    _svc(tmp_path, fake).run()

    assert len(fake.calls) == 2

    # transcript files written with the expected content
    transcript = json.loads((tmp_path / "transcripts" / "ep-1.json").read_text("utf-8"))
    assert transcript["text"] == "hello world"
    assert transcript["language"] == "en"
    assert transcript["model"] == "fake-model"
    assert len(transcript["segments"]) == 2
    assert transcript["segments"][0] == {"start": 0.0, "end": 1.5, "text": "hello"}

    # manifest updated with transcribe-owned fields
    reloaded = yaml.safe_load(
        (tmp_path / manifest_module.MANIFEST_FILENAME).read_text("utf-8")
    )
    ep = reloaded["podcasts"][0]["episodes"][0]
    assert ep["transcriptFile"] == "transcripts/ep-1.json"
    assert ep["language"] == "en"
    assert ep["transcribeModel"] == "fake-model"


def test_run_skips_episode_without_audio(tmp_path: Path) -> None:
    _seed(
        tmp_path,
        [
            {"id": "has-audio", "title": "A", "audioFile": "audio/has-audio.mp3"},
            {"id": "no-audio", "title": "B"},
        ],
    )
    _make_audio(tmp_path, "audio/has-audio.mp3")

    fake = FakeTranscriber()
    _svc(tmp_path, fake).run()

    assert fake.calls == [str(tmp_path / "audio" / "has-audio.mp3")]
    assert not (tmp_path / "transcripts" / "no-audio.json").exists()


def test_run_skips_episode_with_missing_audio_file(tmp_path: Path) -> None:
    _seed(tmp_path, [{"id": "ghost", "title": "G", "audioFile": "audio/ghost.mp3"}])
    # deliberately do not create the audio file

    fake = FakeTranscriber()
    _svc(tmp_path, fake).run()

    assert fake.calls == []
    assert not (tmp_path / "transcripts" / "ghost.json").exists()


def test_run_isolates_per_episode_failure(tmp_path: Path) -> None:
    _seed(tmp_path, [{"id": "boom", "title": "B", "audioFile": "audio/boom.mp3"}])
    _make_audio(tmp_path, "audio/boom.mp3")

    _svc(tmp_path, FailingTranscriber()).run()

    # manifest still written, no transcript, no transcribe fields set
    reloaded = yaml.safe_load(
        (tmp_path / manifest_module.MANIFEST_FILENAME).read_text("utf-8")
    )
    ep = reloaded["podcasts"][0]["episodes"][0]
    assert "transcriptFile" not in ep
    assert not (tmp_path / "transcripts" / "boom.json").exists()


def test_run_parallel_transcribes_all_episodes(tmp_path: Path) -> None:
    """maxParallelEpisodes > 1 should still transcribe all episodes correctly."""
    _seed(
        tmp_path,
        [{"id": f"ep-{i}", "title": f"Ep {i}", "audioFile": f"audio/ep-{i}.mp3"}
         for i in range(4)],
    )
    for i in range(4):
        _make_audio(tmp_path, f"audio/ep-{i}.mp3")

    fake = FakeTranscriber()
    _svc(tmp_path, fake, workers=2).run()

    assert len(fake.calls) == 4
    for i in range(4):
        assert (tmp_path / "transcripts" / f"ep-{i}.json").exists()
