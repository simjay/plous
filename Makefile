GO ?= go
VERSION ?= dev
export GO VERSION

.PHONY: build check dist clean

build:
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -o bin/plous ./cmd/plous

check: build
	@test -z "$$(gofmt -l cmd)" || { echo "Run gofmt on cmd/"; exit 1; }
	$(GO) vet ./...
	$(GO) test ./...
	@bin/plous > bin/actual-output.txt
	@printf 'welcome to plous\n' | cmp - bin/actual-output.txt
	@rm bin/actual-output.txt

dist:
	sh scripts/dist.sh

clean:
	rm -rf bin dist
