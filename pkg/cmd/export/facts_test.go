package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// committedStore reads the real facts/facts.jsonl, both ways.
func committedStore(t *testing.T) ([]byte, []fact.Fact) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	raw, facts, err := readFactStore(root)
	if err != nil {
		t.Fatalf("read the fact store: %v", err)
	}
	return raw, facts
}

// TestFactShardsReconstituteTheCommittedStore is the claim the whole published
// store rests on, asserted over the real file rather than over a fixture.
//
// Concatenating the shards in order must reproduce facts.jsonl byte for byte.
// That is one equality and it covers three failures at once: no fact lost, none
// invented, none rewritten. It holds because fact.less opens on DocID then
// Page, so (doc_id, page) is a strict PREFIX of the total order and each shard
// is a contiguous run of the file.
//
// Asserted here as well as inside buildFactAssets because the two answer
// different questions. The builder proves it about whatever store is being
// shipped, on every run; this proves it about the store as committed, which is
// the one a reader will actually download.
func TestFactShardsReconstituteTheCommittedStore(t *testing.T) {
	raw, facts := committedStore(t)
	assets, err := buildFactAssets(raw, facts, "fisc test")
	if err != nil {
		t.Fatalf("buildFactAssets: %v", err)
	}

	var concat bytes.Buffer
	for _, p := range assets.Pages {
		b, ok := assets.Files[p.Path]
		if !ok {
			t.Fatalf("index names %s and no such file was produced", p.Path)
		}
		concat.Write(b)
	}
	if !bytes.Equal(concat.Bytes(), raw) {
		t.Fatalf("the shards do not reproduce the store: %d bytes against %d",
			concat.Len(), len(raw))
	}

	// AND THE COUNTS ADD UP, which the byte equality alone does not say: a
	// shard could carry the right bytes under a page number read off the wrong
	// fact and the concatenation would still match.
	total := 0
	for _, p := range assets.Pages {
		total += p.Facts
		first := strings.SplitN(string(assets.Files[p.Path]), "\n", 2)[0]
		var f fact.Fact
		if err := json.Unmarshal([]byte(first), &f); err != nil {
			t.Fatalf("decode the first line of %s: %v", p.Path, err)
		}
		if f.DocID != p.DocID || f.Page != p.Page {
			t.Errorf("%s opens on %s p%d, want %s p%d", p.Path, f.DocID, f.Page, p.DocID, p.Page)
		}
		if want := shardPath(f.DocID, f.Page); p.Path != want {
			t.Errorf("shard path %q, want %q computed from the locator", p.Path, want)
		}
	}
	if total != len(facts) {
		t.Errorf("the shards account for %d facts, the store holds %d", total, len(facts))
	}
	if assets.Facts != len(facts) || assets.Bytes != len(raw) {
		t.Errorf("assets report %d facts in %d bytes, want %d in %d",
			assets.Facts, assets.Bytes, len(facts), len(raw))
	}
}

// TestBuildFactAssetsRefusesAStoreItCannotReproduce mutates the INPUT, not an
// in-memory structure, which is this project's standard for proving a check
// can fail: a store whose keys are in a different order on one line is exactly
// what a hand-edited facts.jsonl looks like, and it re-encodes to something
// else.
func TestBuildFactAssetsRefusesAStoreItCannotReproduce(t *testing.T) {
	raw, facts := committedStore(t)
	lines := bytes.SplitN(raw, []byte("\n"), 2)
	var first map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Re-marshalling through a map sorts the keys, which is precisely the
	// silent re-ordering fact.Write's declaration-order guarantee forbids.
	reordered, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	edited := append(append([]byte{}, reordered...), '\n')
	edited = append(edited, lines[1]...)

	_, err = buildFactAssets(edited, facts, "fisc test")
	if err == nil {
		t.Fatal("buildFactAssets accepted a store its shards do not reproduce")
	}
	for _, want := range []string{"do not reproduce", "differ at byte"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not say %q", err, want)
		}
	}
}

// TestBuildFactAssetsRefusesAnUnsortedStore is the same refusal one step
// earlier and with a better message. Every claim the sharding makes rests on
// the total order; a store out of it does not shard into contiguous runs at
// all, and reporting that as a byte mismatch would tell a reader nothing.
func TestBuildFactAssetsRefusesAnUnsortedStore(t *testing.T) {
	raw, facts := committedStore(t)
	swapped := append([]fact.Fact{}, facts...)
	// Across a page boundary: find the first fact of a later page and put it
	// first, so the store's DocID/Page runs are broken rather than merely its
	// within-page order.
	for i := 1; i < len(swapped); i++ {
		if swapped[i].Page != swapped[0].Page {
			swapped[0], swapped[i] = swapped[i], swapped[0]
			break
		}
	}
	_, err := buildFactAssets(raw, swapped, "fisc test")
	if err == nil {
		t.Fatal("buildFactAssets accepted an unsorted store")
	}
	if !strings.Contains(err.Error(), "not in its total order") {
		t.Errorf("error %q does not name the order", err)
	}
}

// TestTheCSVHeaderIsEveryFactFieldInOrder pins the column list against the
// struct that defines it.
//
// ORDER, NOT MEMBERSHIP. fact.Fact's doc comment says field order IS the JSON
// key order and that the order is part of the format; a set comparison would
// leave the one property the CSV promises untested. And it is compared against
// a MARSHALLED FACT rather than against another reflection pass, so the test
// and the code do not share their one possible mistake.
func TestTheCSVHeaderIsEveryFactFieldInOrder(t *testing.T) {
	b, err := json.Marshal(fact.Fact{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys []string
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil { // '{'
		t.Fatalf("open: %v", err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("key: %v", err)
		}
		keys = append(keys, tok.(string))
		var discard any
		if err := dec.Decode(&discard); err != nil {
			t.Fatalf("value: %v", err)
		}
	}

	got := factCSVHeader()
	if len(got) != len(keys) {
		t.Fatalf("header has %d columns, fact.Fact marshals %d keys", len(got), len(keys))
	}
	for i := range keys {
		if got[i] != keys[i] {
			t.Errorf("column %d is %q, want %q", i, got[i], keys[i])
		}
	}
}

// TestTheCSVCarriesAmountCentsVerbatim is the no-float assertion.
//
// AGENTS.md's provenance invariants forbid float amounts, and a CSV is where that
// rule is easiest to break by accident: decode to float64, format, and
// 6999000000 becomes 6.999e+09. Transcoding through json.Number means the
// literal text never becomes a number at all, and this compares the CSV's cell
// against the JSONL's characters to say so.
func TestTheCSVCarriesAmountCentsVerbatim(t *testing.T) {
	raw, facts := committedStore(t)
	out, err := factsCSV(raw)
	if err != nil {
		t.Fatalf("factsCSV: %v", err)
	}
	rows, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("read back the CSV: %v", err)
	}
	if len(rows) != len(facts)+1 {
		t.Fatalf("CSV has %d rows for %d facts plus a header", len(rows), len(facts))
	}
	col := -1
	for i, name := range rows[0] {
		if name == "amount_cents" {
			col = i
		}
	}
	if col < 0 {
		t.Fatal("the CSV has no amount_cents column")
	}

	// The JSONL's own characters, taken off the line rather than off the
	// decoded fact -- comparing against strconv of an int64 would pass through
	// the very conversion this test exists to rule out.
	for i, line := range bytes.Split(bytes.TrimRight(raw, "\n"), []byte("\n")) {
		want := literalOf(t, line, "amount_cents")
		if got := rows[i+1][col]; got != want {
			t.Fatalf("line %d: CSV amount_cents %q, JSONL %q", i+1, got, want)
		}
	}
	// And one spot check that the value is a plain integer, so a store of all
	// zeroes could not satisfy the comparison above vacuously.
	if strings.ContainsAny(rows[1][col], ".eE") {
		t.Errorf("amount_cents %q is not an integer literal", rows[1][col])
	}
}

// literalOf pulls one key's raw literal text out of a JSON object.
func literalOf(t *testing.T, line []byte, key string) string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.UseNumber()
	if _, err := dec.Token(); err != nil {
		t.Fatalf("open: %v", err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("key: %v", err)
		}
		name := tok.(string)
		val, err := dec.Token()
		if err != nil {
			t.Fatalf("value: %v", err)
		}
		if name == key {
			return fmt.Sprint(val)
		}
	}
	t.Fatalf("line carries no %q", key)
	return ""
}

// TestTheCSVQuotesTokensThatCarryACommaAndRoundTrips is the reason the stdlib
// writer is used rather than a Sprintf: 1,110 of the store's fields are grouped
// numbers with commas in them, so a hand-rolled join would shift every column
// after token on those lines.
func TestTheCSVQuotesTokensThatCarryACommaAndRoundTrips(t *testing.T) {
	raw, _ := committedStore(t)
	out, err := factsCSV(raw)
	if err != nil {
		t.Fatalf("factsCSV: %v", err)
	}
	rows, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("read back the CSV: %v", err)
	}
	width := len(rows[0])
	quoted := 0
	for i, row := range rows {
		if len(row) != width {
			t.Fatalf("row %d has %d fields, the header has %d", i, len(row), width)
		}
		for _, cell := range row {
			if strings.Contains(cell, ",") {
				quoted++
			}
		}
	}
	if quoted == 0 {
		t.Fatal("no field carries a comma, so this test asserts nothing about quoting")
	}
	t.Logf("%d fields carry a comma and survived the round trip", quoted)
}

// TestTheCSVRefusesALineMissingAKey closes csv.Writer's own gap: it enforces no
// field count -- only csv.Reader does -- so a short line would ship a short row
// in silence, and every column after the gap would be shifted in a file that
// still parses.
func TestTheCSVRefusesALineMissingAKey(t *testing.T) {
	raw, _ := committedStore(t)
	lines := bytes.SplitN(raw, []byte("\n"), 2)
	var first map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Rebuild the line in declaration order, minus one key, so the failure is
	// the missing key and not the re-ordering.
	var b bytes.Buffer
	b.WriteByte('{')
	for i, name := range factCSVHeader() {
		if name == "department" {
			continue
		}
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		v, err := json.Marshal(first[name])
		if err != nil {
			t.Fatalf("encode %s: %v", name, err)
		}
		fmt.Fprintf(&b, "%q:%s", name, v)
		_ = i
	}
	b.WriteByte('}')
	short := append(append(b.Bytes(), '\n'), lines[1]...)

	if _, err := factsCSV(short); err == nil {
		t.Fatal("factsCSV accepted a line missing a key")
	} else if !strings.Contains(err.Error(), "department") {
		t.Errorf("error %q does not name the key that went missing", err)
	}
}

// TestTheCSVRefusesATruncatedStore is the failure mode a loop that stops on
// io.EOF invites, and it is worse than it sounds.
//
// json.Decoder.Token() returns io.EOF at the end of the stream AND at MOST
// points inside a cut-off object. Measured, because the first version of this
// test got it wrong: a cut after a key, after a colon, mid-number, or straight
// after the opening brace all return bare io.EOF; only a cut inside a string
// literal returns io.ErrUnexpectedEOF. So a transcoder that treats io.EOF as
// "done" writes a CSV silently missing its tail, which parses perfectly and
// carries no sign of what it lost.
//
// EVERY CUT POSITION IN THE FINAL RECORD, not one. The first version cut at a
// fixed offset, landed inside a string, and passed through the one shape the
// rename is not needed for -- so it stayed green with the guard removed. A
// test that proves a guard by accident is the thing this repository keeps
// finding in its own history.
func TestTheCSVRefusesATruncatedStore(t *testing.T) {
	raw, _ := committedStore(t)
	// TWO RECORDS, NOT 1,448. The property is about one cut-off object, and
	// re-transcoding 718 KB per cut position costs twelve seconds to learn
	// nothing extra.
	lines := bytes.SplitN(raw, []byte("\n"), 3)
	body := bytes.Join(lines[:2], []byte("\n"))
	last := len(lines[0]) + 1

	if _, err := factsCSV(append(append([]byte{}, body...), '\n')); err != nil {
		t.Fatalf("factsCSV on the intact pair: %v", err)
	}

	// FROM last+1: cutting at exactly the record boundary leaves a store that
	// is one record shorter and perfectly well formed, and factsCSV cannot know
	// that -- it transcodes what it is given and has no expected count. What
	// catches THAT is buildFactAssets' byte equality against facts.jsonl, which
	// runs over the same bytes before this function ever sees them. The
	// division of labour is worth stating: this refuses a record it cannot
	// finish reading, the reconciliation refuses a store that is not the store.
	for cut := last + 1; cut < len(body); cut++ {
		out, err := factsCSV(body[:cut])
		if err != nil {
			continue
		}
		got, rerr := csv.NewReader(bytes.NewReader(out)).ReadAll()
		if rerr != nil {
			t.Fatalf("cut at %d: read back: %v", cut, rerr)
		}
		t.Fatalf("factsCSV accepted a record truncated at byte %d (%q...) and wrote "+
			"%d rows against the two it was given",
			cut-last, body[last:min(last+30, cut)], len(got)-1)
	}
}

// TestTheFactIndexEnumeratesEveryShardAndIsNotOnTheResolutionPath.
//
// The second half is the one worth an assertion. A locator resolves by
// COMPUTING facts/<doc>/pages/pNNNN.jsonl; the index exists to build a page and
// to check a download. So every path it publishes must equal what shardPath
// returns for the same locator -- if the two could disagree, the index would be
// load-bearing after all.
func TestTheFactIndexEnumeratesEveryShardAndIsNotOnTheResolutionPath(t *testing.T) {
	raw, facts := committedStore(t)
	assets, err := buildFactAssets(raw, facts, "fisc test")
	if err != nil {
		t.Fatalf("buildFactAssets: %v", err)
	}
	var doc factsIndexDoc
	if err := json.Unmarshal(assets.Files[factsIndexPath], &doc); err != nil {
		t.Fatalf("decode the index: %v", err)
	}
	if doc.Store.Pages != len(assets.Pages) || doc.Store.Facts != len(facts) {
		t.Errorf("index reports %d pages and %d facts, want %d and %d",
			doc.Store.Pages, doc.Store.Facts, len(assets.Pages), len(facts))
	}
	for _, p := range doc.Pages {
		if want := shardPath(p.DocID, p.Page); p.Path != want {
			t.Errorf("index path %q for %s p%d, want the computed %q",
				p.Path, p.DocID, p.Page, want)
		}
		b, ok := assets.Files[p.Path]
		if !ok {
			t.Errorf("index names %s and no such file was produced", p.Path)
			continue
		}
		if p.Bytes != len(b) {
			t.Errorf("%s: index says %d bytes, the file is %d", p.Path, p.Bytes, len(b))
		}
	}
}

// TestEveryShardedPageHasItsExtractedText is the p76 case, found before it
// shipped: at the time the store covered 21 pages and the site was shipping the
// text of 20, because p76 is cited by no projection's metadata.sources. A
// provenance link that resolves to a shard whose page text 404s is provenance
// the site does not actually ship. Both counts have moved since -- do not read
// 21 as current. What this test asserts is one direction only: every page the
// store shards has its extracted text committed. The equality the p76 case was
// really about is kept by pageIndex() seeding buildSite's cited set.
//
// NOTE WHAT THAT LEAVES, AND NOTE THAT A SECOND DOCUMENT DID NOT CHANGE IT. This
// arm still cannot go red. The store now spans two documents -- the Budget Book
// topping out at p170 and ACFR p41 -- and BOTH are extracted contiguously and in
// full, p0001..p0268 and p0001..p0195, so every page the store can shard has its
// text by construction exactly as before. fisc-73cq was filed on the premise
// that a second document would make this falsifiable; that premise was wrong,
// measured at 0448a5b. The bead is CLOSED -- on the guard below, not on the
// falsifiability -- and its close reason records the correction.
// What would make it live is a PARTIALLY extracted document, not another one.
// fact-offset-points-at-token forecloses the underlying case from the other
// side in the meantime.
//
// The empty-store guard below is the half of fisc-73cq that was always worth
// having, and it is a different failure: without it this test passes on a store
// with no pages at all, which is the vacuity internal/check/vacuity.go exists
// for, one package over and undeclared. Same shape as the guard in
// TestTheSiteLinksEveryShardItShips.
func TestEveryShardedPageHasItsExtractedText(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	raw, facts := committedStore(t)
	assets, err := buildFactAssets(raw, facts, "fisc test")
	if err != nil {
		t.Fatalf("buildFactAssets: %v", err)
	}
	if len(assets.Pages) == 0 {
		t.Fatal("the store shards no pages, so this test asserts nothing")
	}
	// Two documents are shipped, and the loop below must reach both: a guard
	// that only counted pages would be satisfied by a store that had silently
	// lost one of them.
	docs := map[string]bool{}
	for _, p := range assets.Pages {
		docs[p.DocID] = true
	}
	if len(docs) < 2 {
		t.Errorf("the store shards %d document(s), want at least 2; if a document was "+
			"dropped from mappings/ say so here, because this test is the one that "+
			"would otherwise keep passing over the remainder", len(docs))
	}
	for _, p := range assets.Pages {
		rel := filepath.Join(root, filepath.FromSlash(cmdutil.ExtractedDir),
			p.DocID, "pages", fmt.Sprintf("p%04d.txt", p.Page))
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("%s p%d carries %d facts and its extracted text is not committed: %v",
				p.DocID, p.Page, p.Facts, err)
		}
	}
}

// TestTheSiteLinksEveryShardItShips applies unviewedDocuments' standard to
// assets: bytes no reader can reach are a defect.
//
// assertPublishedReachable governs PROJECTIONS -- it is why four fund-flows
// documents cannot ship as files no page opens. Nothing governed Files, so the
// fact store could have shipped 1.1 MB that no page linked and every check
// would have stayed green. This closes that for the store specifically: every
// key under facts/ is either a row of the page index or an offered download.
func TestTheSiteLinksEveryShardItShips(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}

	linked := map[string]bool{}
	for _, e := range built.PageIndex {
		linked[e.Data] = true
	}
	for _, d := range built.Downloads {
		linked[d.Path] = true
	}
	shipped := 0
	for rel := range built.Files {
		if !strings.HasPrefix(rel, FactsDir+"/") {
			continue
		}
		shipped++
		if !linked[rel] {
			t.Errorf("%s ships and no row or download links it", rel)
		}
	}
	if shipped == 0 {
		t.Fatal("no fact-store file shipped, so this test asserts nothing")
	}
	// And the other direction: a row naming a file that was not produced would
	// publish a dead link. export.Options.validate refuses it, but this says so
	// against the real corpus rather than against a fixture.
	for rel := range linked {
		if _, ok := built.Files[rel]; !ok {
			t.Errorf("the page links %s and no such file was produced", rel)
		}
	}
	t.Logf("%d fact-store files, all linked", shipped)
}

// TestTheProvenanceViewIsPublishedWhenThereIsAStore pins the fourth view into
// the real view set, which TestViewsOpensOnTheSpineAndGivesYearsToItAlone
// deliberately does not (it passes a Result with no page index).
func TestTheProvenanceViewIsPublishedWhenThereIsAStore(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}

	vs := views(built)
	var found *struct {
		path, nav, projection string
	}
	for _, v := range vs {
		if v.Template == "provenance.html.tmpl" {
			found = &struct{ path, nav, projection string }{v.Path, v.Nav, v.Projection}
		}
	}
	if found == nil {
		t.Fatalf("the view set has no provenance page: %d views", len(vs))
	}
	if found.path != "provenance.html" || found.nav == "" {
		t.Errorf("provenance view is %+v, want a labelled provenance.html", *found)
	}
	if found.projection != "" {
		t.Errorf("the provenance view names projection %q; it renders none", found.projection)
	}

	// AND IT IS ABSENT WITHOUT A STORE, so the nav never points at a page that
	// was not written -- the property views() exists to hold.
	for _, v := range views(Result{Projections: built.Projections}) {
		if v.Template == "provenance.html.tmpl" {
			t.Error("a provenance view was published with no page index behind it")
		}
	}
}

// TestTheIndexNamesOnlyFilesTheSiteWrites, under BOTH page-text modes.
//
// The test this replaces compared factPage.TextPath against pageTextPath() --
// the function that produced it -- so it asserted nothing and could not see
// that the field was wrong. It was: export.Write chooses between shipping the
// extracted text and citing a remote browsable copy, and it makes that choice
// AFTER this index is built, so the packager was publishing 21 paths that
// `fisc export --source-browse-url` does not write.
//
// The general property is the one worth pinning, rather than the one field:
// every path facts/index.json names is a file the site actually wrote. It runs
// over both modes because the defect existed in exactly one of them.
func TestTheIndexNamesOnlyFilesTheSiteWrites(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	for _, browse := range []string{"", "https://example.invalid/blob/main"} {
		name := "shipping the page text"
		if browse != "" {
			name = "citing a remote copy"
		}
		t.Run(name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()
			dir := filepath.Join(t.TempDir(), "dist")
			if err := exportRun(&Options{
				IO:              io,
				RepoRoot:        func() (string, error) { return root, nil },
				OutputDir:       dir,
				SourceBrowseURL: browse,
			}); err != nil {
				t.Fatalf("exportRun: %v", err)
			}

			b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(factsIndexPath)))
			if err != nil {
				t.Fatalf("read the index: %v", err)
			}
			var doc factsIndexDoc
			if err := json.Unmarshal(b, &doc); err != nil {
				t.Fatalf("decode the index: %v", err)
			}
			if len(doc.Pages) == 0 {
				t.Fatal("the index names no pages, so this test asserts nothing")
			}

			// Every string field of every entry that looks like a site path
			// must resolve. Walking the decoded JSON rather than named fields,
			// so a path field added later is covered without editing this.
			var entries []map[string]any
			if err := json.Unmarshal(b, &struct {
				Pages *[]map[string]any `json:"pages"`
			}{&entries}); err != nil {
				t.Fatalf("re-decode: %v", err)
			}
			checked := 0
			for _, e := range entries {
				for key, v := range e {
					s, ok := v.(string)
					if !ok || !strings.Contains(s, "/") || strings.Contains(s, "://") {
						continue
					}
					checked++
					if _, serr := os.Stat(filepath.Join(dir, filepath.FromSlash(s))); serr != nil {
						t.Errorf("index key %q names %q, which this export did not write: %v",
							key, s, serr)
					}
				}
			}
			if checked < len(doc.Pages) {
				t.Errorf("checked %d paths across %d entries; every entry names at least "+
					"its records", checked, len(doc.Pages))
			}
			for _, d := range doc.Pages {
				if _, serr := os.Stat(filepath.Join(dir,
					filepath.FromSlash(doc.CSVPath))); serr != nil {
					t.Fatalf("the index offers %q and it was not written: %v", doc.CSVPath, serr)
				}
				_ = d
			}
		})
	}
}

// TestCleanDoesNotDestroyASiteOverAFaultItCouldHaveSeen is the general form of
// the ordering this command has argued for twice and got wrong twice.
//
// FIRST MISS: assetPath ran inside export.Write, after SafeCleanDir, so one bad
// key from a Builder emptied the reader's site and then refused. Fixed by
// hoisting validation.
//
// SECOND MISS, which is what this test is really for: the hoist covered
// validation only, and withPageText was still inside Write. An export whose
// fact store covers a page the extraction does not still destroyed the output
// and then refused -- and the PageIndex seeding WIDENED that class, because it
// makes pages cited that no projection names. p76 is exactly such a page.
//
// So the assertion is not about page text. It is that a fault detectable
// without touching the filesystem never costs a reader their site, and it is
// driven through the page-text arm because that is the one that got left
// behind. export.Prepare resolves everything; only I/O can fail after it.
func TestCleanDoesNotDestroyASiteOverAFaultItCouldHaveSeen(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "dist")
	opts := func() *Options {
		io, _, _, _ := iostreams.Test()
		return &Options{
			IO:        io,
			RepoRoot:  func() (string, error) { return root, nil },
			OutputDir: dir,
		}
	}
	if rerr := exportRun(opts()); rerr != nil {
		t.Fatalf("seed export: %v", rerr)
	}
	before, err := os.ReadDir(dir)
	if err != nil || len(before) == 0 {
		t.Fatalf("seed export wrote nothing: %v", err)
	}

	// A COPY OF THE REPOSITORY'S EXTRACTION, minus one page the fact store
	// covers. The real tree is never touched -- a test that moved a committed
	// file aside would take the repository down with it if it failed midway.
	tree := t.TempDir()
	src := filepath.Join(root, filepath.FromSlash(cmdutil.ExtractedDir))
	if cerr := os.CopyFS(tree, os.DirFS(src)); cerr != nil {
		t.Fatalf("copy the extraction: %v", cerr)
	}
	gone := filepath.Join(tree, budgetDocIDForTest, "pages", "p0076.txt")
	if _, serr := os.Stat(gone); serr != nil {
		t.Fatalf("the page this test removes is not in the extraction: %v", serr)
	}
	if rmerr := os.Remove(gone); rmerr != nil {
		t.Fatalf("remove: %v", rmerr)
	}

	o := opts()
	o.Clean = true
	o.extractedDir = tree
	err = exportRun(o)
	if err == nil {
		t.Fatal("exportRun accepted a locator whose page text is missing")
	}
	if !strings.Contains(err.Error(), "p0076") {
		t.Errorf("got %v, want it to name the missing page", err)
	}

	after, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatalf("the output directory is gone: %v", rerr)
	}
	if len(after) != len(before) {
		t.Errorf("--clean destroyed the site over a pre-detectable fault: %d entries before, "+
			"%d after", len(before), len(after))
	}
}

// budgetDocIDForTest is the one document the committed store covers.
const budgetDocIDForTest = "livermore-budget-fy2026-2027"

// TestEveryShippedDocumentCanResolveItsRecords is the fail-closed arm for a
// field whose absence is legal.
//
// Options.RecordsBase may be empty -- absent is not zero, and a caller
// publishing no records is not a caller publishing a base pointing nowhere --
// so validate cannot demand one. That leaves "the real export forgot to wire
// it" as a silent regression: every page still renders, every chart still
// draws, and only the records links vanish. This is where that is caught, over
// what fisc export actually ships.
func TestEveryShippedDocumentCanResolveItsRecords(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	docs := map[string]bool{}
	for _, e := range built.PageIndex {
		docs[e.DocID] = true
	}
	if len(docs) == 0 {
		t.Fatal("the export published records for no document, so this asserts nothing")
	}
	for doc := range docs {
		if built.RecordsBase[doc] == "" {
			t.Errorf("document %s publishes records and no records base; every link "+
				"citing it would render with nothing to resolve", doc)
		}
	}
}

// TestTheRecordsBaseComposesBackToTheShardPath is what makes the split safe.
//
// shardPath is documented as the ONE place the locator-to-URL rule is spelled,
// and it is now spelled in two halves so a client can hold the first and
// compute the second. This asserts the halves still make the whole for every
// page actually published -- otherwise "the rule is spelled once" would be a
// comment rather than a property.
func TestTheRecordsBaseComposesBackToTheShardPath(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	if len(built.PageIndex) == 0 {
		t.Fatal("the export published no page index, so this asserts nothing")
	}
	for _, e := range built.PageIndex {
		base, ok := built.RecordsBase[e.DocID]
		if !ok {
			t.Errorf("no records base for %s, which page index entry p%d publishes",
				e.DocID, e.Page)
			continue
		}
		if got, want := base+shardFile(e.Page), shardPath(e.DocID, e.Page); got != want {
			t.Errorf("base+file = %q for %s p%d, want %q", got, e.DocID, e.Page, want)
		}
		// And the composed path is a file the site actually writes, which is
		// the claim a reader following a records link depends on.
		if _, ok := built.Files[base+shardFile(e.Page)]; !ok {
			t.Errorf("%s composes to %q, which is not among the files shipped",
				e.DocID, base+shardFile(e.Page))
		}
	}
}
