"""CLI entry point for the transcribe stage."""

from __future__ import annotations

import argparse
import logging
import os
import sys

from transcribe.config import ConfigError, load_config, resolve_path
from transcribe.manifest import ManifestError
from transcribe.service import TranscribeService
from transcribe.transcriber import FasterWhisperTranscriber

_LOG_LEVELS = {
    "debug": logging.DEBUG,
    "info": logging.INFO,
    "warn": logging.WARNING,
    "warning": logging.WARNING,
    "error": logging.ERROR,
}

# Env var for single-episode fan-out mode (consistent with Go stages using EPISODE_ID).
_ENV_EPISODE_ID = "EPISODE_ID"


def main(argv: list[str] | None = None) -> int:
    args = _parse_args(argv)
    _init_logging(logging.INFO)

    config_path = args.config or resolve_path()
    try:
        config = load_config(config_path)
    except ConfigError as exc:
        logging.error("failed to load config: path=%s err=%s", config_path, exc)
        return 1

    _init_logging(_LOG_LEVELS.get(config.log_level.lower(), logging.INFO))
    logging.info(
        "configuration loaded: workDir=%s model=%s device=%s computeType=%s",
        config.work_dir,
        config.model_name,
        config.device,
        config.compute_type,
    )

    service = TranscribeService(
        config,
        lambda: FasterWhisperTranscriber(config),
        episode_id=args.episode_id or os.environ.get(_ENV_EPISODE_ID) or None,
    )
    try:
        service.run()
    except ManifestError as exc:
        logging.error("transcribe stage failed: %s", exc)
        return 1
    return 0


def _parse_args(argv: list[str] | None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        prog="whisper-transcriber",
        description="Transcribe downloaded podcast audio into transcript JSON files.",
    )
    parser.add_argument(
        "--config",
        default=None,
        help="Path to config.yaml (overrides the CONFIG_PATH env var).",
    )
    parser.add_argument(
        "--episode-id",
        default=None,
        metavar="ID",
        help="Process only this episode ID (default: process all eligible episodes).",
    )
    return parser.parse_args(argv)


def _init_logging(level: int) -> None:
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
        force=True,
    )


if __name__ == "__main__":
    sys.exit(main())
