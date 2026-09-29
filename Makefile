GO ?= go
PYTHON ?= python3
TEST_FLAGS ?= -count=1
VERSION ?= v$(shell cat VERSION)
export GO VERSION

.PHONY: build run test test-unit test-e2e test-race test-coverage check fmt-check vet dist clean

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/plous .

run: build
	./bin/plous

test: test-unit test-e2e

test-unit:
	$(GO) test $(TEST_FLAGS) ./...

test-e2e: build
	PLOUS_TEST_BINARY="$(CURDIR)/bin/plous" PYTHONDONTWRITEBYTECODE=1 $(PYTHON) -m unittest discover -s tests/e2e -p 'test_*.py' -v

test-race:
	$(GO) test -race $(TEST_FLAGS) ./...

test-coverage:
	mkdir -p .cache/tests
	$(GO) test $(TEST_FLAGS) -coverprofile=.cache/tests/coverage.out ./...
	$(GO) tool cover -html=.cache/tests/coverage.out -o .cache/tests/coverage.html

check: fmt-check vet test

fmt-check:
	@test -z "$$(gofmt -l main.go internal)" || { echo "Run gofmt on main.go and internal/"; exit 1; }

vet:
	$(GO) vet ./...

dist:
	sh scripts/dist.sh

clean:
	rm -rf bin dist .cache/tests
	rm -f plous
