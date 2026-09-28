GO ?= go
VERSION ?= v$(shell cat cmd/plous/VERSION)
export GO VERSION

.PHONY: build run check dist clean

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/plous ./cmd/plous

run: build
	./bin/plous

check: build
	@test -z "$$(gofmt -l cmd internal)" || { echo "Run gofmt on cmd/ and internal/"; exit 1; }
	$(GO) vet ./...
	$(GO) test ./...
	@bin/plous --version > bin/actual-output.txt
	@cmp cmd/plous/VERSION bin/actual-output.txt
	@rm bin/actual-output.txt

dist:
	sh scripts/dist.sh

clean:
	rm -rf bin dist
