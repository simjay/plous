GO ?= go
PYTHON ?= python3
TEST_FLAGS ?= -count=1
BENCH_FLAGS ?= -bench=. -benchmem -benchtime=300ms
VERSION ?= v$(shell cat cmd/plous/VERSION)
export GO VERSION

.PHONY: build run test test-unit test-e2e test-playground test-minikube test-ai-live test-race test-coverage bench check fmt-check vet dist clean docs-install docs-dev docs-build docs-check

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/plous ./cmd/plous

run: build
	./bin/plous

docs-install:
	npm --prefix docs-site ci

docs-dev:
	npm --prefix docs-site run dev

docs-build:
	npm --prefix docs-site run build

docs-check:
	npm --prefix docs-site test
	npm --prefix docs-site run check
	$(MAKE) test-unit TEST_FLAGS='-count=1 -run TestDisplayedColonCommandsAreRegistered'

test: test-unit test-e2e

test-unit:
	$(GO) test $(TEST_FLAGS) ./...

test-e2e: build
	PLOUS_TEST_BINARY="$(CURDIR)/bin/plous" PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_*.py' -v

test-playground:
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_playground.py' -v

test-ai-live:
	@test -n "$(PLOUS_LIVE_AI_PROFILE)" || { echo "Set PLOUS_LIVE_AI_PROFILE to explicitly test a saved AI connection"; exit 1; }
	$(GO) test -count=1 -v -run TestSavedProviderLive ./internal/agent

test-minikube: build
	@test -n "$(PLOUS_MINIKUBE_TEST_KUBECONFIG)" || { echo "Set PLOUS_MINIKUBE_TEST_KUBECONFIG to an explicit Minikube configuration"; exit 1; }
	$(MAKE) test-unit TEST_FLAGS='-count=1 -v -run TestPreparedMinikubeOperations'
	PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/live -v

test-race:
	$(GO) test -race $(TEST_FLAGS) ./...

bench:
	$(GO) test -run '^$$' $(BENCH_FLAGS) ./internal/app ./internal/logview ./internal/diffview ./internal/editor ./internal/textview

test-coverage:
	mkdir -p .cache/tests
	$(GO) test $(TEST_FLAGS) -coverprofile=.cache/tests/coverage.out ./...
	$(GO) tool cover -html=.cache/tests/coverage.out -o .cache/tests/coverage.html

check: fmt-check vet test

fmt-check:
	@test -z "$$(gofmt -l cmd internal)" || { echo "Run gofmt on cmd/ and internal/"; exit 1; }

vet:
	$(GO) vet ./...

dist:
	sh scripts/dist.sh

clean:
	rm -rf bin dist .cache/tests
	rm -f plous
