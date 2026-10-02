# Podcast TLDR — Transcribe (Stage B)

[![test](https://github.com/jo-hoe/whisper-transcriber/actions/workflows/test.yml/badge.svg)](https://github.com/jo-hoe/whisper-transcriber/actions/workflows/test.yml)
[![Release Image](https://github.com/jo-hoe/whisper-transcriber/actions/workflows/image.yml/badge.svg)](https://github.com/jo-hoe/whisper-transcriber/actions/workflows/image.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Stage B of the [podcast-tldr](https://github.com/jo-hoe) pipeline. It reads the
shared `episodes.yaml` manifest produced by the download stage, transcribes each
episode that has downloaded audio using
[faster-whisper](https://github.com/SYSTRAN/faster-whisper) (CTranslate2), writes
one transcript JSON per episode, and appends the transcribe-owned fields
(`transcriptFile`, `language`, `transcribeModel`) back to the manifest.

Each stage owns only its own manifest fields; every other field it reads (from
upstream stages) or does not recognise (from downstream stages) is preserved
verbatim on save, so the manifest can flow through the whole pipeline without
data loss.

## Deployment Options

### Docker Compose

```sh
docker-compose up transcribe --build
```

This mounts `./config` (the config file), `./volume/work` (the shared work
directory containing `episodes.yaml` and the `audio/`/`transcripts/` folders), and
`./volume/models` (the faster-whisper model cache) into the container.

### Kubernetes (Helm)

The stage runs to completion as a Kubernetes `Job`. In a real pipeline the work
directory is backed by a `ReadWriteMany` PVC shared with the other stages, and the
model cache is backed by its own PVC so models are not re-downloaded on every run.

```sh
helm install whisper-transcriber \
  --set workVolume.existingClaim=podcast-tldr-work \
  --set modelVolume.existingClaim=podcast-tldr-models \
  charts/whisper-transcriber
```

The chart requests 1Gi and limits 2Gi of memory to give the Whisper model room.

### Local k3d Development Cluster

```sh
make start-k3d   # create cluster, build & push image to local registry, deploy chart
make stop-k3d    # tear the cluster down
```

## How to Use

### Initial Setup

```sh
make install        # pip install -e .[dev]
make install-hooks  # optional: install the ruff pre-commit hook
```

Place the shared `episodes.yaml` (from the download stage) in the work directory
configured by `workDir`, alongside the `audio/` files it references. Then run:

```sh
python main.py --config ./config/config.yaml
```

The stage writes `transcripts/<id>.json` for every transcribed episode and updates
`episodes.yaml` in place.

### Configuration

Configuration is a single YAML file. Its path comes from the `CONFIG_PATH`
environment variable, falling back to `./config/config.yaml`. Keys are camelCase;
every field is optional and defaults are applied.

| Key              | Default            | Description |
| ---------------- | ------------------ | ----------- |
| `logLevel`       | `info`             | `debug`, `info`, `warn`, or `error`. |
| `workDir`        | `./mount/work`     | Shared work directory holding `episodes.yaml` and artifacts. |
| `modelName`      | `base`             | faster-whisper model (`tiny`/`base`/`small`/`medium`/`large-v3`). |
| `modelCachePath` | *(empty)*          | Model download/cache directory (`download_root`). Empty uses the faster-whisper default. |
| `device`         | `cpu`              | Compute device: `cpu`, `cuda`, or `auto`. |
| `computeType`    | `int8`             | CTranslate2 compute type, e.g. `int8`, `float16`. |
| `outputFormat`   | `json`             | Transcript output format. Only `json` is supported today. |

### CLI

```
usage: main.py [-h] [--config CONFIG]

options:
  -h, --help       show this help message and exit
  --config CONFIG  path to the config file (overrides the CONFIG_PATH env var)
```

The transcript JSON has the shape:

```json
{
  "text": "full transcript ...",
  "segments": [{ "start": 0.0, "end": 1.5, "text": "hello" }],
  "language": "en",
  "model": "base",
  "duration": 1830.0
}
```

## Linting

```sh
make lint     # ruff check .
make format   # ruff check --fix .
```

## Limitations

- Only the `json` transcript output format is implemented.
- Transcription runs on CPU by default; GPU (`device: cuda`) requires a suitable
  CUDA/CTranslate2 runtime in the image, which is not provided here.
- Whisper models are downloaded on first use; the first run for a given model is
  slower and requires network access unless the model cache is pre-populated.

## Future Work

- Additional output formats (SRT/VTT).
- Word-level timestamps and speaker diarization.
- A CUDA-enabled image variant for GPU transcription.

## Relevant Links

- [faster-whisper](https://github.com/SYSTRAN/faster-whisper)
- [CTranslate2](https://github.com/OpenNMT/CTranslate2)
- [OpenAI Whisper](https://github.com/openai/whisper)
