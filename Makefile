SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO_DIR := labctl-go
GO_FILES := $(shell find $(GO_DIR) -type f -name '*.go' | sort)
ENGINE_RELEASE_BIN := target/release/paraflow-engine
LABCTL_BIN := bin/labctl
VERSION := 0.1.0-alpha.2
GIT_COMMIT := $(shell git rev-parse HEAD 2>/dev/null || printf 'unknown')
<<<<<<< HEAD
GIT_STATE := $(shell if [[ -z "$$(git status --porcelain 2>/dev/null)" ]]; then printf 'clean'; else printf 'dirty'; fi)
=======
GIT_STATE := $(shell if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then printf 'unknown'; elif [[ -z "$$(git status --porcelain)" ]]; then printf 'clean'; else printf 'dirty'; fi)
>>>>>>> 4b93d5b (day 5: ci: exercise disposable benchmark smoke suite)
CARGO_BUILD_ENV := PARAFLOW_SOURCE_COMMIT="$(GIT_COMMIT)" PARAFLOW_SOURCE_STATE="$(GIT_STATE)"

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show repository commands.
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "  %-22s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: fmt
fmt: ## Format Rust and Go source.
	$(CARGO_BUILD_ENV) cargo fmt --all
	gofmt -w $(GO_FILES)

.PHONY: fmt-check
fmt-check: rust-fmt-check go-fmt-check ## Verify formatting without modifying files.

.PHONY: rust-fmt-check
rust-fmt-check:
	$(CARGO_BUILD_ENV) cargo fmt --all -- --check

.PHONY: rust-lint
rust-lint:
	$(CARGO_BUILD_ENV) cargo clippy --locked --workspace --all-targets -- -D warnings

.PHONY: rust-test
rust-test:
	$(CARGO_BUILD_ENV) cargo test --locked --workspace --all-targets

.PHONY: rust-build
rust-build:
	$(CARGO_BUILD_ENV) cargo build --locked --workspace --all-targets

.PHONY: rust-release-build
rust-release-build:
	$(CARGO_BUILD_ENV) cargo build --locked --release -p paraflow-engine

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
		$(CARGO_BUILD_ENV) cargo run --locked --quiet -p paraflow-engine -- validate "$$workload_path"; \
	done

.PHONY: generation-conformance
generation-conformance: rust-release-build ## Run optimized generator conformance tests.
	$(CARGO_BUILD_ENV) cargo test --locked --release -p paraflow-engine --test generation_v1

.PHONY: generation-check
generation-check: contract-check workload-check generation-conformance ## Verify Day 2 generation and portable conformance vectors.

.PHONY: scalar-conformance
scalar-conformance: generation-conformance ## Run optimized scalar-oracle conformance and CLI smoke checks.
	$(CARGO_BUILD_ENV) cargo test --locked --release -p paraflow-engine --test scalar_v1
	$(CARGO_BUILD_ENV) cargo run --locked --release --quiet -p paraflow-engine -- \
		oracle workloads/edge-scalar-v1.json >/dev/null

.PHONY: scalar-check
scalar-check: contract-check workload-check scalar-conformance ## Verify the complete Day 3 scalar correctness oracle.

.PHONY: protocol-conformance
protocol-conformance: rust-release-build go-build ## Exercise the release Rust worker through the real Go controller.
	$(CARGO_BUILD_ENV) cargo test --locked --release -p paraflow-engine --test protocol_v1
	cd $(GO_DIR) && \
		PARAFLOW_ENGINE_PATH="$(abspath $(ENGINE_RELEASE_BIN))" \
		PARAFLOW_REPOSITORY_ROOT="$(CURDIR)" \
		go test -count=1 ./internal/worker \
			-run '^TestRealEngineSessionReusesOneProcess$$'
	./tools/check-protocol-integration.sh \
		"$(LABCTL_BIN)" \
		"$(ENGINE_RELEASE_BIN)"

.PHONY: protocol-check
protocol-check: contract-check workload-check scalar-conformance protocol-conformance ## Verify the complete Day 4 execution protocol.

.PHONY: benchmark-conformance
benchmark-conformance: rust-release-build go-build ## Verify Day 5 measurement contracts and the real Go-to-Rust benchmark boundary.
	$(CARGO_BUILD_ENV) cargo test --locked --release -p paraflow-engine --test benchmark_v1
	cd $(GO_DIR) && go test -count=1 ./internal/benchmark
	cd $(GO_DIR) && \
		PARAFLOW_ENGINE_PATH="$(abspath $(ENGINE_RELEASE_BIN))" \
		PARAFLOW_REPOSITORY_ROOT="$(CURDIR)" \
		go test -count=1 ./internal/benchmark \
			-run '^TestRealEngineRunsWarmupsAndSamplesInsideOneProcess$$'

.PHONY: benchmark-preflight
benchmark-preflight: protocol-check benchmark-conformance go-build-smoke ## Verify release-build and measurement readiness without persisting baseline data.
	$(LABCTL_BIN) doctor

.PHONY: benchmark-smoke
benchmark-smoke: benchmark-preflight ## Run a disposable cross-language smoke suite with raw samples and no timing threshold.
	@output="$(CURDIR)/results/raw/.day05-smoke-$${RANDOM}-$${RANDOM}.json"; \
	rm -f "$$output"; \
	trap 'rm -f "$$output"' EXIT; \
	$(LABCTL_BIN) benchmark \
		--engine "$(abspath $(ENGINE_RELEASE_BIN))" \
		--suite benchmarks/suites/day05-smoke-v1.json \
		--output "$$output" \
		--repository-root "$(CURDIR)"; \
	test -s "$$output"

.PHONY: benchmark-day05
benchmark-day05: benchmark-preflight ## Persist the full Day 5 scalar baseline under results/raw/.
	@timestamp="$$(date -u +%Y%m%dT%H%M%SZ)"; \
	short_commit="$$(printf '%s' '$(GIT_COMMIT)' | cut -c1-12)"; \
	output="$(CURDIR)/results/raw/day05-scalar-$${timestamp}-$${short_commit}.json"; \
	$(LABCTL_BIN) benchmark \
		--engine "$(abspath $(ENGINE_RELEASE_BIN))" \
		--suite benchmarks/suites/day05-scalar-baseline-v1.json \
		--output "$$output" \
		--repository-root "$(CURDIR)"; \
	printf 'persisted %s\n' "$$output"

.PHONY: check
check: contract-check rust-check workload-check go-check scalar-conformance protocol-conformance benchmark-conformance ## Run every current quality gate.

.PHONY: clean
clean: ## Remove generated build outputs.
	cargo clean
	rm -f bin/labctl
	rmdir bin 2>/dev/null || true
