BIN        := fisc
CMD        := ./cmd/fisc
OUT        := bin/$(BIN)
PREFIX     ?= /usr/local
PYTHON     ?= /home/ubuntu/venv/bin/python

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     := $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG        := github.com/jcrussell/livermore-budget/internal/build
LD         := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

# Not named GOFLAGS: that variable is consumed by the go tool itself and
# setting it here would leak into every invocation.
GOBUILD    := CGO_ENABLED=0 go build -trimpath

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: ## Build the fisc binary into bin/
	$(GOBUILD) -ldflags "$(LD)" -o $(OUT) $(CMD)

.PHONY: install
install: build ## Install fisc to $PREFIX/bin
	install -m 0755 $(OUT) $(PREFIX)/bin/$(BIN)

.PHONY: test
test: ## Run tests with the race detector
	go test -race ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go sources
	gofmt -w ./cmd ./internal ./pkg

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	go mod tidy

.PHONY: pre-commit
pre-commit: fmt vet test ## Format, vet, and test (symlink to .git/hooks/pre-commit)

# Extraction is deliberately NOT part of the Go binary. It is a rare,
# human-initiated step whose output is committed; fisc reads only that output
# and needs neither Python nor the PDFs.
#
# DOC=<id> extracts one document; omit it for all three.
.PHONY: extract
extract: ## Re-extract PDFs with xberg (DOC=<id> for one)
	$(PYTHON) tools/extract.py $(if $(DOC),--doc $(DOC))

.PHONY: extract-list
extract-list: ## List extractable documents
	$(PYTHON) tools/extract.py --list

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist
