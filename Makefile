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

# js runs site/app.js under node and checks the claims it makes about itself.
#
# The chart's legibility figures -- 195 ribbon crossings, $457,434,169 of
# overlapping ribbon, 14 stale-stacked pairs -- can only be produced by laying
# the graph out, so before this target they were measured once by hand, out of
# tree, and quoted in comments forever after (fisc-gxa.7). tools/jscheck loads
# the SHIPPED app.js and the vendored d3 into a node vm and re-measures them.
#
# NODE IS OFF THE DEPLOY PATH, the way tools/extract.py is off the build path:
# `make site`, `make build` and `fisc export` never run this, and a contributor
# without node can still build, test and serve the site. There is no
# package.json, no node_modules and no npm.
.PHONY: js
js: ## Check site/app.js's layout claims under node (needs node; nothing else does)
	node tools/jscheck/run.mjs

# js-if-available is to `js` what lint-if-available is to `lint`, and for the
# same reason: node must not become mandatory to commit.
.PHONY: js-if-available
js-if-available: ## Run js, warning rather than failing if node is absent
	@command -v node >/dev/null 2>&1 || { \
		echo "warning: node not on PATH, skipping the app.js checks; CI will still run them" >&2; \
		exit 0; \
	}; \
	$(MAKE) --no-print-directory js

.PHONY: pre-commit
pre-commit: fmt vet test lint-if-available js-if-available ## Format, vet, test, lint, and check app.js

# A HOOK CANNOT BE COMMITTED. .git/hooks is not tracked, so "symlink pre-commit
# into it" is per-checkout setup somebody has to actually run -- and until this
# target existed, CLAUDE.md and docs/agents/workflow.md both described the
# symlink as though it were already there. It was not, in any checkout anyone
# looked at, which made a workflow document assert a guard that did not exist.
#
# THE LOCAL HOOK IS A CONVENIENCE AND CI IS THE GATE. A contributor who never
# runs this target is not doing anything wrong; the required lint and verify
# jobs still fail their PR. Keep it that way -- a repository whose correctness
# depends on every clone having run a setup step has no gate at all.
#
# It refuses an existing regular file rather than clobbering it, because that
# file is somebody's own hook and losing it silently is worse than not
# installing ours.
.PHONY: hooks
hooks: ## Install the local pre-commit hook (idempotent; CI is still the gate)
	@test -d .git/hooks || { echo "no .git/hooks; not a git checkout?" >&2; exit 1; }
	@if [ -e .git/hooks/pre-commit ] && [ ! -L .git/hooks/pre-commit ]; then \
		echo "refusing: .git/hooks/pre-commit exists and is not a symlink" >&2; \
		echo "  move it aside, then re-run 'make hooks'" >&2; \
		exit 1; \
	fi
	@printf '#!/bin/sh\nexec make pre-commit\n' > .git/hooks/pre-commit.fisc
	@chmod +x .git/hooks/pre-commit.fisc
	@ln -sf pre-commit.fisc .git/hooks/pre-commit
	@echo "installed .git/hooks/pre-commit -> pre-commit.fisc (runs 'make pre-commit')"

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
