BINARY := dist/rome
GO ?= go
GORELEASER ?= goreleaser
NPM ?= npm
VERSION ?= dev
LDFLAGS := -X github.com/ompatel-24/rome/internal/version.Value=$(VERSION)
WEB_DIR := web
EMBEDDED_WEB := internal/webassets/dist

.PHONY: build dev web web-check test lint release-check release-snapshot

build: web
	rm -rf dist/web
	mkdir -p dist
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/rome

test: web-check
	$(NPM) --prefix $(WEB_DIR) test
	$(GO) test -race ./...

lint: web-check
	$(NPM) --prefix $(WEB_DIR) run typecheck
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "The following files need gofmt:"; echo "$$files"; exit 1; fi
	$(GO) vet ./...

release-check:
	GORELEASER=$(GORELEASER) ./scripts/check-goreleaser.sh

release-snapshot: release-check
	$(GORELEASER) release --snapshot --clean
	./scripts/verify-release.sh 0.0.0-snapshot

web:
	$(NPM) --prefix $(WEB_DIR) ci
	$(NPM) --prefix $(WEB_DIR) run build

web-check:
	$(NPM) --prefix $(WEB_DIR) ci
	@rome_web_check_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$rome_web_check_dir"' EXIT; \
	(cd $(WEB_DIR) && $(NPM) exec -- vite build --outDir "$$rome_web_check_dir" >/dev/null); \
	diff -qr $(EMBEDDED_WEB) "$$rome_web_check_dir"

dev: web
	ROME_LISTEN=$(ROME_LISTEN) ROME_WEB_DIR=$(abspath $(EMBEDDED_WEB)) $(GO) run ./cmd/rome $(ARGS)
