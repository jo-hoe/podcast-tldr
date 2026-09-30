# Podcast TLDR

Turn podcasts into durable, searchable notes. This is the **orchestrator** for a
pipeline of small, reusable, independently-runnable containers that download a
podcast, transcribe it, summarize it, bundle the artifacts, and back everything up
to a private git archive.

```
 A. download  ->  B. transcribe  ->  C. summarize  ->  D. zip  ->  E. backup
   (RSS ->        (audio ->          (transcript ->     (bundle    (archive to
    audio +        transcript)        summary.md)        .zip)      private repo)
    manifest)
```

Every stage is its own repository and container image, wired together by a shared
`episodes.yaml` manifest that flows through a shared work directory. Stages are
pure: each reads the manifest, produces its artifacts, appends the fields it owns,
and writes the manifest back.

## Repositories

| Stage | Repo | Language | Role |
|------|------|----------|------|
| — | [manifest-lib](https://github.com/jo-hoe/manifest-lib) | Go | Shared stage contract (`episodes.yaml`) |
| A | [rss-audio-downloader](https://github.com/jo-hoe/rss-audio-downloader) | Go | RSS → audio + manifest |
| B | [whisper-transcriber](https://github.com/jo-hoe/whisper-transcriber) | Python | audio → transcript (faster-whisper) |
| C | [llm-summarizer](https://github.com/jo-hoe/llm-summarizer) | Go | transcript → summary (LiteLLM) |
| D | [artifact-zipper](https://github.com/jo-hoe/artifact-zipper) | Go | bundle transcript + metadata → `.zip` |
| E | [git-archive-backup](https://github.com/jo-hoe/git-archive-backup) | Go | push summary + bundle to a private archive |
| — | [media-archive](https://github.com/jo-hoe/media-archive) | — | Private backup target |

## Quickstart

Run the whole pipeline locally with Docker Compose. You need Docker, the `gh` CLI
(for the backup token), and the five stage repos checked out as siblings under `../`
(only if you want to build images locally — otherwise the published `ghcr.io` images
are pulled for you).

```bash
# 1. Point the pipeline at the episode(s) you want (see Example configuration below).
$EDITOR config/download.yaml

# 2. Backup pushes to your private archive repo. Reuse the gh CLI token (repo scope)
#    or export a dedicated PAT. Summarization uses the keyless ai-proxy, so no LLM key.
export PODCAST_TLDR_BACKUP_TOKEN="$(gh auth token)"

# 3. Run all five stages in order on a shared ./volume/work.
docker compose up -d      # download -> transcribe -> summarize -> zip -> backup

# 4. Watch it run; the chain is sequenced, so wait on the last stage.
docker compose logs -f backup

# 5. Your report + bundle are committed to the private archive repo under
#    podcasts/<show>/<episode>/{summary.md,bundle.zip}, and ./volume/work holds
#    the intermediate artifacts (audio/, transcripts/, summaries/, bundles/).
```

> **Running locally built images.** To use images built from the sibling repos
> instead of `ghcr.io`, add the override file:
> `docker compose -f docker-compose.yml -f docker-compose.local.yml up -d`.
> Build them first with `make build-and-push` (k3d) or `docker build` per repo.

> **Note.** Don't use `--abort-on-container-exit` with this compose file — the stages
> are a sequential `depends_on` chain, and that flag tears down the whole run the
> moment the first stage exits. Run detached (`-d`) and follow the `backup` logs.

## Example configuration

Each stage reads one small YAML file from [`config/`](./config). The only file you
normally edit is `config/download.yaml` — which feed(s) to pull and which episodes:

```yaml
# config/download.yaml — what to fetch.
logLevel: info
workDir: /app/mount/work        # shared work dir (mounted); leave as-is
maxParallelDownloads: 4
feeds:
  - url: https://podcasts.files.bbci.co.uk/p0my6g8q.rss   # the RSS feed
    selector:
      # Episodes are newest-first. Pick ONE selection style:
      startEpisode: 1           # index range (1 = latest). Here: just the latest.
      endEpisode: 1
      # startDate: 2026-01-01   # …or a publish-date window (RFC3339), each optional
      # endDate:   2026-12-31
      # nameRegex: "(?i)series 5"   # …or a title regex. Empty = all episodes.
```

The other stage configs work out of the box; tune them only if you need to:

| File | Key knobs |
|------|-----------|
| [`config/transcribe.yaml`](./config/transcribe.yaml) | `modelName` (`base`/`small`/…), `device` (`cpu`/`cuda`), `computeType` |
| [`config/summarize.yaml`](./config/summarize.yaml) | `baseURL`, `model` (`gpt-5`), `reasoningModel`, `maxTokens` |
| [`config/prompt.txt`](./config/prompt.txt) | the report instructions (metadata-aware, error-correcting) |
| [`config/backup.yaml`](./config/backup.yaml) | `repoURL` (your archive repo), `branch`, `authorName`/`authorEmail` |

To route summarization through your own LLM instead of the keyless ai-proxy, point
`config/summarize.yaml`'s `baseURL` at the bundled LiteLLM sidecar
(`http://litellm:4000/v1`) and configure the model in
[`litellm/config.yaml`](./litellm/config.yaml).

## Deployment Options

### Docker Compose (local, end-to-end)

See [Quickstart](#quickstart) for the minimal run. Runs all five stages in order on
a shared `./volume/work`. Summarization talks to the keyless, OpenAI-compatible
[ai-proxy](https://github.com/jo-hoe/ai-proxy)
(`https://ai-proxy.johoe.duckdns.org/openai/v1`, model `gpt-5`) — no LLM key needed.
An optional LiteLLM sidecar is included if you prefer to route through it instead.

### Kubernetes — plain Jobs

```bash
kubectl apply -f k8s/00-pvc.yaml -f k8s/01-configmap.yaml
# Summarize uses the keyless ai-proxy, so only the backup token is required.
kubectl create secret generic podcast-tldr-secrets \
  --from-literal=litellmApiKey="none" \
  --from-literal=backupToken="$(gh auth token)"
make run-jobs           # applies k8s/10-jobs.yaml (apply/wait in order)
```

### Kubernetes — Argo Workflows

The [`argo/`](./argo) `WorkflowTemplate` chains the stages as a DAG over the shared
work PVC, with a configurable `parallelism` limit.

```bash
make install-argo
make deploy-argo-template
make run-argo
```

### Local k3d Development Cluster

Builds every stage image from the sibling repos, pushes to a local registry, and
deploys the shared resources — mirroring the workflow of
[video-to-podcast-service](https://github.com/jo-hoe/video-to-podcast-service).

```bash
make start-k3d          # create cluster + build/push all images + deploy
# ... run the pipeline via `make run-jobs` or Argo ...
make stop-k3d
```

## How to Use

1. Edit [`config/download.yaml`](./config/download.yaml) — the feed URL(s) and
   episode selectors (index range, publish-date range, and/or a title regex).
2. Point [`litellm/config.yaml`](./litellm/config.yaml) at your preferred model.
3. Tune the summarization prompt in [`config/prompt.txt`](./config/prompt.txt).
4. Run the pipeline via compose, Jobs, or Argo.
5. Find summaries + bundles committed to your private
   [archive repo](https://github.com/jo-hoe/media-archive) under
   `podcasts/<show>/<episode>/`.

## The stage contract

A single `episodes.yaml` accumulates state as it flows through the stages
(`audioFile` → `transcriptFile` → `summaryFile` → `bundleFile` → `backedUp`). It is
defined and versioned in
[manifest-lib](https://github.com/jo-hoe/manifest-lib).

## Limitations

- On single-node k3d the work PVC is ReadWriteOnce; a real multi-node run needs an
  RWX storage class (NFS, Longhorn, …).
- The `k8s/10-jobs.yaml` Jobs do not self-sequence — use Argo, or apply/wait one at
  a time. `depends_on` handles ordering under compose.

## Future Work

- Fan-out over feeds/episodes with configurable parallelism in Argo.
- A single CLI to submit a run and tail progress.

## Relevant Links

- Manifest contract: [manifest-lib](https://github.com/jo-hoe/manifest-lib)
- Conventions mirror: [video-to-podcast-service](https://github.com/jo-hoe/video-to-podcast-service)
- [LiteLLM](https://docs.litellm.ai/), [faster-whisper](https://github.com/SYSTRAN/faster-whisper), [Argo Workflows](https://argo-workflows.readthedocs.io/)
