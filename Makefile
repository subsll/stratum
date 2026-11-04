SHELL := /bin/sh

VERSION ?= 1.1.1
GO      ?= go
DIST    := dist
LDFLAGS := -s -w -X github.com/subsll/stratum/internal/model.Version=$(VERSION)

.PHONY: all build local linux test race vet fmt coverage demo checksums install clean

all: build

## build: native binary for the current platform
build:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" \
		-o $(DIST)/stratum ./cmd/stratum

## local: versioned native binary for the current platform
local:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" \
		-o $(DIST)/stratum_$(VERSION)_$(shell $(GO) env GOOS)_$(shell $(GO) env GOARCH) ./cmd/stratum

## linux: static Linux binaries for amd64, arm64, armv7 and 386
linux:
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o $(DIST)/stratum_$(VERSION)_linux_amd64 ./cmd/stratum
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o $(DIST)/stratum_$(VERSION)_linux_arm64 ./cmd/stratum
	CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o $(DIST)/stratum_$(VERSION)_linux_armv7 ./cmd/stratum
	CGO_ENABLED=0 GOOS=linux GOARCH=386 $(GO) build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)" -o $(DIST)/stratum_$(VERSION)_linux_386 ./cmd/stratum

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -w .

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

## demo: build and run a synthetic end-to-end case in a temp directory
demo: build
	./scripts/demo.sh

checksums:
	@cd $(DIST) && \
	if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum stratum* > SHA256SUMS; \
	else \
		shasum -a 256 stratum* > SHA256SUMS; \
	fi && cat SHA256SUMS

install:
	CGO_ENABLED=0 $(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/stratum

clean:
	rm -rf $(DIST) coverage.out
