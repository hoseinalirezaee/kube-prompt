NAME := kube-prompt
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)
REVISION ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BIN := $(NAME)$(if $(filter windows,$(GOOS)),.exe,)
LDFLAGS := -X 'main.version=$(VERSION)' \
           -X 'main.revision=$(REVISION)'
GOIMPORTS ?= goimports
GOCILINT ?= golangci-lint
GO ?= GO111MODULE=on go
ZIP ?= python3 -m zipfile -c
.DEFAULT_GOAL := help

.PHONY: fmt
fmt: ## Formatting source codes.
	@$(GOIMPORTS) -w ./kube

.PHONY: lint
lint: ## Run golint and go vet.
	@$(GOCILINT) fmt --no-config --enable=goimports --diff
	@$(GOCILINT) run --no-config --enable-only=misspell ./...

.PHONY: test
test:  ## Run the tests.
	@$(GO) test ./...

.PHONY: build
build: main.go  ## Build a binary.
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)

.PHONY: code-gen
code-gen: ## Generate source codes.
	./_tools/codegen.sh

.PHONY: cross
cross: main.go  ## Build binaries for cross platform.
	mkdir -p pkg
	@# darwin
	@for arch in "amd64" "arm64"; do \
		GOOS=darwin GOARCH=$${arch} $(MAKE) build; \
		$(ZIP) pkg/kube-prompt_$(VERSION)_darwin_$${arch}.zip kube-prompt; \
	done;
	@# linux
	@for arch in "amd64" "arm64"; do \
		GOOS=linux GOARCH=$${arch} $(MAKE) build; \
		$(ZIP) pkg/kube-prompt_$(VERSION)_linux_$${arch}.zip kube-prompt; \
	done;
	@# windows
	@for arch in "amd64" "arm64"; do \
		GOOS=windows GOARCH=$${arch} $(MAKE) build; \
		$(ZIP) pkg/kube-prompt_$(VERSION)_windows_$${arch}.zip kube-prompt.exe; \
	done;

.PHONY: help
help: ## Show help text
	@echo "Commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "    \033[36m%-20s\033[0m %s\n", $$1, $$2}'
