GOFILES := \
	$(wildcard *.go) \
	$(wildcard cmd/*.go) \
	$(wildcard internal/*/*.go) \
	$(wildcard internal/*/*/*.go)

KPF_BIN=$(HOME)/bin/$(PROG)
## Stop the random illegal instruction crash
GODEBUG=asyncpreemptoff=1

## some runtime settings
PROG=kpf
BUILD_DIR=build

## Version info from git
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

## Build flags for version info
LDFLAGS := -X main.version=$(GIT_TAG) -X main.commit=$(GIT_COMMIT) -X main.date=$(BUILD_DATE)

RUN_ARGS= \
	--kubeconfig=$(HOME)/.kube/config

kpf: $(GOFILES) ## just build the binary
	go build -ldflags "$(LDFLAGS)" -o $(PROG) .

run: $(PROG) ## run the binary
	./$(PROG) $(RUN_ARGS)

install: ## install the binary
	go build -ldflags "$(LDFLAGS)" -o $(PROG) .
	@if [ -d "$(HOME)/bin" ]; then \
		cp $(PROG) $(HOME)/bin/$(PROG); \
		echo "Installed $(PROG) to $(HOME)/bin/$(PROG)"; \
	fi

test: ## run tests
	go test ./...

LINT := $(shell command -v revive 2>/dev/null)

install_lint:
ifndef LINT
	GOPATH=$(shell go env GOPATH) go install github.com/mgechev/revive@latest
endif

lint: install_lint ## run linter
	$(shell go env GOPATH)/bin/revive --exclude=vendor/... -formatter friendly ./...

vet: ## run go vet
	go vet ./...

fmt: ## run go fmt
	go fmt ./...

mod: ## download and tidy go modules
	go mod download
	go mod tidy

clean: ## clean up built binary and build directory
	@echo "Cleaning up binary and build artifacts..."
	rm -f $(PROG) $(HOME)/bin/$(PROG)
	rm -rf $(BUILD_DIR)

demo: kpf ## run a demo with a specific namespace
	./$(PROG) --namespace=kube-system

check: fmt vet lint ## run all checks (fmt, vet, lint)

build-all: ## build for multiple architectures
	@echo "Building for multiple architectures..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(PROG)-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(PROG)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(PROG)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(PROG)-darwin-arm64 .
	@echo "Build artifacts created in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
.DEFAULT_GOAL := help