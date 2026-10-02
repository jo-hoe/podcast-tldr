# Podcast TLDR — Summarize (Stage C)

[![Test Status](https://github.com/jo-hoe/llm-summarizer/workflows/test/badge.svg)](https://github.com/jo-hoe/llm-summarizer/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/llm-summarizer/workflows/lint/badge.svg)](https://github.com/jo-hoe/llm-summarizer/actions?workflow=lint)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/llm-summarizer/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/llm-summarizer?branch=main)
[![Image Version](https://ghcr-badge.egpl.dev/jo-hoe/llm-summarizer/latest_tag?trim=major&label=image&color=blue)](https://github.com/jo-hoe/llm-summarizer/pkgs/container/llm-summarizer)

Stage **C** of the [podcast-tldr](https://github.com/jo-hoe/podcast-tldr) pipeline.

It reads the shared [`episodes.yaml`](https://github.com/jo-hoe/manifest-lib)
manifest and each episode's transcript, summarizes the transcript with an LLM
through an OpenAI-compatible [LiteLLM](https://github.com/BerriAI/litellm) proxy,
writes `summaries/<id>.md` into the shared work directory, and records the summary
path and model back on the manifest.

```
[feeds] --> download --> transcribe --> summarize (this) --> zip --> backup
```

- **Input**: `<workDir>/episodes.yaml` + `<workDir>/<transcriptFile>` per episode
- **Output**: `<workDir>/summaries/<id>.md` + updated `episodes.yaml`

## Deployment Options

### Docker Compose

```bash
export LITELLM_API_KEY=sk-...   # read from the environment, never stored in config
make start-docker               # builds the image and runs the stage once
```

Artifacts land in `./volume/work`.

### Kubernetes (Helm)

The stage runs as a Kubernetes **Job** (run-once). Mount a `ReadWriteMany` PVC as
the shared work directory so it can read the transcripts produced upstream and
write the summaries. The LLM API key is supplied through a Secret.

```bash
helm install summarize ./charts/llm-summarizer \
  --set workVolume.existingClaim=podcast-tldr-work \
  --set apiKey.existingSecret=litellm-credentials \
  --set apiKey.existingSecretKey=LITELLM_API_KEY \
  --set config.baseURL=http://litellm:4000/v1 \
  --set config.model=summarizer
```

The config (including the *name* of the API-key environment variable) is rendered
into a ConfigMap; the API key itself is only ever mounted from a Secret.

### Local k3d Development Cluster

```bash
make start-k3d   # create cluster + build/push image + deploy chart
make stop-k3d
```

## How to Use

### Initial Setup

```bash
make install-hooks   # installs a pre-commit go-fmt hook
```

### Configuration

`config/config.yaml`:

```yaml
logLevel: info
workDir: ./mount/work
baseURL: http://litellm:4000/v1   # OpenAI-compatible proxy endpoint
model: summarizer                 # model name configured in the proxy
apiKeyEnv: LITELLM_API_KEY         # env var holding the key (key is never in config)
promptPath: ./config/prompt.txt    # summarization prompt template
maxTokens: 1024
temperature: 0.3
```

`baseURL` and `model` are required; everything else has a default. The config path
may be overridden with the `CONFIG_PATH` environment variable. The API key is read
from the environment variable named by `apiKeyEnv` at runtime and is never written
to the manifest or the config file.

The prompt template (`config/prompt.txt`) is sent as the system instruction and the
transcript text as the user message.

### Transcript Format

Transcripts are the JSON produced by the transcribe stage: an object with a `text`
field (the full transcript) and/or a `segments` array of `{start, end, text}`
objects. The stage uses `text` when present and otherwise joins the segment texts,
so it tolerates either shape.

## Development

Dependencies are **vendored** (the shared `manifest-lib` module is resolved
via a local `replace` during development), so all builds, tests, and image builds
run with `-mod=vendor` and never require the sibling checkout. After changing
dependencies run `make vendor`.

```bash
make build    # go build
make test     # go test with coverage
make lint     # golangci-lint
```

## Linting

Uses [`golangci-lint`](https://golangci-lint.run/). Run `make lint`.

## Limitations

- Episodes without a `transcriptFile` are skipped.
- A per-episode failure (unreadable transcript, LLM error) is logged and skipped; it
  does not abort the whole run.
- The stage does not chunk very long transcripts; a transcript that exceeds the
  model's context window is sent as-is and may be truncated by the proxy.

## Future Work

- Chunked/map-reduce summarization for transcripts longer than the context window.
- Skip episodes that already have an up-to-date summary.
- Configurable summary structure per podcast.

## Relevant Links

- Orchestrator: [podcast-tldr](https://github.com/jo-hoe/podcast-tldr)
- Manifest contract: [manifest-lib](https://github.com/jo-hoe/manifest-lib)
- LLM client: [github.com/sashabaranov/go-openai](https://github.com/sashabaranov/go-openai)
- Proxy: [LiteLLM](https://github.com/BerriAI/litellm)
