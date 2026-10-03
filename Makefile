include help.mk

ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# Stage names — each maps to stages/<name>/ and the local image name.
STAGES := rss-audio-downloader whisper-transcriber llm-summarizer artifact-zipper git-archive-backup
IMAGE_VERSION := 1.0.0

.DEFAULT_GOAL := help

# --- Local end-to-end via docker compose ---

.PHONY: start-compose
start-compose: ## run the full pipeline locally via docker compose (pulls ghcr images)
	docker compose up -d download transcribe summarize zip backup
	docker compose logs -f backup

.PHONY: stop-compose
stop-compose: ## stop and remove the compose pipeline
	docker compose down

.PHONY: clean-work
clean-work: ## delete local pipeline artifacts under ./volume
	@rm -rf ${ROOT_DIR}volume/work ${ROOT_DIR}volume/models

# --- K3d cluster management ---

.PHONY: create-cluster
create-cluster: ## create the local k3d cluster
	@k3d cluster create --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml

.PHONY: build-and-push
build-and-push: ## build every stage image from stages/ and push to the local k3d registry
	@for stage in $(STAGES); do \
		echo "building $$stage"; \
		docker build -t localhost:5000/$$stage:${IMAGE_VERSION} ${ROOT_DIR}stages/$$stage; \
		docker push localhost:5000/$$stage:${IMAGE_VERSION}; \
	done

.PHONY: create-secret
create-secret: ## create the pipeline secret (prompts for values)
	@echo "Creating podcast-tldr-secrets..."
	@kubectl create secret generic podcast-tldr-secrets \
		--from-literal=litellmApiKey="$${LITELLM_API_KEY:-none}" \
		--from-literal=backupToken="$${PODCAST_TLDR_BACKUP_TOKEN:-$$(gh auth token)}" \
		--from-literal=backupRepoURL="$${PODCAST_TLDR_BACKUP_REPO:?set PODCAST_TLDR_BACKUP_REPO to your archive repo URL}" \
		--dry-run=client -o yaml | kubectl apply -f -

.PHONY: deploy
deploy: ## apply all shared resources to the cluster using the k3d overlay (local registry images)
	@kubectl apply -k ${ROOT_DIR}k8s/overlays/k3d
	@echo ""
	@echo "Next: create the secret with:"
	@echo "  make create-secret PODCAST_TLDR_BACKUP_REPO=https://github.com/you/archive.git"

.PHONY: deploy-plain
deploy-plain: ## apply shared resources using ghcr.io images (non-k3d clusters)
	@kubectl apply -f ${ROOT_DIR}k8s/00-pvc.yaml
	@kubectl apply -f ${ROOT_DIR}k8s/01-configmap.yaml
	@kubectl apply -f ${ROOT_DIR}k8s/03-litellm.yaml
	@kubectl apply -f ${ROOT_DIR}k8s/10-jobs.yaml
	@echo ""
	@echo "Next: create the secret with:"
	@echo "  make create-secret PODCAST_TLDR_BACKUP_REPO=https://github.com/you/archive.git"

.PHONY: run-jobs
run-jobs: ## run the pipeline as plain sequential k8s Jobs (serial, all episodes)
	@kubectl apply -f ${ROOT_DIR}k8s/10-jobs.yaml

.PHONY: run-fanout-jobs
run-fanout-jobs: ## run per-episode fan-out Jobs in k8s (incremental archive commits)
	@bash ${ROOT_DIR}scripts/k8s-fanout.sh

.PHONY: run-fanout-compose
run-fanout-compose: ## run per-episode fan-out locally via docker compose
	@bash ${ROOT_DIR}scripts/fan-out.sh

.PHONY: install-argo
install-argo: ## install Argo Workflows into the cluster (UI auth disabled for dev)
	@kubectl create namespace argo || true
	@kubectl apply -n argo -f https://github.com/argoproj/argo-workflows/releases/latest/download/quick-start-minimal.yaml
	@echo "Patching argo-server to use --auth-mode=client (no login required for dev)..."
	@kubectl patch deployment argo-server -n argo \
		--type='json' \
		-p='[{"op":"replace","path":"/spec/template/spec/containers/0/args","value":["server","--auth-mode=client","--secure=false"]}]'
	@kubectl rollout status deployment/argo-server -n argo --timeout=120s
	@echo "Argo UI available at http://localhost:2746 (no login required)"

.PHONY: argo-ui
argo-ui: ## open the Argo Workflows UI (k3d: http://localhost:2746; other: port-forward)
	@if kubectl config current-context 2>/dev/null | grep -q k3d; then \
		echo "Argo UI: http://localhost:2746"; \
		open http://localhost:2746 2>/dev/null || xdg-open http://localhost:2746 2>/dev/null || start http://localhost:2746 2>/dev/null || true; \
	else \
		echo "Port-forwarding Argo UI to http://localhost:2746 (Ctrl-C to stop)..."; \
		kubectl port-forward -n argo svc/argo-server 2746:2746; \
	fi

.PHONY: deploy-argo-template
deploy-argo-template: ## register the podcast-tldr Argo WorkflowTemplate
	@kubectl apply -f ${ROOT_DIR}argo/workflow-template.yaml

.PHONY: run-argo
run-argo: ## submit one pipeline run via Argo
	@argo submit ${ROOT_DIR}argo/workflow-run.yaml --watch

.PHONY: start-k3d
start-k3d: create-cluster build-and-push deploy install-argo deploy-argo-template ## create cluster, build/push images, deploy everything including Argo

.PHONY: stop-k3d
stop-k3d: ## delete the k3d cluster
	@k3d cluster delete --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml
