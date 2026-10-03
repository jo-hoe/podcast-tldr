#!/usr/bin/env bash
# k8s-fanout.sh — fan-out the pipeline in Kubernetes using per-episode Jobs.
#
# Reads episodes.yaml from the work PVC via a temporary pod, then creates
# per-episode Job chains (transcribe->summarize->zip->backup) with EPISODE_ID set.
# Jobs run in parallel up to PARALLELISM active Jobs at once.
#
# Prerequisites: download Job must have completed successfully first.
#   kubectl wait --for=condition=complete job/rss-audio-downloader --timeout=2h
#
# Usage:
#   bash scripts/k8s-fanout.sh [PARALLELISM]   (default: 4)
#
# For k3d (local images):
#   Use `make run-argo` instead — Argo handles fan-out natively with withParam.

set -euo pipefail

PARALLELISM="${1:-4}"
NAMESPACE="${KUBECTL_NAMESPACE:-default}"
WORK_PVC="podcast-tldr-work"

echo "=== Reading episode IDs from PVC ==="
EPISODES_JSON=$(kubectl run scatter-tmp \
  --image=python:3.12-slim --restart=Never --rm -i \
  --overrides="{\"spec\":{\"volumes\":[{\"name\":\"work\",\"persistentVolumeClaim\":{\"claimName\":\"${WORK_PVC}\"}}],\"containers\":[{\"name\":\"scatter-tmp\",\"image\":\"python:3.12-slim\",\"volumeMounts\":[{\"name\":\"work\",\"mountPath\":\"/app/mount/work\"}]}]}}" \
  -n "$NAMESPACE" \
  -- python3 -c "
import json, yaml, pathlib
data = yaml.safe_load(pathlib.Path('/app/mount/work/episodes.yaml').read_text()) or {}
ids = [ep['id'] for pod in data.get('podcasts',[]) for ep in pod.get('episodes',[])
       if ep.get('audioFile') and not ep.get('backedUp', False)]
print(json.dumps(ids))
" 2>/dev/null)

EPISODE_IDS=$(echo "$EPISODES_JSON" | python3 -c "import json,sys; [print(i) for i in json.load(sys.stdin)]")
TOTAL=$(echo "$EPISODE_IDS" | grep -c . || true)
echo "=== $TOTAL episodes to fan out (parallelism=$PARALLELISM) ==="

active=0
while IFS= read -r ep_id; do
  # Wait if at max parallelism
  while [ "$active" -ge "$PARALLELISM" ]; do
    sleep 5
    active=$(kubectl get jobs -n "$NAMESPACE" \
      -l "podcast-tldr/fanout=true" \
      --field-selector=status.active=1 \
      --no-headers 2>/dev/null | grep -c . || true)
  done

  slug="${ep_id:0:52}"  # k8s name max 63 chars; leave room for suffix
  echo "Submitting: $ep_id"

  # Create a single Job that runs all 4 stages sequentially via initContainers.
  cat <<EOF | kubectl apply -n "$NAMESPACE" -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: fanout-$(echo "$slug" | tr '.' '-' | tr '_' '-')
  labels:
    podcast-tldr/fanout: "true"
    podcast-tldr/episode: "$(echo "$slug" | tr '.' '-' | tr '_' '-')"
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      securityContext: { runAsNonRoot: true, runAsUser: 65532, fsGroup: 65532 }
      initContainers:
        - name: transcribe
          image: ghcr.io/jo-hoe/podcast-tldr/whisper-transcriber:latest
          command: ["python3.12", "/app/main.py", "--episode-id", "${ep_id}"]
          env:
            - { name: CONFIG_PATH, value: /app/config/transcribe.yaml }
            - { name: EPISODE_ID, value: "${ep_id}" }
          resources:
            requests: { cpu: "500m", memory: 2Gi }
            limits: { cpu: "2", memory: 4Gi }
          volumeMounts:
            - { name: work, mountPath: /app/mount/work }
            - { name: models, mountPath: /app/mount/models }
            - { name: config, mountPath: /app/config, readOnly: true }
        - name: summarize
          image: ghcr.io/jo-hoe/podcast-tldr/llm-summarizer:latest
          env:
            - { name: CONFIG_PATH, value: /app/config/summarize.yaml }
            - { name: EPISODE_ID, value: "${ep_id}" }
            - name: LITELLM_API_KEY
              valueFrom: { secretKeyRef: { name: podcast-tldr-secrets, key: litellmApiKey } }
          volumeMounts:
            - { name: work, mountPath: /app/mount/work }
            - { name: config, mountPath: /app/config, readOnly: true }
        - name: zip
          image: ghcr.io/jo-hoe/podcast-tldr/artifact-zipper:latest
          env:
            - { name: CONFIG_PATH, value: /app/config/zip.yaml }
            - { name: EPISODE_ID, value: "${ep_id}" }
          volumeMounts:
            - { name: work, mountPath: /app/mount/work }
            - { name: config, mountPath: /app/config, readOnly: true }
      containers:
        - name: backup
          image: ghcr.io/jo-hoe/podcast-tldr/git-archive-backup:latest
          env:
            - { name: CONFIG_PATH, value: /app/config/backup.yaml }
            - { name: EPISODE_ID, value: "${ep_id}" }
            - name: PODCAST_TLDR_BACKUP_TOKEN
              valueFrom: { secretKeyRef: { name: podcast-tldr-secrets, key: backupToken } }
            - name: PODCAST_TLDR_BACKUP_REPO
              valueFrom: { secretKeyRef: { name: podcast-tldr-secrets, key: backupRepoURL } }
          volumeMounts:
            - { name: work, mountPath: /app/mount/work }
            - { name: config, mountPath: /app/config, readOnly: true }
      volumes:
        - { name: work, persistentVolumeClaim: { claimName: podcast-tldr-work } }
        - { name: models, persistentVolumeClaim: { claimName: podcast-tldr-models } }
        - { name: config, configMap: { name: podcast-tldr-config } }
EOF
  active=$((active + 1))
done <<< "$EPISODE_IDS"

echo "=== All $TOTAL fan-out Jobs submitted ==="
echo "Watch: kubectl get jobs -l podcast-tldr/fanout=true -w"
