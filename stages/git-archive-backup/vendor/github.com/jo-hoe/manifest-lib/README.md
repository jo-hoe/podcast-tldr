# Podcast TLDR — Manifest

[![Test Status](https://github.com/jo-hoe/manifest-lib/workflows/test/badge.svg)](https://github.com/jo-hoe/manifest-lib/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/manifest-lib/workflows/lint/badge.svg)](https://github.com/jo-hoe/manifest-lib/actions?workflow=lint)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/manifest-lib/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/manifest-lib?branch=main)

Shared Go library defining the **stage contract** for the
[podcast-tldr](https://github.com/jo-hoe/podcast-tldr) pipeline.

A single `episodes.yaml` file lives in a shared work directory and flows through
every stage:

```
download  ->  transcribe  ->  summarize  ->  zip  ->  backup
```

Each stage reads the manifest, produces its artifacts into the work directory,
appends the fields it owns, and writes the manifest back. Stages treat fields
owned by other stages as read-only — they only append, never mutate upstream data.

## Manifest shape

```yaml
podcasts:
  - showTitle: "Science Vs"
    showDescription: "..."
    feedURL: "https://.../rss"
    episodes:
      - id: "science-vs-42"                    # stable slug, drives folder naming
        title: "The A.I. Episode"
        published: 2026-01-15T00:00:00Z        # RFC3339
        description: "..."
        audioURL: "https://.../ep.mp3"
        audioFile: "audio/science-vs-42.mp3"   # set by download (A)
        transcriptFile: "transcripts/science-vs-42.json"  # set by transcribe (B)
        summaryFile: "summaries/science-vs-42.md"         # set by summarize (C)
        bundleFile: "bundles/science-vs-42.zip"           # set by zip (D)
        backedUp: true                                    # set by backup (E)
```

Artifact paths are always **work-relative with forward slashes** so the manifest
is portable across mounts and operating systems.

## Usage

```go
import "github.com/jo-hoe/manifest-lib"

m, err := manifest.Load(workDir)      // read episodes.yaml
// ... produce artifacts, set the fields this stage owns ...
ep.TranscriptFile = ep.TranscriptPath("json")
err = m.Save(workDir)                 // atomic write-back
```

Canonical sub-directories (`audio/`, `transcripts/`, `summaries/`, `bundles/`) and
the `Slugify` helper are exported so every stage names files identically.

## How to Use

```bash
make test   # run tests with coverage
make lint   # run golangci-lint
```

## Linting

Linting uses [`golangci-lint`](https://golangci-lint.run/). Run `make lint`.

## Relevant Links

- Orchestrator: [podcast-tldr](https://github.com/jo-hoe/podcast-tldr)
- [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3)
