BIN        := fisc
CMD        := ./cmd/fisc
OUT        := bin/$(BIN)
PREFIX     ?= /usr/local
# Prefer a repo-local venv, then one on PATH. Override with PYTHON=... for a
# venv elsewhere. Any Python 3 will do -- tools/extract.py is standard library
# only. The extractor itself is poppler-utils on PATH; see requirements.txt.
PYTHON     ?= $(shell test -x .venv/bin/python && echo .venv/bin/python || echo python3)

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
lint: ## Run golangci-lint (warns if the version differs from .golangci-version)
	@want=$$(tr -d '[:space:]' < .golangci-version); \
	got=$$(golangci-lint version --short 2>/dev/null | tr -d 'v[:space:]'); \
	test "$$want" = "$$got" || echo "warning: golangci-lint $$got on PATH, .golangci-version pins $$want; CI will lint with $$want" >&2
	golangci-lint run

.PHONY: fmt
fmt: ## Format Go sources
	gofmt -w ./cmd ./internal ./pkg ./site

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	go mod tidy

# lint-if-available is what the commit hook runs, and it is NOT `lint`.
#
# golangci-lint is not required to build or test this project, and adding it to
# pre-commit would make an external binary mandatory to commit at all -- a
# contributor with Go and nothing else could no longer land a change. So absence
# WARNS and continues, while a lint that actually runs still fails the commit.
#
# The degradation is for ABSENCE ONLY. `lint` itself must keep hard-failing,
# because CI's required lint job is that target and a version of it that shrugged
# would defeat the gate this bead exists to strengthen.
.PHONY: lint-if-available
lint-if-available: ## Run lint, warning rather than failing if golangci-lint is absent
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "warning: golangci-lint not on PATH, skipping lint; CI will still lint this commit" >&2; \
		exit 0; \
	}; \
	$(MAKE) --no-print-directory lint

.PHONY: pre-commit
pre-commit: fmt vet test lint-if-available ## Format, vet, test, and lint (symlink to .git/hooks/pre-commit)

# Extraction is deliberately NOT part of the Go binary. It is a rare,
# human-initiated step whose output is committed; fisc reads only that output
# and needs neither Python nor the PDFs.
#
# DOC=<id> extracts one document; omit it for all three.
.PHONY: extract
extract: ## Re-extract PDFs with poppler (DOC=<id> for one)
	$(PYTHON) tools/extract.py $(if $(DOC),--doc $(DOC))

.PHONY: extract-list
extract-list: ## List extractable documents
	$(PYTHON) tools/extract.py --list

# The site is a static export of the committed fact store: `fisc export` reads
# facts/facts.jsonl and does NOT re-run the mapping engine, so this target
# cannot change a published figure. The page fetches data/sankey.json, so it
# needs a server -- app.js detects file:// and says so, but the hint belongs
# where someone would look for it.
.PHONY: site
site: build ## Build the static site into dist/
	$(OUT) export --clean
	@echo
	@echo "  Serve it:  python3 -m http.server -d dist 8000"
	@echo

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist
