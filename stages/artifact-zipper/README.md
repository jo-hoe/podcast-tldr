# Podcast TLDR — Zip (Stage D)

[![Test Status](https://github.com/jo-hoe/artifact-zipper/workflows/test/badge.svg)](https://github.com/jo-hoe/artifact-zipper/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/artifact-zipper/workflows/lint/badge.svg)](https://github.com/jo-hoe/artifact-zipper/actions?workflow=lint)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/artifact-zipper/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/artifact-zipper?branch=main)
[![Image Version](https://ghcr-badge.egpl.dev/jo-hoe/artifact-zipper/latest_tag?trim=major&label=image&color=blue)](https://github.com/jo-hoe/artifact-zipper/pkgs/container/artifact-zipper)

Stage **D** of the [podcast-tldr](https://github.com/jo-hoe/podcast-tldr) pipeline.

It reads the shared [`episodes.yaml`](https://github.com/jo-hoe/manifest-lib)
manifest and, for every episode that has a transcript, builds a zip bundle
containing the transcript plus a generated `metadata.yaml` describing the podcast
and episode (the intermediate states). It records the bundle path back on the
manifest for the downstream backup stage.

```
[feeds] --> download --> transcribe --> summarize --> zip (this) --> backup
```

- **Input**: `<workDir>/episodes.yaml` + `<workDir>/transcripts/*.json`
- **Output**: `<workDir>/bundles/<id>.zip` + updated `<workDir>/episodes.yaml` (`bundleFile` set)

> **The summary markdown is intentionally NOT placed inside the zip.** It stays
> unzipped in `<workDir>/summaries/` so the backup stage (and humans) can read it
> directly without unpacking the archive. Each bundle instead embeds a
> `metadata.yaml` whose `transcriptEntry` field ties the archived transcript back
> to its podcast and episode, so a bundle is self-describing on its own.

## Deployment Options

### Docker Compose

```bash
make start-docker      # builds the image and runs the stage once
```

Artifacts land in `./volume/work`.

### Kubernetes (Helm)

The stage runs as a Kubernetes **Job** (run-once). Mount a `ReadWriteMany` PVC as
the shared work directory so it can read the transcripts produced upstream and
write the bundles the backup stage consumes.

```bash
helm install zip ./charts/artifact-zipper \
  --set workVolume.existingClaim=podcast-tldr-work
```

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
```

The config path may be overridden with the `CONFIG_PATH` environment variable.

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

- Episodes without a transcript are logged and skipped; they produce no bundle.
- A failed bundle write is logged and skipped; it does not abort the run.

## Future Work

- Optional checksum manifest embedded alongside the bundled files.
- Configurable inclusion of the source audio in the bundle.

## Relevant Links

- Orchestrator: [podcast-tldr](https://github.com/jo-hoe/podcast-tldr)
- Manifest contract: [manifest-lib](https://github.com/jo-hoe/manifest-lib)
