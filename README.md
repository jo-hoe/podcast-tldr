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
| — | [podcast-tldr-manifest](https://github.com/jo-hoe/podcast-tldr-manifest) | Go | Shared stage contract (`episodes.yaml`) |
| A | [podcast-tldr-download](https://github.com/jo-hoe/podcast-tldr-download) | Go | RSS → audio + manifest |
| B | [podcast-tldr-transcribe](https://github.com/jo-hoe/podcast-tldr-transcribe) | Python | audio → transcript (faster-whisper) |
| C | [podcast-tldr-summarize](https://github.com/jo-hoe/podcast-tldr-summarize) | Go | transcript → summary (LiteLLM) |
| D | [podcast-tldr-zip](https://github.com/jo-hoe/podcast-tldr-zip) | Go | bundle transcript + metadata → `.zip` |
| E | [podcast-tldr-backup](https://github.com/jo-hoe/podcast-tldr-backup) | Go | push summary + bundle to a private archive |
| — | [podcast-tldr-archive](https://github.com/jo-hoe/podcast-tldr-archive) | — | Private backup target |

## Deployment Options

### Docker Compose (local, end-to-end)

Runs all five stages in order on a shared `./volume/work`. Summarization talks to
the keyless, OpenAI-compatible [ai-proxy](https://github.com/jo-hoe/ai-proxy)
(`https://ai-proxy.johoe.duckdns.org/openai/v1`, model `gpt-5`) — no LLM key needed.
An optional LiteLLM sidecar is included if you prefer to route through it instead.

```bash
# Backup pushes to the private archive repo. For a local run you can reuse the
# gh CLI's token (needs repo scope); or set a dedicated PAT.
export PODCAST_TLDR_BACKUP_TOKEN="$(gh auth token)"

make start-compose      # download -> transcribe -> summarize -> zip -> backup
```

Configure each stage in [`config/`](./config) and the LLM gateway in
[`litellm/config.yaml`](./litellm/config.yaml).

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
   [archive repo](https://github.com/jo-hoe/podcast-tldr-archive) under
   `podcasts/<show>/<episode>/`.

## The stage contract

A single `episodes.yaml` accumulates state as it flows through the stages
(`audioFile` → `transcriptFile` → `summaryFile` → `bundleFile` → `backedUp`). It is
defined and versioned in
[podcast-tldr-manifest](https://github.com/jo-hoe/podcast-tldr-manifest).

## Limitations

- On single-node k3d the work PVC is ReadWriteOnce; a real multi-node run needs an
  RWX storage class (NFS, Longhorn, …).
- The `k8s/10-jobs.yaml` Jobs do not self-sequence — use Argo, or apply/wait one at
  a time. `depends_on` handles ordering under compose.

## Future Work

- Fan-out over feeds/episodes with configurable parallelism in Argo.
- A single CLI to submit a run and tail progress.

## Relevant Links

- Manifest contract: [podcast-tldr-manifest](https://github.com/jo-hoe/podcast-tldr-manifest)
- Conventions mirror: [video-to-podcast-service](https://github.com/jo-hoe/video-to-podcast-service)
- [LiteLLM](https://docs.litellm.ai/), [faster-whisper](https://github.com/SYSTRAN/faster-whisper), [Argo Workflows](https://argo-workflows.readthedocs.io/)
