BIN        := fisc
CMD        := ./cmd/fisc
OUT        := bin/$(BIN)
PREFIX     ?= /usr/local
# Prefer a repo-local venv, then one on PATH. Override with PYTHON=... for a
# venv elsewhere. Any Python 3 will do -- tools/extract.py is standard library
# only. The extractor itself is poppler-utils on PATH; see README.md.
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
	gofmt -w ./cmd ./internal ./pkg ./schema ./site ./tools

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy go.mod/go.sum
	go mod tidy

# narration refuses the two phrases that only ever introduce history into a
# comment -- a review credit and an erratum -- per AGENTS.md, "History's home is
# git". Deliberately two narrow phrases: "an earlier version" has honest uses
# about a reprinted DOCUMENT. tools/narration matches over each run of comment
# lines joined, since gofmt wraps a phrase anywhere, and fails on a path it
# cannot read.
#
# The bead arm reads the committed export and is a full gate; its phrase list is
# its own (see tools/errata). The memory arm reads bd's memories, fails when bd
# can answer and warns and continues when it cannot, so it is advisory and never
# a CI gate.
.PHONY: narration
narration: ## Refuse review credits and errata in Go sources, beads and memories
	@go run ./tools/narration ./cmd ./internal ./pkg ./schema ./site ./tools
	@go run ./tools/errata beads .beads/issues.jsonl
	@command -v bd >/dev/null 2>&1 || { \
		echo "warning: bd not on PATH, skipping the memory errata check" >&2; \
		exit 0; \
	}; \
	err=$$(mktemp "$${TMPDIR:-/tmp}/errata-bd-err.XXXXXX") || { \
		echo "warning: mktemp failed, skipping the memory errata check" >&2; \
		exit 0; \
	}; \
	memories=$$(bd memories --json 2>"$$err") || { \
		echo "warning: 'bd memories --json' failed, skipping the memory errata check:" >&2; \
		sed 's/^/  /' "$$err" >&2; \
		rm -f "$$err"; \
		exit 0; \
	}; \
	rm -f "$$err"; \
	printf '%s' "$$memories" | go run ./tools/errata memories

# beadrefs refuses a bead id that names no bead, fisc-* and byob-* alike. It
# resolves against the committed .beads/issues.jsonl, so it is a real gate; where
# bd is present it is asked about ids the export lacks. It fails on a path in the
# list below it cannot read.
.PHONY: beadrefs
beadrefs: ## Refuse bead ids that name no bead, in prose and in comments
	@go run ./tools/beadrefs .beads/issues.jsonl \
		AGENTS.md CLAUDE.md README.md Makefile \
		.github docs cmd internal pkg tools site schema mappings data testdata

# doccheck refuses a citation naming no section (or bold lead phrase) of
# AGENTS.md, a citation outside the canonical form, a docs/ path naming no file,
# and a source path under internal/, pkg/, cmd/ or tools/ naming no file. It
# reads committed files only, so it is a real gate. The boundary of each arm is
# in the tool's package comment.
.PHONY: doccheck
doccheck: ## Refuse dead citations, malformed citations, and dead docs/ and source paths
	@go run ./tools/doccheck AGENTS.md \
		AGENTS.md CLAUDE.md README.md Makefile \
		.github docs cmd internal pkg tools site schema mappings data testdata

# lint-if-available warns and continues when golangci-lint is absent, so it is
# not mandatory to commit; `lint` itself hard-fails, as CI's required job.
.PHONY: lint-if-available
lint-if-available: ## Run lint, warning rather than failing if golangci-lint is absent
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "warning: golangci-lint not on PATH, skipping lint; CI will still lint this commit" >&2; \
		exit 0; \
	}; \
	$(MAKE) --no-print-directory lint

# js runs node's test runner over site/*.test.mjs under jsdom. Node is off the
# deploy path, and `npm ci` runs once per lockfile change: npm writes
# node_modules/.package-lock.json last, which makes it the stamp. The glob is
# quoted so node expands it.
#
# The run is capped by a cgroup, not V8: a failing assert over a mark formats the
# laid graph off the V8 heap (a 2 GB heap_size_limit child reached 7.2 GB RSS on
# a 7.8 GiB host, render.test.mjs, 2026-10-10). Under systemd-run an oom-kill is
# read back from the scope and named; with no user systemd (CI) the run is
# uncapped. Measured at concurrency 2, 2026-10-10: every file but render peaks at
# 735 MB of summed node RSS, render alone at about 915 MB. JS_FILES narrows it.
JS_FILES ?= "site/*.test.mjs"
JS_MEM ?= 3G
JS_JOBS ?= 2

node_modules/.package-lock.json: package-lock.json
	npm ci --ignore-scripts --no-audit --no-fund

.PHONY: js
js: node_modules/.package-lock.json ## Run the client's tests under node and jsdom, memory-capped (needs node and npm; nothing else does)
	@if systemd-run --user --scope -q true 2>/dev/null; then \
		unit=fisc-js-$$$$; \
		systemd-run --user --scope -q --unit=$$unit -p MemoryMax=$(JS_MEM) -p MemorySwapMax=0 \
			node --test --test-concurrency=$(JS_JOBS) $(JS_FILES); status=$$?; \
		if [ "$$(systemctl --user show -p Result --value $$unit.scope 2>/dev/null)" = oom-kill ]; then \
			echo "js: the run was killed at the $(JS_MEM) memory cap; look for an assert whose operand is a mark or a laid node (see AGENTS.md, The node boundary)" >&2; \
			systemctl --user reset-failed $$unit.scope 2>/dev/null; \
		fi; \
		exit $$status; \
	else \
		node --test --test-concurrency=$(JS_JOBS) $(JS_FILES); \
	fi

# js-if-available is to `js` what lint-if-available is to `lint`.
.PHONY: js-if-available
js-if-available: ## Run js, warning rather than failing if node or npm is absent
	@command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1 || { \
		echo "warning: node or npm not on PATH, skipping the client tests; CI will still run them" >&2; \
		exit 0; \
	}; \
	$(MAKE) --no-print-directory js

# codehash fingerprints Go files by code alone, to prove a change comments-only:
#
#     make codehash FILES="internal/geom/geom.go" > /tmp/before
#     ...rewrite the comments...
#     make codehash FILES="internal/geom/geom.go" | diff /tmp/before -
.PHONY: codehash
codehash: ## Fingerprint FILES=... by code alone, ignoring comments
	@test -n "$(FILES)" || { echo "usage: make codehash FILES=\"a.go b.go\"" >&2; exit 2; }
	@go run ./tools/codehash $(FILES)

.PHONY: pre-commit
pre-commit: fmt vet narration beadrefs doccheck test facts verify lint-if-available js-if-available ## Format, vet, narration, beadrefs, doccheck, test, facts, verify, lint, and check app.js

# The Go tests do not re-run the checks over the committed corpus, nor
# rebuild it from the mappings: these do, as CI's facts and verify jobs do.
.PHONY: facts verify
facts: build ## Rebuild facts.jsonl from the mappings and compare it to the committed one
	./$(OUT) build --output bin/facts-rebuilt.jsonl
	@cmp -s bin/facts-rebuilt.jsonl facts/facts.jsonl || { \
		echo "facts/facts.jsonl is not what the mappings produce: $$(diff facts/facts.jsonl bin/facts-rebuilt.jsonl | grep -c '^[<>]') line(s) differ, the first of them:" >&2; \
		diff -u facts/facts.jsonl bin/facts-rebuilt.jsonl | head -n 50 >&2; exit 1; }

verify: build ## Run fisc verify --strict over the committed corpus
	./$(OUT) verify --strict

# hooks installs where git reads hooks (`git rev-parse --git-path hooks`, which
# is core.hooksPath when set), and reads the result back before claiming success.
# It refuses a directory outside this repository, a hook it does not own (it
# never appends: an arm after `exec` never runs, and shell appended to a
# non-shell hook breaks every commit), and a hook git tracks, as .beads/hooks is
# here. CI is the gate; the local hook is a convenience.
.PHONY: hooks
hooks: ## Install the local pre-commit hook where git reads it, or refuse and say why
	@dir="$$(git rev-parse --git-path hooks)"; \
	test -n "$$dir" || { echo "could not resolve the hooks directory; not a git checkout?" >&2; exit 1; }; \
	top="$$(git rev-parse --show-toplevel)"; \
	common="$$(cd "$$(git rev-parse --git-common-dir)" 2>/dev/null && pwd)"; \
	maintop="$$(git -C "$$common/.." rev-parse --show-toplevel 2>/dev/null)"; \
	test -n "$$common" || common="$$top/.git"; \
	test -n "$$maintop" || maintop="$$top"; \
	case "$$dir" in \
		/*) abs="$$dir";; \
		*) abs="$$top/$$dir";; \
	esac; \
	case "$$abs/" in \
		"$$top"/*) ;; \
		"$$common"/*) ;; \
		"$$maintop"/*) ;; \
		*) echo "refusing: $$dir is outside this repository." >&2; \
		   echo "  core.hooksPath is set to a directory git uses for OTHER repos" >&2; \
		   echo "  too -- probably a global setting -- so installing here would run" >&2; \
		   echo "  'make pre-commit' on every commit you make anywhere." >&2; \
		   exit 1;; \
	esac; \
	if [ "$$dir" != ".git/hooks" ] && [ -e .git/hooks/pre-commit ]; then \
		echo "note: .git/hooks/pre-commit exists and git does NOT run it, because" >&2; \
		echo "  core.hooksPath points at $$dir. It is dead; remove it if you like." >&2; \
	fi; \
	hook="$$dir/pre-commit"; \
	if git ls-files --error-unmatch "$$hook" >/dev/null 2>&1; then \
		echo "refusing: git tracks $$hook, so this target will not write it." >&2; \
		echo "  core.hooksPath points at a tracked directory, which means a hook" >&2; \
		echo "  installed here is either an uncommitted change to a tracked file" >&2; \
		echo "  or a commit that makes 'make pre-commit' mandatory for everyone" >&2; \
		echo "  who has bd -- including a contributor with no Go toolchain." >&2; \
		echo "  Run 'make pre-commit' yourself before committing; CI is the gate." >&2; \
		exit 1; \
	fi; \
	ours=no; \
	if [ ! -e "$$hook" ] && [ ! -L "$$hook" ]; then \
		ours=new; \
	elif [ -L "$$hook" ]; then \
		case "$$(readlink "$$hook")" in pre-commit.fisc) ours=yes;; esac; \
	elif [ "$$(cat "$$hook" 2>/dev/null)" = "$$(printf '#!/bin/sh\nexec "$$(dirname "$$0")/pre-commit.fisc"')" ]; then \
		ours=yes; \
	fi; \
	if [ "$$ours" = no ]; then \
		echo "refusing: $$hook already exists and is not ours." >&2; \
		echo "  Another tool owns it, and this target will not append to a hook" >&2; \
		echo "  it did not write: an arm added after an 'exec' or an 'exit 0'" >&2; \
		echo "  never runs, and shell syntax appended to a non-shell hook breaks" >&2; \
		echo "  every commit. Chain it yourself if you want both:" >&2; \
		echo "    \"\$$(dirname \"\$$0\")/pre-commit.fisc\" || exit \$$?" >&2; \
		exit 1; \
	fi; \
	mkdir -p "$$dir" || exit 1; \
	printf '#!/bin/sh\nexec make pre-commit\n' > "$$dir/pre-commit.fisc"; \
	chmod +x "$$dir/pre-commit.fisc"; \
	if [ "$$ours" = new ]; then \
		printf '#!/bin/sh\nexec "$$(dirname "$$0")/pre-commit.fisc"\n' > "$$hook"; \
	fi; \
	chmod +x "$$hook" 2>/dev/null || true; \
	back="$$(git rev-parse --git-path hooks)"; \
	reaches=no; \
	if [ -L "$$back/pre-commit" ]; then \
		case "$$(readlink "$$back/pre-commit")" in pre-commit.fisc) reaches=yes;; esac; \
	elif [ "$$(cat "$$back/pre-commit" 2>/dev/null)" = "$$(printf '#!/bin/sh\nexec "$$(dirname "$$0")/pre-commit.fisc"')" ]; then \
		reaches=yes; \
	fi; \
	if [ ! -x "$$back/pre-commit" ] || [ ! -x "$$back/pre-commit.fisc" ] || [ "$$reaches" = no ]; then \
		echo "refusing to claim success: $$back/pre-commit does not invoke pre-commit.fisc" >&2; \
		exit 1; \
	fi; \
	echo "installed $$back/pre-commit -> pre-commit.fisc (runs 'make pre-commit')"

# Extraction is not part of the Go binary; its output is committed.
# DOC=<id> extracts one document; omit it for all three.
.PHONY: extract
extract: ## Re-extract PDFs with poppler (DOC=<id> for one)
	$(PYTHON) tools/extract.py $(if $(DOC),--doc $(DOC))

# -B so the import leaves no tools/__pycache__; -E -s so neither PYTHON*
# variables nor user site-packages reach it. Not -I, which would also drop
# the script's own directory, where extract.py is.
.PHONY: extract-test
extract-test: ## Hold extract.py's schema subset to the cases Go refuses
	$(PYTHON) -B -E -s tools/extract_test.py

.PHONY: extract-list
extract-list: ## List extractable documents
	$(PYTHON) tools/extract.py --list

# `fisc export` reads facts/facts.jsonl and never re-runs the mapping engine.
.PHONY: site
site: build ## Build the static site into dist/
	$(OUT) export --clean
	@echo
	@echo "  Serve it:  python3 -m http.server -d dist 8000"
	@echo

.PHONY: clean
clean: ## Remove build output
	rm -rf bin dist
