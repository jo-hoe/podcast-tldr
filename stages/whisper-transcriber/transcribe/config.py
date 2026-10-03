"""Configuration loading for the transcribe stage.

Mirrors the org convention used by the Go stages: a single YAML file whose path is
taken from the ``CONFIG_PATH`` environment variable, falling back to
``./config/config.yaml`` under the current working directory. Keys are camelCase,
defaults are applied for every optional field, and relative paths are resolved
against the current working directory.
"""

from __future__ import annotations

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml

ENV_CONFIG_PATH = "CONFIG_PATH"
# Runtime override for maxParallelEpisodes — set via k8s/Argo without changing the ConfigMap.
ENV_MAX_PARALLEL_EPISODES = "WHISPER_MAX_PARALLEL_EPISODES"

_VALID_LOG_LEVELS = frozenset({"debug", "info", "warn", "warning", "error"})
_VALID_DEVICES = frozenset({"cpu", "cuda", "auto"})


@dataclass(slots=True)
class Config:
    """Typed transcribe-stage configuration.

    Attributes:
        log_level: One of debug, info, warn, error. Defaults to ``info``.
        work_dir: Shared work directory holding ``episodes.yaml`` and artifacts.
        model_name: faster-whisper model to load (e.g. ``tiny``/``base``/``small``/
            ``medium``/``large-v3``). Defaults to ``base``.
        model_cache_path: Directory where models are downloaded/cached
            (passed to faster-whisper as ``download_root``). Empty means the
            faster-whisper default cache.
        device: Compute device: ``cpu`` (default), ``cuda`` or ``auto``.
        compute_type: CTranslate2 compute type, e.g. ``int8`` (default), ``float16``.
        output_format: Transcript output format. Only ``json`` is supported today.
        max_parallel_episodes: Maximum number of episodes to transcribe concurrently.
            Defaults to 1 (sequential). On CPU, each Whisper instance uses multiple
            threads internally, so values above the number of physical cores bring
            diminishing returns. On CUDA, set to 1 (GPU handles parallelism internally).
    """

    log_level: str = "info"
    work_dir: str = "./mount/work"
    model_name: str = "base"
    model_cache_path: str = ""
    device: str = "cpu"
    compute_type: str = "int8"
    output_format: str = "json"
    max_parallel_episodes: int = 1
    language: str = ""  # force transcription language (e.g. "en"); empty = auto-detect


def resolve_path() -> str:
    """Return the config path from ``CONFIG_PATH`` or the default location."""
    override = os.environ.get(ENV_CONFIG_PATH)
    if override:
        return override
    return str(Path.cwd() / "config" / "config.yaml")


def load_config(config_path: str) -> Config:
    """Read, parse, default, resolve and validate the config at ``config_path``."""
    try:
        raw = Path(config_path).read_text(encoding="utf-8")
    except OSError as exc:
        raise ConfigError(f"failed to read config file {config_path}: {exc}") from exc

    data = yaml.safe_load(raw) or {}
    if not isinstance(data, dict):
        raise ConfigError(f"config file {config_path} must contain a YAML mapping")

    cfg = _from_mapping(data)
    _make_paths_absolute(cfg)
    _validate(cfg)
    return cfg


class ConfigError(ValueError):
    """Raised when the configuration is missing or invalid."""


def _from_mapping(data: dict[str, Any]) -> Config:
    """Build a Config from a camelCase mapping, applying dataclass defaults."""
    defaults = Config()
    cfg = Config(
        log_level=_str(data, "logLevel", defaults.log_level),
        work_dir=_str(data, "workDir", defaults.work_dir),
        model_name=_str(data, "modelName", defaults.model_name),
        model_cache_path=_str(data, "modelCachePath", defaults.model_cache_path),
        device=_str(data, "device", defaults.device),
        compute_type=_str(data, "computeType", defaults.compute_type),
        output_format=_str(data, "outputFormat", defaults.output_format),
        max_parallel_episodes=_int(data, "maxParallelEpisodes", defaults.max_parallel_episodes),
        language=_str(data, "language", defaults.language),
    )
    # Allow WHISPER_MAX_PARALLEL_EPISODES env var to override config (e.g. from Argo parameter).
    env_parallel = os.environ.get(ENV_MAX_PARALLEL_EPISODES, "").strip()
    if env_parallel:
        try:
            cfg.max_parallel_episodes = int(env_parallel)
        except ValueError:
            pass
    return cfg


def _int(data: dict[str, Any], key: str, default: int) -> int:
    """Return the int value for ``key``, or ``default`` when absent/None."""
    if key not in data or data[key] is None:
        return default
    try:
        return int(data[key])
    except (TypeError, ValueError):
        return default


def _str(data: dict[str, Any], key: str, default: str) -> str:
    """Return the string value for ``key``, or ``default`` when the key is absent.

    A key that is present but empty is kept as-is (an empty string) so that
    validation can reject fields that must not be blank — an explicit empty value
    is a configuration error, not a request for the default.
    """
    if key not in data or data[key] is None:
        return default
    return str(data[key])


def _make_paths_absolute(cfg: Config) -> None:
    cfg.work_dir = _absolute(cfg.work_dir)
    if cfg.model_cache_path:
        cfg.model_cache_path = _absolute(cfg.model_cache_path)


def _absolute(path: str) -> str:
    p = Path(path)
    if p.is_absolute():
        return str(p)
    return str(Path.cwd() / p)


def _validate(cfg: Config) -> None:
    if cfg.log_level.lower() not in _VALID_LOG_LEVELS:
        raise ConfigError(f"invalid logLevel {cfg.log_level!r}")
    if cfg.device.lower() not in _VALID_DEVICES:
        raise ConfigError(f"invalid device {cfg.device!r}")
    if cfg.output_format.lower() != "json":
        raise ConfigError(
            f"unsupported outputFormat {cfg.output_format!r} (only 'json' is supported)"
        )
    if cfg.model_name.strip() == "":
        raise ConfigError("modelName must not be empty")
