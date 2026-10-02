"""podcast-tldr transcribe stage (B).

Transcribes downloaded podcast audio into transcript JSON files using
faster-whisper, updating the shared ``episodes.yaml`` manifest in place.
"""

from transcribe.config import Config, load_config
from transcribe.manifest import Episode, Manifest, Podcast
from transcribe.service import TranscribeService
from transcribe.transcriber import (
    FasterWhisperTranscriber,
    Segment,
    Transcriber,
    TranscriptionResult,
)

__all__ = [
    "Config",
    "load_config",
    "Episode",
    "Manifest",
    "Podcast",
    "TranscribeService",
    "FasterWhisperTranscriber",
    "Segment",
    "Transcriber",
    "TranscriptionResult",
]
