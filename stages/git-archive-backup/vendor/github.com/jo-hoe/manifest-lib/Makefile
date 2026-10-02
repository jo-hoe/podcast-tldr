include help.mk

# get root dir
ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

.DEFAULT_GOAL := test

.PHONY: build
build: ## build the library
	go build ${ROOT_DIR}...

.PHONY: update
update: ## pull git repo and tidy modules
	@git -C ${ROOT_DIR} pull
	go mod tidy

.PHONY: test
test: ## run go tests with coverage
	go test -timeout 0 ${ROOT_DIR}... -covermode=count

.PHONY: lint
lint: ## run golangci-lint
	golangci-lint run ${ROOT_DIR}...

.PHONY: install-hooks
install-hooks: ## install git hooks
	@echo Installing git hooks...
	@go run -C .githooks install.go
