GO ?= go
PYTHON ?= python3
TEST_FLAGS ?= -count=1
TEST_PACKAGES ?= ./...
TEST_E2E_PATTERN ?= test_*.py
TEST_E2E_ARGS ?=
BENCH_FLAGS ?= -bench=. -benchmem -benchtime=300ms
PLOUSBENCH_FLAGS ?= -benchtime=5x -count=1
VERSION ?= v$(shell cat cmd/plous/VERSION)
export GO VERSION

.PHONY: build build-test-tools test-ux run test eval test-unit test-e2e test-playground test-minikube test-ai-live test-agent-clients test-race test-coverage bench check fmt-check vet dist clean docs-check build-plousbench test-plousbench bench-plousbench

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/plous ./cmd/plous

run: build
	./bin/plous

docs-check:
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) tests/docs/check.py

test: test-unit test-e2e

eval:
	$(GO) test $(TEST_FLAGS) ./tests/acceptance/ai/...

test-unit:
	$(GO) test $(TEST_FLAGS) $(TEST_PACKAGES)

build-test-tools:
	mkdir -p bin
	$(GO) build -trimpath -buildvcs=false -o bin/test-terminalscreen ./tests/support/terminalscreen

test-e2e: build build-test-tools
	PLOUS_TEST_BINARY="$(CURDIR)/bin/plous" PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p '$(TEST_E2E_PATTERN)' -v $(TEST_E2E_ARGS)

test-ux: build build-test-tools
	$(GO) test -count=1 -run 'TestUXScreenMatrix|TestTerminalOutputBelongsToRenderer|TestCodeStartupFramesStayInsideTerminal' ./internal/app
	PLOUS_TEST_BINARY="$(CURDIR)/bin/plous" PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_ux.py' -v
	PLOUS_TEST_BINARY="$(CURDIR)/bin/plous" PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_kustomize.py' -k restored_sources -v

test-playground:
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_playground.py' -v

test-ai-live:
	@test -n "$(PLOUS_LIVE_AI_PROFILE)" || { echo "Set PLOUS_LIVE_AI_PROFILE to explicitly test a saved AI connection"; exit 1; }
	$(GO) test -count=1 -v -run TestSavedProviderLive ./tests/live/ai

test-agent-clients:
	PLOUS_TEST_INSTALLED_CLI=claude-code $(MAKE) test-unit TEST_PACKAGES='./tests/integration/nativeagents' TEST_FLAGS='-count=1 -v -run TestInstalledNativeCLIWithMockProvider'
	PLOUS_TEST_INSTALLED_CLI=codex $(MAKE) test-unit TEST_PACKAGES='./tests/integration/nativeagents' TEST_FLAGS='-count=1 -v -run TestInstalledNativeCLIWithMockProvider'

build-plousbench:
	mkdir -p bin
	$(GO) build -trimpath -buildvcs=false -o bin/plousbench ./benchmark/cmd/plousbench

test-plousbench:
	$(MAKE) test-unit TEST_PACKAGES='./benchmark/...'

bench-plousbench:
	$(GO) test -run '^$$' -bench '^BenchmarkPlousBenchTools$$' $(PLOUSBENCH_FLAGS) ./benchmark/integration

test-minikube: build
	@test -n "$(PLOUS_MINIKUBE_TEST_KUBECONFIG)" || { echo "Set PLOUS_MINIKUBE_TEST_KUBECONFIG to an explicit Minikube configuration"; exit 1; }
	$(MAKE) test-unit TEST_FLAGS='-count=1 -v -run TestPreparedMinikubeOperations'
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/live -v

test-race:
	$(GO) test -race $(TEST_FLAGS) $(TEST_PACKAGES)

bench:
	$(GO) test -run '^$$' $(BENCH_FLAGS) ./internal/app ./internal/ui/logs ./internal/ui/diff ./internal/ui/editor ./internal/ui/text

test-coverage:
	mkdir -p .cache/tests
	$(GO) test $(TEST_FLAGS) -coverprofile=.cache/tests/coverage.out ./...
	$(GO) tool cover -html=.cache/tests/coverage.out -o .cache/tests/coverage.html

check: docs-check fmt-check vet test

fmt-check:
	@test -z "$$(gofmt -l cmd internal benchmark tests)" || { echo "Run gofmt on cmd/, internal/, benchmark/, and tests/"; exit 1; }

vet:
	$(GO) vet ./...

dist:
	sh scripts/dist.sh

clean:
	rm -rf bin dist .cache/tests
	rm -f plous
