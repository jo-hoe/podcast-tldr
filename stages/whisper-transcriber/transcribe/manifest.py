"""Read/write access to the shared ``episodes.yaml`` manifest.

The manifest schema is defined canonically by the Go ``manifest-lib``
module. This module mirrors that schema for the transcribe stage. Fields owned by
other stages (and any future fields this stage does not know about) are preserved
verbatim on round-trip: the dataclasses keep an ``extra`` mapping so nothing is
lost when the manifest is written back.

Paths stored in the manifest are always work-relative with forward slashes.
"""

from __future__ import annotations

import contextlib
import os
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

import yaml

MANIFEST_FILENAME = "episodes.yaml"


class _NoTimestampLoader(yaml.SafeLoader):
    """SafeLoader that does not auto-convert ISO-8601 scalars into ``datetime``.

    The manifest is a language-neutral contract: the Go stages write ``published``
    as an RFC3339 string and expect it back verbatim. PyYAML's default implicit
    ``timestamp`` resolver would parse it into a ``datetime`` and re-emit it in a
    space-separated form (``2026-09-24 09:00:00+00:00``) that Go's RFC3339 parser
    rejects. Dropping that resolver keeps such values as plain strings so unknown
    upstream fields round-trip byte-for-byte.
    """


_NoTimestampLoader.yaml_implicit_resolvers = {
    ch: [(tag, regexp) for tag, regexp in resolvers if tag != "tag:yaml.org,2002:timestamp"]
    for ch, resolvers in yaml.SafeLoader.yaml_implicit_resolvers.items()
}

# Canonical artifact sub-directories (must match the Go manifest constants).
AUDIO_DIR = "audio"
TRANSCRIPT_DIR = "transcripts"
SUMMARY_DIR = "summaries"
BUNDLE_DIR = "bundles"


@dataclass(slots=True)
class Episode:
    """A single podcast episode and the artifacts produced for it.

    Only the fields the transcribe stage reads or writes are modelled explicitly;
    every other key present in the YAML is retained in ``extra`` and re-emitted on
    save so upstream/downstream data is never dropped.
    """

    id: str = ""
    title: str = ""
    audio_file: str = ""
    transcript_file: str = ""
    language: str = ""
    transcribe_model: str = ""
    extra: dict[str, Any] = field(default_factory=dict)

    # --- keys this dataclass models explicitly (everything else -> extra) ---
    _KNOWN = frozenset(
        {"id", "title", "audioFile", "transcriptFile", "language", "transcribeModel"}
    )

    @classmethod
    def from_mapping(cls, data: dict[str, Any]) -> Episode:
        extra = {k: v for k, v in data.items() if k not in cls._KNOWN}
        return cls(
            id=str(data.get("id", "")),
            title=str(data.get("title", "")),
            audio_file=str(data.get("audioFile", "")),
            transcript_file=str(data.get("transcriptFile", "")),
            language=str(data.get("language", "")),
            transcribe_model=str(data.get("transcribeModel", "")),
            extra=extra,
        )

    def to_mapping(self) -> dict[str, Any]:
        """Serialize back to a camelCase mapping, dropping empty owned fields.

        ``extra`` is spread first so explicitly-modelled fields take precedence,
        and empty owned strings are omitted to match the Go ``omitempty`` output.
        """
        out: dict[str, Any] = dict(self.extra)
        out["id"] = self.id
        out["title"] = self.title
        _set_if(out, "audioFile", self.audio_file)
        _set_if(out, "transcriptFile", self.transcript_file)
        _set_if(out, "language", self.language)
        _set_if(out, "transcribeModel", self.transcribe_model)
        return out

    def transcript_path(self, ext: str = "json") -> str:
        """Work-relative transcript path, matching Go ``Episode.TranscriptPath``."""
        suffix = ext if ext.startswith(".") else f".{ext}"
        return f"{TRANSCRIPT_DIR}/{self.id}{suffix}"


@dataclass(slots=True)
class Podcast:
    """A show and its selected episodes. Unknown keys are preserved in ``extra``."""

    episodes: list[Episode] = field(default_factory=list)
    extra: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_mapping(cls, data: dict[str, Any]) -> Podcast:
        episodes = [Episode.from_mapping(e) for e in (data.get("episodes") or [])]
        extra = {k: v for k, v in data.items() if k != "episodes"}
        return cls(episodes=episodes, extra=extra)

    def to_mapping(self) -> dict[str, Any]:
        out: dict[str, Any] = dict(self.extra)
        out["episodes"] = [e.to_mapping() for e in self.episodes]
        return out


@dataclass(slots=True)
class Manifest:
    """Root manifest document that flows through the pipeline."""

    podcasts: list[Podcast] = field(default_factory=list)

    @classmethod
    def from_mapping(cls, data: dict[str, Any]) -> Manifest:
        podcasts = [Podcast.from_mapping(p) for p in (data.get("podcasts") or [])]
        return cls(podcasts=podcasts)

    def to_mapping(self) -> dict[str, Any]:
        return {"podcasts": [p.to_mapping() for p in self.podcasts]}

    def iter_episodes(self):
        """Yield every episode across all podcasts."""
        for podcast in self.podcasts:
            yield from podcast.episodes


def load(work_dir: str) -> Manifest:
    """Load the manifest from ``episodes.yaml`` inside ``work_dir``."""
    path = Path(work_dir) / MANIFEST_FILENAME
    try:
        raw = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise ManifestError(f"failed to read manifest {path}: {exc}") from exc

    data = yaml.load(raw, Loader=_NoTimestampLoader) or {}
    if not isinstance(data, dict):
        raise ManifestError(f"manifest {path} must contain a YAML mapping")
    return Manifest.from_mapping(data)


def save(manifest: Manifest, work_dir: str) -> None:
    """Atomically write the manifest to ``episodes.yaml`` inside ``work_dir``."""
    work = Path(work_dir)
    work.mkdir(parents=True, exist_ok=True)
    path = work / MANIFEST_FILENAME

    text = yaml.safe_dump(manifest.to_mapping(), sort_keys=False, allow_unicode=True)

    # Write to a temp file in the same directory, then atomically rename into place.
    fd, tmp_name = tempfile.mkstemp(dir=str(work), prefix=".episodes-", suffix=".tmp")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as handle:
            handle.write(text)
        os.replace(tmp_name, path)
    except OSError as exc:
        _silent_remove(tmp_name)
        raise ManifestError(f"failed to write manifest {path}: {exc}") from exc


class ManifestError(RuntimeError):
    """Raised when the manifest cannot be read or written."""


def _set_if(out: dict[str, Any], key: str, value: str) -> None:
    if value:
        out[key] = value


def _silent_remove(path: str) -> None:
    with contextlib.suppress(OSError):
        os.remove(path)
