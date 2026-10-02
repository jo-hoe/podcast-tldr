"""Audio-to-transcript abstraction.

``Transcriber`` is a small ABC so the orchestration layer never depends on
faster-whisper directly and tests can inject a fake. The real implementation
(:class:`FasterWhisperTranscriber`) imports faster-whisper lazily, inside
``transcribe``, so importing this module — and running unit tests with a fake —
does not require faster-whisper to be installed.
"""

from __future__ import annotations

from abc import ABC, abstractmethod
from dataclasses import dataclass, field

from transcribe.config import Config


@dataclass(slots=True)
class Segment:
    """A single timestamped transcript segment."""

    start: float
    end: float
    text: str


@dataclass(slots=True)
class TranscriptionResult:
    """The full result of transcribing one audio file."""

    text: str
    segments: list[Segment] = field(default_factory=list)
    language: str = ""
    model: str = ""
    duration: float = 0.0


class Transcriber(ABC):
    """Transcribes an audio file into a :class:`TranscriptionResult`."""

    @abstractmethod
    def transcribe(self, audio_path: str) -> TranscriptionResult:
        """Transcribe the audio at ``audio_path``."""
        raise NotImplementedError


class FasterWhisperTranscriber(Transcriber):
    """Production transcriber backed by faster-whisper (CTranslate2).

    The model is loaded once (lazily, on first use) and reused across episodes.
    """

    def __init__(self, config: Config) -> None:
        self._config = config
        self._model = None  # loaded lazily to keep imports/tests light

    def _load_model(self):
        if self._model is not None:
            return self._model

        # Imported here so the module (and the test suite using a fake) does not
        # require faster-whisper to be installed.
        from faster_whisper import WhisperModel

        kwargs: dict[str, object] = {
            "device": self._config.device,
            "compute_type": self._config.compute_type,
        }
        if self._config.model_cache_path:
            kwargs["download_root"] = self._config.model_cache_path

        self._model = WhisperModel(self._config.model_name, **kwargs)
        return self._model

    def transcribe(self, audio_path: str) -> TranscriptionResult:
        model = self._load_model()
        raw_segments, info = model.transcribe(audio_path)

        segments = [
            Segment(start=float(s.start), end=float(s.end), text=s.text.strip())
            for s in raw_segments
        ]
        text = " ".join(s.text for s in segments).strip()

        return TranscriptionResult(
            text=text,
            segments=segments,
            language=getattr(info, "language", "") or "",
            model=self._config.model_name,
            duration=float(getattr(info, "duration", 0.0) or 0.0),
        )
