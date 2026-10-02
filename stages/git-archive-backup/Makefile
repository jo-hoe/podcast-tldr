include help.mk

ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

IMAGE_NAME := git-archive-backup
IMAGE_VERSION := 1.0.0

.DEFAULT_GOAL := start-docker

.PHONY: build
build: ## build the binary
	go build -mod=vendor ${ROOT_DIR}

.PHONY: update
update: ## pull git repo, tidy and re-vendor modules
	@git -C ${ROOT_DIR} pull
	go mod tidy
	go mod vendor

.PHONY: vendor
vendor: ## refresh the vendor directory
	go mod vendor

.PHONY: test
test: ## run go tests with coverage
	go test -mod=vendor -timeout 0 ${ROOT_DIR}... -covermode=count

.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run ${ROOT_DIR}...

.PHONY: install-hooks
install-hooks: ## install git hooks
	@echo Installing git hooks...
	@go run -C .githooks install.go

.PHONY: start-docker
start-docker: ## build and run the stage via docker compose
	docker-compose up backup --build

# --- K3d cluster management ---

.PHONY: create-cluster
create-cluster: ## create the local k3d cluster
	@k3d cluster create --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml

.PHONY: build-and-push
build-and-push: ## build and push image to the local registry
	@docker build -f ${ROOT_DIR}Dockerfile . -t ${IMAGE_NAME}
	@docker tag ${IMAGE_NAME} localhost:5000/${IMAGE_NAME}:${IMAGE_VERSION}
	@docker push localhost:5000/${IMAGE_NAME}:${IMAGE_VERSION}

.PHONY: deploy-helm
deploy-helm: ## install the helm chart into the cluster
	@helm install ${IMAGE_NAME} \
		--set image.repository=registry.localhost:5000/${IMAGE_NAME} \
		--set image.tag=${IMAGE_VERSION} \
		${ROOT_DIR}charts/${IMAGE_NAME}

.PHONY: start-k3d
start-k3d: create-cluster build-and-push deploy-helm ## start k3d and deploy the local image

.PHONY: stop-k3d
stop-k3d: ## delete the k3d cluster
	@k3d cluster delete --config ${ROOT_DIR}k3d/podcasttldrcluster.yaml

.PHONY: generate-helm-docs
generate-helm-docs: ## generate helm chart docs
	@docker run --rm --volume "$(ROOT_DIR)charts/${IMAGE_NAME}:/helm-docs" jnorwood/helm-docs:latest
