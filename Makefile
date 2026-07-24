SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO_DIR := labctl-go
GO_FILES := $(shell find $(GO_DIR) -type f -name '*.go' | sort)
VERSION := 0.1.0-alpha.1
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || printf 'unknown')
GIT_STATE := $(shell if [[ -z "$$(git status --porcelain 2>/dev/null)" ]]; then printf 'clean'; else printf 'dirty'; fi)

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
	cargo clippy --locked --workspace --all-targets -- -D warnings

.PHONY: rust-test
rust-test:
	cargo test --locked --workspace --all-targets

.PHONY: rust-build
rust-build:
	cargo build --locked --workspace --all-targets

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
		-ldflags "-X main.version=$(VERSION) -X main.commit=$(GIT_COMMIT) -X main.sourceState=$(GIT_STATE)" \
		-o ../bin/labctl \
		./cmd/labctl

.PHONY: go-build-smoke
go-build-smoke: go-build
	@output="$$(./bin/labctl version)"; \
	[[ "$$output" == *"$(GIT_COMMIT)"* ]] || { \
		printf 'labctl version omitted commit identity: %s\n' "$$output" >&2; \
		exit 1; \
	}; \
	[[ "$$output" == *"$(GIT_STATE)"* ]] || { \
		printf 'labctl version omitted source state: %s\n' "$$output" >&2; \
		exit 1; \
	}

.PHONY: go-check
go-check: go-fmt-check go-lint go-test go-build-smoke ## Run all Go quality gates.

.PHONY: contract-check
contract-check: ## Check machine-readable contracts and fixtures.
	./tools/check-contracts.sh

.PHONY: workload-check
workload-check: ## Semantically validate every checked-in workload with Rust.
	@mapfile -t workload_files < <(find workloads -type f -name '*.json' -print | sort); \
	[[ "$${#workload_files[@]}" -gt 0 ]] || { \
		printf 'no workload fixtures found\n' >&2; \
		exit 1; \
	}; \
	for workload_path in "$${workload_files[@]}"; do \
		cargo run --locked --quiet -p paraflow-engine -- validate "$$workload_path"; \
	done

.PHONY: benchmark-preflight
benchmark-preflight: contract-check workload-check go-build-smoke ## Verify readiness without timing fake work.
	./bin/labctl doctor

.PHONY: check
check: contract-check rust-check workload-check go-check ## Run every Day 1 quality gate.

.PHONY: clean
clean: ## Remove generated build outputs.
	cargo clean
	rm -f bin/labctl
	rmdir bin 2>/dev/null || true
