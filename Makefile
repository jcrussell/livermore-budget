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
	gofmt -w ./cmd ./internal ./pkg ./site ./tools

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	go mod tidy

# narration refuses the two phrases that only ever introduce HISTORY into a
# source comment: a review credit, and an erratum about what the comment used to
# say. AGENTS.md's "History's home is git" forbids both and was a paragraph until
# now; one lane put seventeen of them into four files with that paragraph in
# context the whole time, so this is the rule restated as something that can go
# red.
#
# IT IS DELIBERATELY TWO NARROW PHRASES AND NOT A PROSE DETECTOR. "an earlier
# version" and "used to say" have honest uses in a doc comment about a DOCUMENT
# -- the city reprints schedules -- so widening this would refuse true sentences
# about the corpus and teach people to reword around it. What it catches is the
# two forms that are never about the code's present.
#
# Pre-existing hits are the ones AGENTS.md says to drop in the file you were
# editing anyway rather than in a sweep, so this target's failure is the moment
# you touch such a file, which is exactly when the rule says to fix it.
#
# THE SECOND ARM READS THE MEMORIES, which bd prime injects into every session.
# Same rule, worse placed: a memory's erratum arrives in context whether or not
# anyone opens the file it is about. It is a second arm on this target rather
# than a target of its own so that pre-commit gains no new step. The two print
# DIFFERENT messages, because the remedies differ: a source comment is edited in
# the file and its history stays in git, a memory is edited with bd remember and
# leaves no git artifact at all.
#
# It WARNS AND CONTINUES WITHOUT bd, the way lint-if-available does without
# golangci-lint. Memories live in the Dolt DB and in no git artifact, so unlike
# every other arm of pre-commit this one cannot be a CI job -- there is nothing
# in a checkout for CI to read. That makes it advisory by construction, which is
# worth knowing before trusting it.
.PHONY: narration
narration: ## Refuse review credits and comment errata in Go sources and memories
	@if grep -rnE '^[[:space:]]*//.*(Found by /code-review|this comment used to say)' \
		--include='*.go' ./cmd ./internal ./pkg ./tools 2>/dev/null; then \
		echo "" >&2; \
		echo "narration: the lines above put HISTORY in a source comment." >&2; \
		echo "  A review credit belongs in the commit body; an erratum belongs" >&2; \
		echo "  in git. Keep the rule the comment carries and delete the history" >&2; \
		echo "  that argued for it. See AGENTS.md, \"History's home is git\"." >&2; \
		exit 1; \
	fi
	@command -v bd >/dev/null 2>&1 || { \
		echo "warning: bd not on PATH, skipping the memory errata check" >&2; \
		exit 0; \
	}; \
	bd memories --json | go run ./tools/memcheck

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

# codehash fingerprints Go files by their code alone, so that a change asserted
# to be comments-only can be proved to be one. It is a BEFORE-AND-AFTER
# comparison and so is deliberately not part of pre-commit, which a single-shot
# target could not express:
#
#     make codehash FILES="internal/geom/geom.go" > /tmp/before
#     ...rewrite the comments...
#     make codehash FILES="internal/geom/geom.go" | diff /tmp/before -
#
# Reading `git diff` and requiring every changed line to start with // would be
# cheaper and answers a different question: a diff reports which LINES differ,
# not whether any code moved. See tools/codehash/main.go for why it is also
# unsound line by line.
.PHONY: codehash
codehash: ## Fingerprint FILES=... by code alone, ignoring comments
	@test -n "$(FILES)" || { echo "usage: make codehash FILES=\"a.go b.go\"" >&2; exit 2; }
	@go run ./tools/codehash $(FILES)

.PHONY: pre-commit
pre-commit: fmt vet narration test lint-if-available js-if-available ## Format, vet, narration, test, lint, and check app.js

# A HOOK CANNOT BE COMMITTED. .git/hooks is not tracked, so "symlink pre-commit
# into it" is per-checkout setup somebody has to actually run -- and until this
# target existed, CLAUDE.md and the since-merged docs/agents/workflow.md both
# described the symlink as though it were already there. It was not, in any checkout anyone
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
