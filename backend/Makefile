GO ?= go
BASH ?= bash
COVERAGE_FILE ?= coverage.out
GO_PACKAGES ?= ./...
CI_GATE := ./scripts/ci-security-gate.sh

.DEFAULT_GOAL := help

.PHONY: help build fmt-check vet lint test coverage check ci-local ci-quick ci-docker ci-security ci-sonar ci-all beta-rc perf-k6-core-read perf-k6-stream-credentials dra-up dra-down dra-e2e clean

help:
	@printf '%s\n' 'Backend developer targets:'
	@printf '%s\n' '  make build        Build all backend packages'
	@printf '%s\n' '  make fmt-check    Verify Go files are gofmt-formatted'
	@printf '%s\n' '  make vet          Run go vet on all backend packages'
	@printf '%s\n' '  make lint         Run formatting and vet checks'
	@printf '%s\n' '  make test         Run unit tests'
	@printf '%s\n' '  make coverage     Run tests with coverage.out'
	@printf '%s\n' '  make check        Run the quick local CI gate'
	@printf '%s\n' '  make ci-local     Alias for make check'
	@printf '%s\n' '  make ci-quick     Alias for make check'
	@printf '%s\n' '  make ci-docker    Run Docker-backed CI gate'
	@printf '%s\n' '  make ci-security  Run security and image scan gate'
	@printf '%s\n' '  make ci-sonar     Run SonarScanner quality gate'
	@printf '%s\n' '  make ci-all       Run all CI gates'
	@printf '%s\n' '  make beta-rc      Run production beta RC gate'
	@printf '%s\n' '  make perf-k6-core-read  Run optional k6 core read performance harness'
	@printf '%s\n' '  make perf-k6-stream-credentials  Run optional k6 stream credential performance harness'
	@printf '%s\n' '  make dra-up       Bring up a GPU-less kind cluster with dra-example-driver'
	@printf '%s\n' '  make dra-e2e      Run the live DRA E2E against the fake-GPU cluster'
	@printf '%s\n' '  make dra-down     Delete the dra-example-driver kind cluster'
	@printf '%s\n' '  make clean        Remove local coverage output'

build:
	$(GO) build $(GO_PACKAGES)

fmt-check:
	@unformatted="$$(find . -name '*.go' -type f -print0 | xargs -0 gofmt -l)"; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

vet:
	$(GO) vet $(GO_PACKAGES)

lint: fmt-check vet

test:
	$(GO) test $(GO_PACKAGES) -count=1

coverage:
	$(GO) test $(GO_PACKAGES) -coverprofile=$(COVERAGE_FILE) -count=1

check ci-local ci-quick:
	$(BASH) $(CI_GATE) quick

ci-docker:
	$(BASH) $(CI_GATE) docker

ci-security:
	$(BASH) $(CI_GATE) security

ci-sonar:
	$(BASH) $(CI_GATE) sonar

ci-all:
	$(BASH) $(CI_GATE) all

beta-rc:
	$(BASH) $(CI_GATE) beta-rc

perf-k6-core-read:
	mkdir -p .tmp
	k6 run scripts/perf/core-read-100vu.js

perf-k6-stream-credentials:
	mkdir -p .tmp
	k6 run scripts/perf/stream-credentials-100vu.js

DRA_E2E_TEST ?= ^TestLiveK8sConfigFileDRADispatchE2E$$

dra-up:
	$(BASH) scripts/dra-kind-up.sh

dra-down:
	$(BASH) scripts/dra-kind-down.sh

dra-e2e:
	TEST_LIVE_K8S_CONFIGFILE_DRA=1 $(GO) test -tags e2e ./internal/e2e -run '$(DRA_E2E_TEST)' -count=1 -v

clean:
	rm -f $(COVERAGE_FILE)
