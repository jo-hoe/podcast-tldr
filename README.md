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

Run the whole pipeline locally with Docker Compose. You need Docker and the `gh` CLI
(for the backup token). The published `ghcr.io` images are pulled automatically; no
local builds are required for a first run.

```bash
# 1. Create your personal config overrides (gitignored — never committed).
cp config/download.yaml config/download.local.yaml
cp config/backup.yaml   config/backup.local.yaml

# 2. Edit your feed URL + episode selector.
$EDITOR config/download.local.yaml   # set url: and selector: (see Example configuration)

# 3. Set your private archive repo and point the pipeline at your local configs.
$EDITOR config/backup.local.yaml     # set repoURL: https://github.com/you/your-archive.git
```

Then export the backup token and start the pipeline:

```bash
export PODCAST_TLDR_BACKUP_TOKEN="$(gh auth token)"

# Use ghcr.io images (default) — or add -f docker-compose.local.yml for locally built ones.
docker compose up -d download transcribe summarize zip backup

# Watch the run — stages sequence automatically; follow the last stage.
docker compose logs -f backup
```

When it completes, your reports + bundles are committed to the private archive repo
under `podcasts/<show>/<episode>/{summary.md,bundle.zip}`. Intermediate artifacts
(audio, transcripts, summaries, bundles) land in `./volume/work`.

> **Config override pattern.** The committed `config/*.yaml` files are safe generic
> examples — they are safe to commit because they contain no secrets or personal data.
> For your real feed URL, archive repo, and other personal values, copy the relevant
> file to `config/*.local.yaml` and edit there. Those files are gitignored and will
> never be committed. To make the containers pick them up, either edit
> `config/*.yaml` directly (fine for a one-off run) or create a compose override
> file that remounts the `.local.yaml` paths — see the comment in
> [docker-compose.local.yml](./docker-compose.local.yml).

> **Note.** Don't use `--abort-on-container-exit` — the stages are a sequential
> `depends_on` chain and that flag tears everything down the moment the first stage
> exits. Always run detached (`-d`) and follow `backup` logs.

## Example configuration

Each stage reads one small YAML file from [`config/`](./config). The committed files
are safe generic examples — copy any to `config/*.local.yaml` for your real values
(gitignored, never committed).

### Feed selector (`config/download.yaml`)

The `selector` block defines a **window** of episodes to fetch from a feed. Episodes
are indexed **newest-first** (index 1 = most recent). All rules combine with AND;
omit a rule to leave that dimension unconstrained.

```yaml
# config/download.yaml
logLevel: info
workDir: /app/mount/work   # shared mount; leave as-is
maxParallelDownloads: 4
feeds:
  - url: https://feeds.megaphone.fm/sciencevs   # RSS feed URL
    selector:
      # Episodes are indexed chronologically (episode 1 = first/oldest ever published).
      # startEpisode and endEpisode define an inclusive window; omit either for open-ended.
      startEpisode: 1    # episode 1 = the very first episode of the podcast
      endEpisode: 1      # same → fetch only the single first episode
      # startEpisode: 1, endEpisode: 10    →  first 10 episodes ever published
      # startEpisode: 340, endEpisode: 342 →  three specific mid-run episodes
      # omit both                          →  all episodes

      # --- Publish-date window (RFC3339, each bound optional) ---
      # startDate: 2026-01-01T00:00:00Z
      # endDate:   2026-12-31T23:59:59Z

      # --- Title filter (Go regexp, optional) ---
      # nameRegex: "(?i)series 5"

  # Add more feeds as needed:
  # - url: https://podcasts.files.bbci.co.uk/p0my6g8q.rss
  #   selector:
  #     startEpisode: 1
  #     endEpisode: 5
```

### Other stage configs

The other stage configs work out of the box; tune them only if you need to:

| File | Key knobs |
|------|-----------|
| [`config/transcribe.yaml`](./config/transcribe.yaml) | `modelName` (`base`/`small`/`medium`/`large-v3`), `device` (`cpu`/`cuda`), `computeType` (`int8`/`float16`) |
| [`config/summarize.yaml`](./config/summarize.yaml) | `baseURL` (LLM endpoint), `model`, `reasoningModel` (omits temperature, uses `max_completion_tokens`), `maxTokens` |
| [`config/prompt.txt`](./config/prompt.txt) | Report instructions — metadata-aware, silently corrects mistranscriptions, outputs outline + takeaways + suggestions |
| [`config/backup.yaml`](./config/backup.yaml) | `repoURL` (your private archive repo), `tokenEnv`, `branch`, `authorName`/`authorEmail` |

Summarization talks to the keyless [ai-proxy](https://github.com/jo-hoe/ai-proxy)
by default (`config/summarize.yaml` → `baseURL: https://ai-proxy.johoe.duckdns.org/openai/v1`,
model `gpt-5`) — no LLM key needed. To use your own model, point `baseURL` at the
bundled LiteLLM sidecar (`http://litellm:4000/v1`) and configure it in
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
