# Podcast TLDR — Backup (Stage E)

[![Test Status](https://github.com/jo-hoe/git-archive-backup/workflows/test/badge.svg)](https://github.com/jo-hoe/git-archive-backup/actions?workflow=test)
[![Lint Status](https://github.com/jo-hoe/git-archive-backup/workflows/lint/badge.svg)](https://github.com/jo-hoe/git-archive-backup/actions?workflow=lint)
[![Coverage Status](https://coveralls.io/repos/github/jo-hoe/git-archive-backup/badge.svg?branch=main)](https://coveralls.io/github/jo-hoe/git-archive-backup?branch=main)
[![Image Version](https://ghcr-badge.egpl.dev/jo-hoe/git-archive-backup/latest_tag?trim=major&label=image&color=blue)](https://github.com/jo-hoe/git-archive-backup/pkgs/container/git-archive-backup)

Stage **E** of the [podcast-tldr](https://github.com/jo-hoe/podcast-tldr) pipeline.

It reads the shared [`episodes.yaml`](https://github.com/jo-hoe/manifest-lib)
manifest, and for every episode that has a bundle it copies the zipped bundle and
the unzipped summary into a **private git archive repository**, commits, and pushes.
Each episode is stored under an intuitive, greppable layout and the episode is then
marked `backedUp` in the manifest.

```
[feeds] --> download --> transcribe --> summarize --> zip --> backup (this)
```

- **Input**: `<workDir>/episodes.yaml` + the bundle/summary artifacts it references
- **Output**: files committed to the archive repo, plus `backedUp: true` in the manifest

Archive layout:

```
podcasts/<show-slug>/<episode-slug>/summary.md
podcasts/<show-slug>/<episode-slug>/bundle.zip
```

where `<show-slug>` is the slugified show title and `<episode-slug>` is the episode ID.

> **The archive repository is PRIVATE.** Pushing to it requires an access token.
> The token is **never** stored in the config file, the ConfigMap, or on disk — it is
> always read from an environment variable (see [Configuration](#configuration)).

## Deployment Options

### Docker Compose

```bash
export PODCAST_TLDR_BACKUP_TOKEN=ghp_your_token_here
make start-docker      # builds the image and runs the stage once
```

The token is passed through to the container via the environment; the shared work
directory is mounted from `./volume/work`.

### Kubernetes (Helm)

The stage runs as a Kubernetes **Job** (run-once). Mount a `ReadWriteMany` PVC as
the shared work directory so this stage can read the artifacts produced upstream.
The access token is supplied via a Kubernetes **Secret** and injected as an
environment variable — it never appears in the ConfigMap.

```bash
helm install backup ./charts/git-archive-backup \
  --set workVolume.existingClaim=podcast-tldr-work \
  --set config.repoURL=https://github.com/jo-hoe/podcast-archive.git \
  --set-string secret.token=ghp_your_token_here
```

For production, manage the Secret out of band and reference it instead of letting
the chart create one:

```bash
kubectl create secret generic archive-token --from-literal=token=ghp_your_token_here

helm install backup ./charts/git-archive-backup \
  --set secret.create=false \
  --set secret.existingSecret=archive-token \
  --set config.repoURL=https://github.com/jo-hoe/podcast-archive.git
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
repoURL: https://github.com/jo-hoe/podcast-archive.git   # PRIVATE repo, required
tokenEnv: PODCAST_TLDR_BACKUP_TOKEN                        # env var holding the token
branch: main
authorName: git-archive-backup
authorEmail: git-archive-backup@users.noreply.github.com
# checkoutDir: ./mount/work/.backup-checkout              # defaults under workDir
```

The config path may be overridden with the `CONFIG_PATH` environment variable.

**Token / auth setup.** The access token is read from the environment variable named
by `tokenEnv` (default `PODCAST_TLDR_BACKUP_TOKEN`) — it is deliberately absent from
the config file so the secret never lands on disk. Provide it before running:

```bash
export PODCAST_TLDR_BACKUP_TOKEN=ghp_your_token_here
```

The token is used for HTTP basic auth against the archive repo (username `git`,
token as password), which works with GitHub/GitLab personal access tokens and
fine-grained tokens that have `contents: write` on the archive repository.

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

All git operations live behind a small `Repository` interface (`internal/gitrepo`),
so the service is unit-tested against an in-memory fake, and there is one
integration test that performs a real go-git round trip against a local bare repo
(no network required).

## Linting

Uses [`golangci-lint`](https://golangci-lint.run/). Run `make lint`.

## Limitations

- Episodes without a bundle are skipped; episodes already marked `backedUp` are not
  re-uploaded.
- A missing bundle/summary file for an episode is logged and skipped; it does not
  abort the run.
- All staged episodes are recorded in a single commit per run.

## Future Work

- Per-episode commits for finer-grained history.
- Verification that the pushed content matches the local artifacts.

## Relevant Links

- Orchestrator: [podcast-tldr](https://github.com/jo-hoe/podcast-tldr)
- Manifest contract: [manifest-lib](https://github.com/jo-hoe/manifest-lib)
- Git library: [github.com/go-git/go-git](https://github.com/go-git/go-git)
