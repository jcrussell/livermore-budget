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
# IT WARNS AND CONTINUES WHENEVER bd CANNOT ANSWER -- absent from PATH, or
# present and failing, which is what a fresh clone with no Dolt DB looks like.
# Without the second of those a contributor with Go and no beads database could
# not commit at all, and "a contributor with only Go can still land a change" is
# a property this project keeps.
#
# An empty memory set warns for the same reason, one layer down in memcheck: a
# checkout between `bd init` and a pull of the project's memories has a working
# bd and nothing for this to read, and must still be able to commit.
#
# THE COST OF THAT IS REAL AND IS THE REASON IT IS SPELT OUT. This arm FAILS like
# any other WHEN bd CAN ANSWER, and skips whenever it cannot -- three ways above,
# and a fourth inside memcheck for a database holding no memories. What it can
# never be is a CI gate, because memories live in the Dolt DB and in no git
# artifact. So a green memory arm is evidence only that the memories were
# readable and clean on THIS machine, and a skip is not evidence of anything.
#
# The Go arm has no such limit -- it reads committed source -- and CI runs this
# target for it. Between them the target is a real gate over the tree and an
# advisory check over the database.
.PHONY: narration
narration: ## Refuse review credits and comment errata in Go sources and memories
	@hits=$$(grep -rnE '//.*(Found by /code-review|this comment used to say)' \
		--include='*.go' ./cmd ./internal ./pkg ./site ./tools); \
	status=$$?; \
	if [ $$status -gt 1 ]; then \
		if [ -n "$$hits" ]; then echo "$$hits" >&2; fi; \
		echo "narration: grep failed with status $$status; the directory list above" >&2; \
		echo "  is hand-maintained and one of its entries is probably gone." >&2; \
		exit 1; \
	fi; \
	if [ -n "$$hits" ]; then \
		echo "$$hits" >&2; \
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
	err=$$(mktemp "$${TMPDIR:-/tmp}/memcheck-bd-err.XXXXXX") || { \
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
	printf '%s' "$$memories" | go run ./tools/memcheck

# beadrefs refuses a bead id that names no bead.
#
# An invented id does not read as a mistake: the work really is tracked and only
# the pointer is dead, so it reads as done and nobody goes looking. It has
# happened twice, and it was a prose rule in AGENTS.md until now.
#
# UNLIKE narration'S MEMORY ARM THIS IS A REAL GATE. It resolves against
# .beads/issues.jsonl, which is committed, so it needs no bd, no Dolt server and
# no network, and CI runs it. Where bd IS present it is asked about ids the
# export lacks, which is the window between filing a bead and committing the
# export.
#
# The path list is hand-maintained and beadrefs fails on a path it cannot read,
# so an entry deleted from the tree takes this red rather than silently
# narrowing the scan.
.PHONY: beadrefs
beadrefs: ## Refuse bead ids that name no bead, in prose and in comments
	@go run ./tools/beadrefs .beads/issues.jsonl \
		AGENTS.md CLAUDE.md README.md Makefile requirements.txt \
		.github docs cmd internal pkg tools site mappings data testdata

# doccheck refuses a citation that names no section of AGENTS.md.
#
# The tree cites AGENTS.md by section name from Go, from this file and from
# site/app.js -- thirteen of them at the time this landed -- and nothing checked
# the strings. So renaming a section left every citation of it pointing at
# nothing, which is worse than no pointer: it reads as though the rule is
# written down and sends the reader looking for a heading that is gone.
#
# THREE OF THEM ARE PRINTED TO A TERMINAL rather than only sitting in a comment
# -- narration's failure message above, tools/memcheck and tools/beadrefs -- so a
# stale one is a false claim made to a user already dealing with a failure.
#
# A BOLD LEAD PHRASE IS AN ANCHOR TOO, and that is not a nicety: four of the
# thirteen cite "History's home is git", which is bolded text inside a section
# and not a heading at all.
#
# Like beadrefs and unlike narration's memory arm, this is a real gate. It reads
# the committed AGENTS.md and nothing else -- no bd, no Dolt, no node, no
# network -- so CI runs it too.
.PHONY: doccheck
doccheck: ## Refuse citations that name no section of AGENTS.md
	@go run ./tools/doccheck AGENTS.md \
		AGENTS.md CLAUDE.md README.md Makefile requirements.txt \
		.github docs cmd internal pkg tools site mappings data testdata

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
pre-commit: fmt vet narration beadrefs doccheck test lint-if-available js-if-available ## Format, vet, narration, beadrefs, doccheck, test, lint, and check app.js

# A HOOK CANNOT BE COMMITTED. The hooks directory is not tracked, so "install
# the pre-commit hook" is per-checkout setup somebody has to actually run -- and
# until this target existed, CLAUDE.md and the since-merged docs/agents/workflow.md
# both described the symlink as though it were already there. It was not, in any
# checkout anyone looked at, which made a workflow document assert a guard that
# did not exist.
#
# THE SECOND VERSION OF THAT DEFECT WAS THIS TARGET'S OWN. It wrote into
# .git/hooks unconditionally and announced success. But core.hooksPath REPLACES
# .git/hooks rather than adding to it, and bd sets it to .beads/hooks here, so
# git never read what this installed: the symlink and its script sat on disk for
# a week, the local gate never ran once, and the target printed that it had
# installed a hook that runs `make pre-commit`. A false claim the program prints
# is the worst kind, so the path now comes from git and the claim is READ BACK
# from the file git will actually execute before anything is printed.
#
# `git rev-parse --git-path hooks` is the one question that answers this: it
# returns core.hooksPath when set and .git/hooks when not.
#
# IT REFUSES A HOOK IT DOES NOT OWN, rather than appending to it. Appending was
# the first attempt and it was wrong three ways at once: a hook ending in `exec`
# or `exit 0` never reaches an appended arm, and the read-back below greps for
# the string rather than for reachability, so it would have printed success over
# a gate that could not run; appending shell syntax to a hook that is not shell
# -- the pre-commit framework installs a Python one -- makes every commit die on
# a SyntaxError with nothing to say why; and retargeting an existing symlink is
# clobbering whatever owns it. All three were found by review over this range.
#
# BUT IT REFUSES A HOOK GIT TRACKS, and that is the case here: core.hooksPath
# points at .beads/hooks, whose six hooks are committed files. Writing there
# would either leave the tree permanently dirty or, if committed, turn an opt-in
# convenience into a hook that fires for everyone who has bd -- including a
# contributor with no Go toolchain, who could then not commit at all. The
# paragraph below is the rule that forbids that, so this refuses and says what
# the options are instead of quietly making the trade.
#
# THE LOCAL HOOK IS A CONVENIENCE AND CI IS THE GATE. A contributor who never
# runs this target is not doing anything wrong; the required lint and verify
# jobs still fail their PR. Keep it that way -- a repository whose correctness
# depends on every clone having run a setup step has no gate at all.
.PHONY: hooks
hooks: ## Install the local pre-commit hook where git reads it, or refuse and say why
	@dir="$$(git rev-parse --git-path hooks)"; \
	test -n "$$dir" || { echo "could not resolve the hooks directory; not a git checkout?" >&2; exit 1; }; \
	top="$$(git rev-parse --show-toplevel)"; \
	case "$$(cd "$$(dirname "$$dir")" 2>/dev/null && pwd)/" in \
		"$$top"/*|"$$top"/) ;; \
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
