package export

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/repo"
	"github.com/jcrussell/livermore-budget/pkg/iostreams"
)

// committedStore reads the real facts/facts.jsonl, both ways.
func committedStore(t *testing.T) ([]byte, []fact.Fact) {
	t.Helper()
	root := repoRootForTest(t)
	raw, facts, err := readFactStore(root)
	if err != nil {
		t.Fatalf("read the fact store: %v", err)
	}
	return raw, facts
}

// TestFactShardsReconstituteTheCommittedStore: the shards concatenated in order
// reproduce the committed facts.jsonl byte for byte -- none lost, invented or
// rewritten -- because (doc_id, page) prefixes fact.less's order.
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

// TestBuildFactAssetsRefusesAStoreItCannotReproduce mutates the input: one line
// with its keys reordered, as a hand edit would, re-encodes to something else.
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

// TestBuildFactAssetsRefusesAnUnsortedStore: a store out of total order is
// refused as unsorted, before it could surface as a byte mismatch.
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

// TestTheCSVHeaderIsEveryFactFieldInOrder pins the column list, in order, to
// the keys of a marshalled fact.Fact.
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

// TestTheCSVCarriesAmountCentsVerbatim: a CSV amount cell is the JSONL's
// characters, never a float's formatting (6999000000, not 6.999e+09).
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

// TestTheCSVQuotesTokensThatCarryACommaAndRoundTrips: grouped-number tokens
// carry commas, so the CSV must quote them and read back to the same columns.
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

// TestTheCSVSpellsAnAbsentFundAsAnEmptyCellAndRefusesNullElsewhere: a null
// fund is an empty CSV cell, and a null in any other column is refused. Over
// the committed store, where both an absent and a present fund must occur.
func TestTheCSVSpellsAnAbsentFundAsAnEmptyCellAndRefusesNullElsewhere(t *testing.T) {
	raw, facts := committedStore(t)
	out, err := factsCSV(raw)
	if err != nil {
		t.Fatalf("factsCSV: %v", err)
	}
	rows, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatalf("read back the CSV: %v", err)
	}
	col := -1
	for i, name := range rows[0] {
		if name == "fund" {
			col = i
		}
	}
	if col < 0 {
		t.Fatal("the CSV has no fund column")
	}
	absent, named := 0, 0
	for i, f := range facts {
		got := rows[i+1][col]
		if f.Fund == nil {
			absent++
			if got != "" {
				t.Fatalf("line %d names no fund and the CSV cell is %q, want empty", i+1, got)
			}
			continue
		}
		named++
		if want := strconv.Itoa(*f.Fund); got != want {
			t.Fatalf("line %d names fund %s and the CSV cell is %q", i+1, want, got)
		}
	}
	if absent == 0 || named == 0 {
		t.Fatalf("%d facts name no fund and %d name one; both spellings must occur for "+
			"this test to see either", absent, named)
	}

	// A null anywhere but a nullable column is refused, and the refusal names
	// the column it landed in.
	lines := bytes.SplitN(raw, []byte("\n"), 2)
	var first map[string]any
	if err := json.Unmarshal(lines[0], &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var b bytes.Buffer
	b.WriteByte('{')
	for _, name := range factCSVHeader() {
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		v := []byte("null")
		if name != "amount_cents" {
			var err error
			if v, err = json.Marshal(first[name]); err != nil {
				t.Fatalf("encode %s: %v", name, err)
			}
		}
		fmt.Fprintf(&b, "%q:%s", name, v)
	}
	b.WriteByte('}')
	nulled := append(append(b.Bytes(), '\n'), lines[1]...)
	if _, err := factsCSV(nulled); err == nil {
		t.Fatal("factsCSV accepted a null amount_cents")
	} else if !strings.Contains(err.Error(), "amount_cents") || !strings.Contains(err.Error(), "null") {
		t.Errorf("error %q does not name the null and the column it is in", err)
	}
}

// TestTheCSVRefusesALineMissingAKey: csv.Writer enforces no field count, so a
// short line must be refused rather than shift every later column.
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

// TestTheCSVRefusesATruncatedStore cuts the final record at every position:
// json.Decoder.Token returns bare io.EOF at most cut points, so a loop that
// stops on io.EOF would ship a CSV missing its tail.
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

	// From last+1: a cut at the record boundary is a well-formed shorter store,
	// which buildFactAssets' byte equality refuses rather than factsCSV.
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

// TestTheFactIndexEnumeratesEveryShardAndIsNotOnTheResolutionPath: every path
// the index publishes equals shardPath for the same locator.
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

// TestEveryShardedPageHasItsExtractedText: every page the store shards has its
// extracted text committed. The per-page arm cannot go red while every document
// is extracted whole; the empty-store guard is the half that can fail.
func TestEveryShardedPageHasItsExtractedText(t *testing.T) {
	root := repoRootForTest(t)
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
		rel := filepath.Join(root, filepath.FromSlash(repo.ExtractedDir),
			p.DocID, "pages", fmt.Sprintf("p%04d.txt", p.Page))
		if _, err := os.Stat(rel); err != nil {
			t.Errorf("%s p%d carries %d facts and its extracted text is not committed: %v",
				p.DocID, p.Page, p.Facts, err)
		}
	}
}

// TestTheSiteLinksEveryShardItShips: every key under facts/ is a row of the
// page index or an offered download, so no shipped byte is unreachable.
func TestTheSiteLinksEveryShardItShips(t *testing.T) {
	root := repoRootForTest(t)
	built, err := cachedBuildAll(root)
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
		if !strings.HasPrefix(rel, factsDir+"/") {
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
// the real view set, which TestViewsOpensOnTheSpineAndGivesYearsToItsCharts
// deliberately does not (it passes a Result with no page index).
func TestTheProvenanceViewIsPublishedWhenThereIsAStore(t *testing.T) {
	root := repoRootForTest(t)
	built, err := cachedBuildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}

	vs := mustViews(t, built)
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
	for _, v := range mustViews(t, result{Projections: built.Projections}) {
		if v.Template == "provenance.html.tmpl" {
			t.Error("a provenance view was published with no page index behind it")
		}
	}
}

// TestTheIndexNamesOnlyFilesTheSiteWrites: every path facts/index.json names is
// a file the site wrote, under both page-text modes.
func TestTheIndexNamesOnlyFilesTheSiteWrites(t *testing.T) {
	root := repoRootForTest(t)
	for _, browse := range []string{"", "https://example.invalid/blob/main"} {
		name := "shipping the page text"
		if browse != "" {
			name = "citing a remote copy"
		}
		t.Run(name, func(t *testing.T) {
			// Shipping the page text is the shared export; citing a remote
			// copy is an export of its own, over the same cached build.
			dir := exportedSite(t)
			if browse != "" {
				io, _, _, _ := iostreams.Test()
				dir = filepath.Join(t.TempDir(), "dist")
				if err := exportRun(context.Background(), &Options{
					IO:              io,
					RepoRoot:        func() (string, error) { return root, nil },
					Build:           cachedBuildAll,
					OutputDir:       dir,
					SourceBrowseURL: browse,
				}); err != nil {
					t.Fatalf("exportRun: %v", err)
				}
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

// TestCleanDoesNotDestroyASiteOverAFaultItCouldHaveSeen: a store covering a
// page the extraction lacks (p76) is refused in Prepare, before --clean removes
// anything.
func TestCleanDoesNotDestroyASiteOverAFaultItCouldHaveSeen(t *testing.T) {
	root := repoRootForTest(t)
	dir := filepath.Join(t.TempDir(), "dist")
	opts := func() *Options {
		io, _, _, _ := iostreams.Test()
		return &Options{
			IO:        io,
			RepoRoot:  func() (string, error) { return root, nil },
			OutputDir: dir,
		}
	}
	if rerr := exportRun(context.Background(), opts()); rerr != nil {
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
	src := filepath.Join(root, filepath.FromSlash(repo.ExtractedDir))
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
	err = exportRun(context.Background(), o)
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

// TestEveryShippedDocumentCanResolveItsRecords: Options.RecordsBase may be
// empty, so this is where the real export failing to wire it is caught.
func TestEveryShippedDocumentCanResolveItsRecords(t *testing.T) {
	root := repoRootForTest(t)
	built, err := cachedBuildAll(root)
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

// TestTheRecordsBaseComposesBackToTheShardPath: the records base and the page
// suffix compose to shardPath for every published page.
func TestTheRecordsBaseComposesBackToTheShardPath(t *testing.T) {
	root := repoRootForTest(t)
	built, err := cachedBuildAll(root)
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
