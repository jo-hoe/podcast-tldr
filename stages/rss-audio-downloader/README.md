# Podcast TLDR — Download (Stage A)

[![Test Status](https://github.com/jo-hoe/rss-audio-downloader/workflows/test/badge.svg)](https://github.com/jo-hoe/rss-audio-downloader/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/rss-audio-downloader/workflows/lint/badge.svg)](https://github.com/jo-hoe/rss-audio-downloader/actions?workflow=lint)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/rss-audio-downloader/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/rss-audio-downloader?branch=main)
[![Image Version](https://ghcr-badge.egpl.dev/jo-hoe/rss-audio-downloader/latest_tag?trim=major&label=image&color=blue)](https://github.com/jo-hoe/rss-audio-downloader/pkgs/container/rss-audio-downloader)

Stage **A** of the [podcast-tldr](https://github.com/jo-hoe/podcast-tldr) pipeline.

It parses one or more podcast RSS feeds, selects episodes according to a per-feed
selector, downloads the selected episodes' audio into a shared work directory, and
writes an [`episodes.yaml`](https://github.com/jo-hoe/manifest-lib)
manifest that the downstream stages consume.

```
[feeds] --> download (this) --> transcribe --> summarize --> zip --> backup
```

- **Input**: `config.yaml` (feed URLs + selectors)
- **Output**: `<workDir>/audio/*.<ext>` + `<workDir>/episodes.yaml`

## Deployment Options

### Docker Compose

```bash
make start-docker      # builds the image and runs the stage once
```

Artifacts land in `./volume/work`.

### Kubernetes (Helm)

The stage runs as a Kubernetes **Job** (run-once). Mount a `ReadWriteMany` PVC as
the shared work directory so downstream stages can read the artifacts.

```bash
helm install download ./charts/rss-audio-downloader \
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
maxParallelDownloads: 4
feeds:
  - url: https://feeds.megaphone.fm/sciencevs
    selector:
      # Episodes are indexed chronologically (episode 1 = first/oldest ever published).
      # startEpisode and endEpisode define an inclusive window; omit either for open-ended.
      startEpisode: 1        # episode 1 = the very first episode of the podcast
      endEpisode: 3          # through the 3rd episode (chronologically)
      # startEpisode: 340, endEpisode: 342  →  three specific mid-run episodes
      # omit both                           →  all episodes

      # To fetch recent episodes, a date window is more practical than a high index:
      # startDate: 2026-09-01T00:00:00Z     →  everything published since Sep 2026
      # endDate:   2026-09-30T23:59:59Z     →  upper bound (optional)

      # Restrict by title (Go regexp syntax, optional):
      # nameRegex: "(?i)bread"
```

All active rules combine with **AND** semantics; omit any rule to leave that
dimension unconstrained. Episode IDs are generated as `show-YYYY-MM-DD-title`
(e.g. `science-vs-2016-06-28-sneak-peek`) — stable across runs regardless of feed
order or selection window. The config path may be overridden with the `CONFIG_PATH`
environment variable.

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

- Episodes without an audio enclosure in the feed are skipped.
- A failed episode download is logged and skipped; it does not abort the run.

## Future Work

- Optional resume/skip of already-downloaded episodes.
- Checksum verification of downloaded audio.

## Relevant Links

- Orchestrator: [podcast-tldr](https://github.com/jo-hoe/podcast-tldr)
- Manifest contract: [manifest-lib](https://github.com/jo-hoe/manifest-lib)
- Feed parsing: [github.com/mmcdole/gofeed](https://github.com/mmcdole/gofeed)
