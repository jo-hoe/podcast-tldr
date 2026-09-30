include help.mk

ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

# The feature repos, expected as sibling checkouts under ../ for local image builds.
STAGES := download transcribe summarize zip backup
IMAGE_VERSION := 1.0.0

.DEFAULT_GOAL := help

# --- Local end-to-end via docker compose ---

.PHONY: start-compose
start-compose: ## run the full pipeline locally via docker compose (pulls ghcr images)
	docker-compose up --abort-on-container-exit

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
		echo "building podcast-tldr-$$stage"; \
		docker build -t localhost:5000/podcast-tldr-$$stage:${IMAGE_VERSION} ${ROOT_DIR}../podcast-tldr-$$stage; \
		docker push localhost:5000/podcast-tldr-$$stage:${IMAGE_VERSION}; \
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
install-argo: ## install Argo Workflows into the cluster
	@kubectl create namespace argo || true
	@kubectl apply -n argo -f https://github.com/argoproj/argo-workflows/releases/latest/download/quick-start-minimal.yaml

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
