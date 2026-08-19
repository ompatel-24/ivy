BINARY := dist/ivy
GO ?= go
NPM ?= npm
VERSION ?= dev
LDFLAGS := -X github.com/ompatel-24/ivy/internal/version.Value=$(VERSION)
WEB_DIR := web
WEB_BUILD := $(WEB_DIR)/dist
DIST_WEB := dist/web

.PHONY: build dev web test lint

build: web
	rm -rf $(DIST_WEB)
	mkdir -p $(DIST_WEB)
	cp -R $(WEB_BUILD)/. $(DIST_WEB)/
	mkdir -p dist
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/ivy

test:
	$(NPM) --prefix $(WEB_DIR) ci
	$(NPM) --prefix $(WEB_DIR) test
	$(GO) test -race ./...

lint:
	$(NPM) --prefix $(WEB_DIR) ci
	$(NPM) --prefix $(WEB_DIR) run typecheck
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "The following files need gofmt:"; echo "$$files"; exit 1; fi
	$(GO) vet ./...

web:
	$(NPM) --prefix $(WEB_DIR) ci
	$(NPM) --prefix $(WEB_DIR) run build

dev: web
	IVY_LISTEN=$(IVY_LISTEN) IVY_WEB_DIR=$(abspath $(WEB_BUILD)) $(GO) run ./cmd/ivy $(ARGS)
