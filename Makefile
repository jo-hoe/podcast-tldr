include help.mk

ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# The feature repos, expected as sibling checkouts under ../ for local image builds.
# Each name is both the sibling directory and the image name.
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
	docker-compose down

.PHONY: clean-work
clean-work: ## delete local pipeline artifacts under ./volume
	@rm -rf ${ROOT_DIR}volume/work ${ROOT_DIR}volume/models

# --- K3d cluster management ---

.PHONY: create-cluster
create-cluster: ## create the local k3d cluster
	@k3d cluster create --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml

.PHONY: build-and-push
build-and-push: ## build every stage image from sibling repos and push to the local registry
	@for stage in $(STAGES); do \
		echo "building $$stage"; \
		docker build -t localhost:5000/$$stage:${IMAGE_VERSION} ${ROOT_DIR}../$$stage; \
		docker push localhost:5000/$$stage:${IMAGE_VERSION}; \
	done

.PHONY: deploy
deploy: ## apply shared resources (PVCs, config, secrets, LiteLLM) to the cluster
	@kubectl apply -f ${ROOT_DIR}k8s/00-pvc.yaml
	@kubectl apply -f ${ROOT_DIR}k8s/01-configmap.yaml
	@echo "NOTE: create the real secret before running stages:"
	@echo "  kubectl create secret generic podcast-tldr-secrets --from-literal=litellmApiKey=... --from-literal=backupToken=..."
	@kubectl apply -f ${ROOT_DIR}k8s/03-litellm.yaml

.PHONY: run-jobs
run-jobs: ## run the pipeline as plain sequential k8s Jobs
	@kubectl apply -f ${ROOT_DIR}k8s/10-jobs.yaml

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
argo-ui: ## open the Argo Workflows UI in the browser (k3d: direct; other clusters: port-forward)
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
start-k3d: create-cluster build-and-push deploy ## create cluster, build/push images, deploy shared resources

.PHONY: stop-k3d
stop-k3d: ## delete the k3d cluster
	@k3d cluster delete --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml
