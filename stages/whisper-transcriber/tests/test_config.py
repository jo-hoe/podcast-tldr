"""Tests for transcribe.config."""

from __future__ import annotations

from pathlib import Path

import pytest

from transcribe.config import Config, ConfigError, load_config, resolve_path


def _write(tmp_path: Path, body: str) -> str:
    path = tmp_path / "config.yaml"
    path.write_text(body, encoding="utf-8")
    return str(path)


def test_load_applies_defaults(tmp_path: Path) -> None:
    cfg = load_config(_write(tmp_path, "workDir: /tmp/work\n"))
    assert cfg.log_level == "info"
    assert cfg.model_name == "base"
    assert cfg.device == "cpu"
    assert cfg.compute_type == "int8"
    assert cfg.output_format == "json"


def test_load_parses_all_fields(tmp_path: Path) -> None:
    cfg = load_config(
        _write(
            tmp_path,
            """
logLevel: debug
workDir: /data/work
modelName: small
modelCachePath: /models
device: cuda
computeType: float16
outputFormat: json
""",
        )
    )
    assert cfg.log_level == "debug"
    assert Path(cfg.work_dir).is_absolute()
    assert cfg.work_dir.replace("\\", "/").endswith("data/work")
    assert cfg.model_name == "small"
    assert Path(cfg.model_cache_path).is_absolute()
    assert cfg.model_cache_path.replace("\\", "/").endswith("models")
    assert cfg.device == "cuda"
    assert cfg.compute_type == "float16"


def test_load_makes_work_dir_absolute(tmp_path: Path) -> None:
    cfg = load_config(_write(tmp_path, "workDir: ./relative/work\n"))
    assert Path(cfg.work_dir).is_absolute()


def test_empty_config_uses_all_defaults(tmp_path: Path) -> None:
    cfg = load_config(_write(tmp_path, ""))
    assert cfg == Config(work_dir=cfg.work_dir)
    assert Path(cfg.work_dir).is_absolute()


@pytest.mark.parametrize(
    "body",
    [
        "logLevel: verbose\n",
        "device: tpu\n",
        "outputFormat: srt\n",
        "modelName: ''\n",
    ],
)
def test_invalid_values_are_rejected(tmp_path: Path, body: str) -> None:
    with pytest.raises(ConfigError):
        load_config(_write(tmp_path, body))


def test_missing_file_is_config_error(tmp_path: Path) -> None:
    with pytest.raises(ConfigError):
        load_config(str(tmp_path / "does-not-exist.yaml"))


def test_resolve_path_prefers_env(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("CONFIG_PATH", "/custom/config.yaml")
    assert resolve_path() == "/custom/config.yaml"


def test_resolve_path_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("CONFIG_PATH", raising=False)
    assert resolve_path().endswith(str(Path("config") / "config.yaml"))
