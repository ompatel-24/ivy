BINARY := dist/ivy
GO ?= go
VERSION ?= dev
LDFLAGS := -X github.com/ompatel-24/ivy/internal/version.Value=$(VERSION)

.PHONY: build test lint

build:
	mkdir -p dist
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/ivy

test:
	$(GO) test -race ./...

lint:
	@files="$$(gofmt -l .)"; if [ -n "$$files" ]; then echo "The following files need gofmt:"; echo "$$files"; exit 1; fi
	$(GO) vet ./...
