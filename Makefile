REGISTRY ?= ghcr.io/kelos-dev
IMAGE_NAME ?= token-refresher
IMAGE_REPOSITORY ?= $(REGISTRY)/$(IMAGE_NAME)
VERSION ?= latest

BIN_DIR ?= bin
BIN ?= $(BIN_DIR)/token-refresher
GOCACHE ?= $(CURDIR)/.cache/go-build
GOMODCACHE ?= $(CURDIR)/.cache/go-mod
GO_SOURCES := $(shell find cmd internal -name '*.go' -type f 2>/dev/null)
BUILD_DIRS := $(BIN_DIR) $(GOCACHE) $(GOMODCACHE)

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

.PHONY: help
help: ## Display available targets.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

$(BUILD_DIRS):
	mkdir -p $@

.PHONY: build
build: | $(BUILD_DIRS) ## Build the token-refresher binary.
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go build -o $(BIN) ./cmd/token-refresher

.PHONY: test
test: | $(BUILD_DIRS) ## Run unit tests.
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test ./...

.PHONY: verify
verify: | $(BUILD_DIRS) ## Verify formatting, tests, and vet checks.
	@unformatted="$$(gofmt -l $(GO_SOURCES))"; \
	if [[ -n "$$unformatted" ]]; then \
		echo "unformatted Go files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go test ./...
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go vet ./...

.PHONY: update
update: | $(BUILD_DIRS) ## Format Go code and tidy modules.
	gofmt -w $(GO_SOURCES)
	GOCACHE=$(GOCACHE) GOMODCACHE=$(GOMODCACHE) go mod tidy

.PHONY: image
image: ## Build the container image.
	docker build -t $(IMAGE_REPOSITORY):$(VERSION) .

.PHONY: clean
clean: ## Remove local build artifacts.
	rm -rf $(BIN_DIR) .cache
