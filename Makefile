SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO_DIR := labctl-go
GO_FILES := $(shell find $(GO_DIR) -type f -name '*.go' | sort)
VERSION := 0.1.0-alpha.1
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || printf 'unknown')

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show repository commands.
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Format Rust and Go source.
	cargo fmt --all
	gofmt -w $(GO_FILES)

.PHONY: fmt-check
fmt-check: rust-fmt-check go-fmt-check ## Verify formatting without modifying files.

.PHONY: rust-fmt-check
rust-fmt-check:
	cargo fmt --all -- --check

.PHONY: rust-lint
rust-lint:
	cargo clippy --workspace --all-targets -- -D warnings

.PHONY: rust-test
rust-test:
	cargo test --workspace --all-targets

.PHONY: rust-build
rust-build:
	cargo build --workspace --all-targets

.PHONY: rust-check
rust-check: rust-fmt-check rust-lint rust-test rust-build ## Run all Rust quality gates.

.PHONY: go-fmt-check
go-fmt-check:
	@unformatted="$$(gofmt -l $(GO_FILES))"; \
	if [[ -n "$$unformatted" ]]; then \
		printf 'Go files require formatting:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi

.PHONY: go-lint
go-lint:
	cd $(GO_DIR) && go vet ./...

.PHONY: go-test
go-test:
	cd $(GO_DIR) && go test -race -count=1 ./...

.PHONY: go-build
go-build:
	mkdir -p bin
	cd $(GO_DIR) && go build \
		-trimpath \
		-ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT)" \
		-o ../bin/labctl \
		./cmd/labctl

.PHONY: go-check
go-check: go-fmt-check go-lint go-test go-build ## Run all Go quality gates.

.PHONY: contract-check
contract-check: ## Check machine-readable contracts and fixtures.
	./tools/check-contracts.sh

.PHONY: validate-workload
validate-workload: ## Validate the checked-in smoke workload with Rust.
	cargo run -p paraflow-engine -- validate workloads/smoke-uniform-v1.json

.PHONY: benchmark-preflight
benchmark-preflight: contract-check validate-workload ## Verify readiness without timing fake work.
	cd $(GO_DIR) && go run ./cmd/labctl doctor --json

.PHONY: check
check: contract-check rust-check go-check ## Run every Day 1 quality gate.

.PHONY: clean
clean: ## Remove generated build outputs.
	cargo clean
	rm -f bin/labctl
	rmdir bin 2>/dev/null || true
