"""Shared pytest fixtures for the transcribe stage tests."""

from __future__ import annotations

import pytest

from transcribe.transcriber import Segment, Transcriber, TranscriptionResult


class FakeTranscriber(Transcriber):
    """A Transcriber that returns canned segments without loading any model.

    Records the audio paths it was asked to transcribe so tests can assert which
    episodes were processed.
    """

    def __init__(self, result: TranscriptionResult | None = None) -> None:
        self.calls: list[str] = []
        self._result = result or TranscriptionResult(
            text="hello world",
            segments=[
                Segment(start=0.0, end=1.5, text="hello"),
                Segment(start=1.5, end=3.0, text="world"),
            ],
            language="en",
            model="fake-model",
            duration=3.0,
        )

    def transcribe(self, audio_path: str) -> TranscriptionResult:
        self.calls.append(audio_path)
        return self._result


class FailingTranscriber(Transcriber):
    """A Transcriber that always raises, to exercise per-episode error isolation."""

    def transcribe(self, audio_path: str) -> TranscriptionResult:
        raise RuntimeError("boom")


@pytest.fixture
def fake_transcriber() -> FakeTranscriber:
    return FakeTranscriber()
