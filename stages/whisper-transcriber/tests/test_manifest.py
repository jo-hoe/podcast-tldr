"""Tests for transcribe.manifest, focusing on preserving other stages' fields."""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from transcribe import manifest as manifest_module
from transcribe.manifest import Episode, Manifest, ManifestError


def _write_manifest(work_dir: Path, data: dict) -> None:
    (work_dir / manifest_module.MANIFEST_FILENAME).write_text(
        yaml.safe_dump(data, sort_keys=False), encoding="utf-8"
    )


def test_published_rfc3339_survives_round_trip_as_string(tmp_path: Path) -> None:
    """Regression: the Go stages write `published` as RFC3339 and must read it back
    unchanged. PyYAML's default timestamp resolver would parse it to a datetime and
    re-emit `2026-09-24 09:00:00+00:00` (space-separated), which Go's RFC3339 parser
    rejects. The manifest must round-trip the exact string."""
    _write_manifest(
        tmp_path,
        {
            "podcasts": [
                {
                    "episodes": [
                        {
                            "id": "science-vs-001-stress",
                            "title": "Stress",
                            "published": "2026-09-24T09:00:00Z",
                            "audioFile": "audio/science-vs-001-stress.mp3",
                        }
                    ]
                }
            ]
        },
    )

    manifest = manifest_module.load(str(tmp_path))
    # published is not a modelled field, so it lives in extra and must survive.
    assert manifest.podcasts[0].episodes[0].extra["published"] == "2026-09-24T09:00:00Z"

    manifest_module.save(manifest, str(tmp_path))
    raw = (tmp_path / manifest_module.MANIFEST_FILENAME).read_text(encoding="utf-8")

    # The RFC3339 string survives verbatim (PyYAML may quote it, which Go's YAML
    # parser strips before RFC3339-parsing); what must NOT appear is the corrupt
    # space-separated datetime form that Go rejects.
    assert "2026-09-24T09:00:00Z" in raw
    assert "2026-09-24 09:00:00" not in raw  # no space-separated datetime form

    # And it re-loads unchanged.
    reloaded = manifest_module.load(str(tmp_path))
    assert reloaded.podcasts[0].episodes[0].extra["published"] == "2026-09-24T09:00:00Z"


def test_transcript_path_matches_go_convention() -> None:
    ep = Episode(id="science-vs-001-sourdough")
    assert ep.transcript_path("json") == "transcripts/science-vs-001-sourdough.json"
    assert ep.transcript_path(".json") == "transcripts/science-vs-001-sourdough.json"


def test_round_trip_preserves_other_stage_fields(tmp_path: Path) -> None:
    original = {
        "podcasts": [
            {
                "showTitle": "Science Vs",
                "showDescription": "facts vs everything",
                "feedURL": "https://example.com/rss",
                "author": "Spotify",
                "episodes": [
                    {
                        "id": "science-vs-001-sourdough",
                        "title": "Sourdough",
                        "published": "2026-01-15T00:00:00Z",
                        "description": "bread!",
                        "audioURL": "https://cdn/ep.mp3",
                        "audioFile": "audio/science-vs-001-sourdough.mp3",
                        "duration": "1830",
                        # downstream-owned fields that must survive:
                        "summaryFile": "summaries/science-vs-001-sourdough.md",
                        "bundleFile": "bundles/science-vs-001-sourdough.zip",
                        "backedUp": True,
                    }
                ],
            }
        ]
    }
    _write_manifest(tmp_path, original)

    manifest = manifest_module.load(str(tmp_path))
    # Simulate the transcribe stage writing its owned fields.
    ep = manifest.podcasts[0].episodes[0]
    ep.transcript_file = ep.transcript_path("json")
    ep.language = "en"
    ep.transcribe_model = "base"
    manifest_module.save(manifest, str(tmp_path))

    reloaded = yaml.safe_load(
        (tmp_path / manifest_module.MANIFEST_FILENAME).read_text(encoding="utf-8")
    )
    saved_ep = reloaded["podcasts"][0]["episodes"][0]

    # transcribe-owned fields written
    assert saved_ep["transcriptFile"] == "transcripts/science-vs-001-sourdough.json"
    assert saved_ep["language"] == "en"
    assert saved_ep["transcribeModel"] == "base"
    # upstream fields preserved
    assert saved_ep["audioFile"] == "audio/science-vs-001-sourdough.mp3"
    assert saved_ep["audioURL"] == "https://cdn/ep.mp3"
    assert saved_ep["duration"] == "1830"
    assert saved_ep["published"] == "2026-01-15T00:00:00Z"
    # downstream fields preserved untouched
    assert saved_ep["summaryFile"] == "summaries/science-vs-001-sourdough.md"
    assert saved_ep["bundleFile"] == "bundles/science-vs-001-sourdough.zip"
    assert saved_ep["backedUp"] is True
    # show-level fields preserved
    assert reloaded["podcasts"][0]["showTitle"] == "Science Vs"
    assert reloaded["podcasts"][0]["feedURL"] == "https://example.com/rss"


def test_empty_owned_fields_are_omitted(tmp_path: Path) -> None:
    _write_manifest(
        tmp_path,
        {"podcasts": [{"episodes": [{"id": "e1", "title": "t", "audioFile": "audio/e1.mp3"}]}]},
    )
    manifest = manifest_module.load(str(tmp_path))
    manifest_module.save(manifest, str(tmp_path))
    reloaded = yaml.safe_load(
        (tmp_path / manifest_module.MANIFEST_FILENAME).read_text(encoding="utf-8")
    )
    ep = reloaded["podcasts"][0]["episodes"][0]
    assert "transcriptFile" not in ep
    assert "language" not in ep


def test_save_is_atomic_no_temp_left(tmp_path: Path) -> None:
    manifest_module.save(Manifest(), str(tmp_path))
    leftovers = list(tmp_path.glob(".episodes-*.tmp"))
    assert leftovers == []
    assert (tmp_path / manifest_module.MANIFEST_FILENAME).exists()


def test_load_missing_is_error(tmp_path: Path) -> None:
    with pytest.raises(ManifestError):
        manifest_module.load(str(tmp_path))


def test_iter_episodes_spans_all_podcasts(tmp_path: Path) -> None:
    _write_manifest(
        tmp_path,
        {
            "podcasts": [
                {"episodes": [{"id": "a"}, {"id": "b"}]},
                {"episodes": [{"id": "c"}]},
            ]
        },
    )
    manifest = manifest_module.load(str(tmp_path))
    assert [e.id for e in manifest.iter_episodes()] == ["a", "b", "c"]
