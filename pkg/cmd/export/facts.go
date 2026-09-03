package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/fact"
)

// FactsDir is where the published fact store goes in the output tree.
//
// TOP-LEVEL, NOT UNDER data/, and the choice is forced rather than stylistic.
// export.assetPath rejects any key beginning with data/ or vendor/, because
// docs/sankey-contract.md promises data/ means "one file per projection" and an
// asset landing there would shadow that promise. facts/ is free, so the store
// ships with no guard change at all.
//
// The tree under it mirrors PageTextDir's — <doc-id>/pages/pNNNN — so a
// citation's two halves come from parallel paths and a reader who knows one
// layout knows the other.
const FactsDir = "facts"

// CSVPath and IndexPath are the two whole-store artifacts.
const (
	factsCSVPath   = FactsDir + "/facts.csv"
	factsIndexPath = FactsDir + "/index.json"
)

// factPage is one shard: the facts of one (doc_id, page), and where both halves
// of that page's provenance live.
type factPage struct {
	DocID string `json:"doc_id"`
	Page  int    `json:"page"`
	Facts int    `json:"facts"`
	Bytes int    `json:"bytes"`
	// SHA256 is over the shard as shipped. It is here so a consumer can check
	// a download it did not watch being written.
	SHA256 string `json:"sha256"`
	// Path is where the records are: site-relative, and computable from
	// (doc_id, page) by shardPath. Published anyway, because an index that
	// makes a consumer re-implement a path rule is an index that will disagree
	// with it.
	//
	// THERE IS NO text_path HERE, and its absence is the considered answer.
	// The obvious companion field would name the page's extracted text, and
	// this packager CANNOT KNOW that path: export.Write chooses between the
	// shipped copy under extracted/ and a remote browse URL, and it makes that
	// choice after this index has been built. An earlier draft published the
	// local form unconditionally, so `fisc export --source-browse-url` shipped
	// an index naming 21 files the site does not write. Publishing a path this
	// side cannot guarantee is a false statement in a file whose whole value is
	// that its statements hold; the HTML rows carry the correct link because
	// they are composed inside Write, where the decision lives.
	Path string `json:"path"`
	// Rules and Years are what is on this page, for a reader choosing which
	// shard to open. Read off the facts, never declared.
	Rules []string `json:"rules"`
	Years []int    `json:"fiscal_years"`
}

// factAssets is everything the fact store contributes to a site.
type factAssets struct {
	// Files is keyed by site-relative path, ready for export.Options.Files.
	Files map[string][]byte
	// Pages is one entry per shard, in the store's own order.
	Pages []factPage
	// Facts and Bytes are the whole store, for the page's prose.
	Facts int
	Bytes int
	// CSVBytes is the size of the transcoded store, which the download link
	// prints so a reader knows what they are about to fetch.
	CSVBytes int
}

// shardPath is the ONE place the locator-to-URL rule is spelled.
//
// A provenance link resolves by COMPUTING this from (doc_id, page) -- one
// fetch, no index lookup, nothing content-addressed. That is what makes it
// survive a rebuild: fact.MakeID hashes rule_id among other things, so a rule
// split moves every id on a page, while (doc_id, page) cannot move at all.
func shardPath(docID string, page int) string {
	return shardBase(docID) + shardFile(page)
}

// shardBase and shardFile are shardPath split at the point a CLIENT has to
// compose it: the base is per-document and goes into the page config once, and
// the file is computed from the page number by whoever holds the locator.
//
// It is the same split internal/export already uses for the OTHER half of a
// citation -- LocalPageTextBase plus pageTextFile -- and it exists so that
// package can publish a records base without learning the rule. It must not:
// buildProvenancePage's comment says the locator-to-URL rule belongs to
// whoever produced the records, and that is here.
func shardBase(docID string) string {
	return fmt.Sprintf("%s/%s/pages/", FactsDir, docID)
}

func shardFile(page int) string {
	return fmt.Sprintf("p%04d.jsonl", page)
}

// buildFactAssets shards the committed store by (doc_id, page), transcodes it
// to CSV, and describes both.
//
// IT RECONCILES IN THE BUILDER RATHER THAN IN A TEST, which is a deliberate
// exception to "fisc export runs no checks". The precedent is buildTrendsPage's
// counts reconciliation and the argument is the same: a published artifact that
// silently disagrees with the file it claims to be is the one failure this
// project exists to refuse, and a test proves it about the store as committed
// today while this proves it about the store as shipped, every run.
//
// THE RECONCILIATION IS ONE EQUALITY, and it is strong because of a property of
// the sort rather than because of care taken here. fact.less opens on DocID
// then Page, so (doc_id, page) is a strict PREFIX of the total order and every
// shard is a contiguous run of facts.jsonl. Concatenating the shards in order
// must therefore reproduce the file byte for byte -- no fact lost, none
// invented, none rewritten, and no re-encoding to argue about.
//
// raw and facts must be the same store: raw as committed, facts as read from
// it. The caller owns that pairing (see readFactStore).
func buildFactAssets(raw []byte, facts []fact.Fact, version string) (factAssets, error) {
	if len(facts) == 0 {
		return factAssets{}, fmt.Errorf("the fact store is empty; nothing to publish")
	}
	// UNSORTED FIRST, because every claim below rests on the order. A store
	// out of order does not shard into contiguous runs at all, and the byte
	// comparison would report a mismatch without saying why.
	if err := fact.CheckSorted(facts); err != nil {
		return factAssets{}, fmt.Errorf(
			"the fact store is not in its total order, so its pages are not contiguous "+
				"runs and cannot be sharded: %w", err)
	}

	assets := factAssets{Files: map[string][]byte{}, Facts: len(facts), Bytes: len(raw)}
	var concat bytes.Buffer
	seen := map[string]bool{}

	for start := 0; start < len(facts); {
		end := start + 1
		for end < len(facts) &&
			facts[end].DocID == facts[start].DocID && facts[end].Page == facts[start].Page {
			end++
		}
		group := facts[start:end]
		docID, page := group[0].DocID, group[0].Page

		// A PAIR RECURRING AFTER A GAP means the run is not contiguous even
		// though the store passed CheckSorted -- possible only if less and this
		// grouping disagree about what they key on. Refused rather than
		// silently overwriting the earlier shard.
		key := docID + "\x1f" + strconv.Itoa(page)
		if seen[key] {
			return factAssets{}, fmt.Errorf(
				"%s p%d appears in two separate runs of the fact store; its facts are not "+
					"contiguous and one shard would overwrite the other", docID, page)
		}
		seen[key] = true

		var shard bytes.Buffer
		if err := fact.Write(&shard, group); err != nil {
			return factAssets{}, fmt.Errorf("encode the shard for %s p%d: %w", docID, page, err)
		}
		b := shard.Bytes()
		concat.Write(b)

		sum := sha256.Sum256(b)
		rel := shardPath(docID, page)
		assets.Files[rel] = b
		assets.Pages = append(assets.Pages, factPage{
			DocID: docID, Page: page,
			Facts: len(group), Bytes: len(b), SHA256: hex.EncodeToString(sum[:]),
			Path: rel, Rules: distinctRules(group), Years: distinctYears(group),
		})
		start = end
	}

	// THE EQUALITY. Reported with the offset and the page it falls in, because
	// "the shards do not reproduce the store" leaves a reader to go and diff
	// the whole fact store by hand.
	if got := concat.Bytes(); !bytes.Equal(got, raw) {
		return factAssets{}, fmt.Errorf(
			"the shards do not reproduce the fact store: %s", firstDifference(got, raw, assets.Pages))
	}

	csvBytes, err := factsCSV(raw)
	if err != nil {
		return factAssets{}, err
	}
	assets.CSVBytes = len(csvBytes)
	assets.Files[factsCSVPath] = csvBytes

	index, err := factsIndex(assets, version)
	if err != nil {
		return factAssets{}, err
	}
	assets.Files[factsIndexPath] = index
	return assets, nil
}

// firstDifference names where two encodings of the store diverge, in the terms
// a reader can act on: a byte offset, and the page that offset lands in.
func firstDifference(got, want []byte, pages []factPage) string {
	n := min(len(got), len(want))
	at := n
	for i := range n {
		if got[i] != want[i] {
			at = i
			break
		}
	}
	where := "past the end of the store"
	run := 0
	for _, p := range pages {
		if at < run+p.Bytes {
			where = fmt.Sprintf("%s p%d", p.DocID, p.Page)
			break
		}
		run += p.Bytes
	}
	return fmt.Sprintf("they differ at byte %d, in the shard for %s (shards are %d bytes, "+
		"the store is %d)", at, where, len(got), len(want))
}

// distinctRules and distinctYears say what is on a page, read off its facts.
func distinctRules(group []fact.Fact) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, f := range group {
		if !seen[f.RuleID] {
			seen[f.RuleID] = true
			out = append(out, f.RuleID)
		}
	}
	sort.Strings(out)
	return out
}

func distinctYears(group []fact.Fact) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, f := range group {
		if !seen[f.FiscalYear] {
			seen[f.FiscalYear] = true
			out = append(out, f.FiscalYear)
		}
	}
	sort.Ints(out)
	return out
}

// factsIndexDoc is the shape of facts/index.json.
type factsIndexDoc struct {
	SchemaVersion int    `json:"schema_version"`
	GeneratedBy   string `json:"generated_by"`
	// Store is the whole store, so a consumer can size a download before
	// starting one.
	Store struct {
		Documents int `json:"documents"`
		Pages     int `json:"pages"`
		Facts     int `json:"facts"`
		Bytes     int `json:"bytes"`
		CSVBytes  int `json:"csv_bytes"`
	} `json:"store"`
	CSVPath string     `json:"csv_path"`
	Pages   []factPage `json:"pages"`
}

// factsIndex enumerates the shards.
//
// IT IS NOT ON THE RESOLUTION PATH, and that has to be said because an index
// invites the assumption. A locator resolves by computing shardPath from
// (doc_id, page): one fetch, no lookup, nothing to keep in sync. This file
// exists so the provenance page can be built, so a consumer can enumerate the
// store without a directory listing a static host may not serve, and so a
// download can be checked against a hash. Nothing breaks if it is absent.
func factsIndex(a factAssets, version string) ([]byte, error) {
	docs := map[string]bool{}
	for _, p := range a.Pages {
		docs[p.DocID] = true
	}
	doc := factsIndexDoc{SchemaVersion: 1, GeneratedBy: version, CSVPath: factsCSVPath, Pages: a.Pages}
	doc.Store.Documents = len(docs)
	doc.Store.Pages = len(a.Pages)
	doc.Store.Facts = a.Facts
	doc.Store.Bytes = a.Bytes
	doc.Store.CSVBytes = a.CSVBytes

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, fmt.Errorf("encode the fact index: %w", err)
	}
	return buf.Bytes(), nil
}

// factCSVHeader is the CSV's column list: every fact.Fact JSON tag, in
// declaration order.
//
// TAKEN BY REFLECTION FROM THE STRUCT, not written out here and not read off a
// marshalled value. A literal list drifts the first time a field lands. A
// marshalled ZERO Fact would be worse than either: it looks authoritative and
// would silently shrink the header the day any field gained an omitempty, so a
// test comparing header against header would compare wrong with wrong and stay
// green. fact.Write's doc comment states that no field is omitempty and every
// line carries the same keys in the same order; this reads that guarantee off
// the type that makes it.
func factCSVHeader() []string {
	t := reflect.TypeOf(fact.Fact{})
	out := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		tag, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		out = append(out, tag)
	}
	return out
}

// factsCSV transcodes the store's JSONL into CSV, value for value.
//
// TRANSCODED RATHER THAN RE-SERIALISED FROM []fact.Fact, and that is what makes
// "no float" structural instead of promised. json.Decoder.Token() with
// UseNumber yields each number as the LITERAL TEXT the encoder wrote, so
// amount_cents lands in the CSV as the same digits that are in facts.jsonl and
// no float64 is constructed anywhere on the money path. AGENTS.md's provenance
// invariants forbid float amounts; this makes it impossible rather than avoided.
//
// AND IT IS Token() RATHER THAN Decode(&map[string]any) FOR THE KEY ORDER.
// Decoding an object into a Go map loses it, so a header "in declaration order"
// could not be recovered from the decoded value at all -- the column order
// would be whatever the map iterated. Streaming tokens keeps the file's own
// order, which is then checked against the struct's.
//
// The CSV carries no amount_dollars. It would be a float, which is forbidden,
// or a second decimal spelling of one integer, which is a second answer to what
// the amount is. The column says cents in its name.
func factsCSV(raw []byte) ([]byte, error) {
	header := factCSVHeader()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write the CSV header: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for line := 1; ; line++ {
		row, err := csvRow(dec, header)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("transcode %s line %d: %w", factsPath, line, err)
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write CSV line %d: %w", line, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("write the CSV: %w", err)
	}
	return buf.Bytes(), nil
}

// csvRow reads one JSON object off the stream and returns its values in header
// order, requiring the object to present exactly those keys in exactly that
// order.
//
// THE KEY SEQUENCE IS CHECKED PER LINE, not assumed from the first. csv.Writer
// enforces no field count -- only csv.Reader does -- so a line missing a key
// would otherwise ship a short row in silence, which is the shape of defect
// this whole file exists to refuse. Fail closed, naming the key and the
// position.
func csvRow(dec *json.Decoder, header []string) ([]string, error) {
	tok, err := dec.Token()
	if err != nil {
		// The ONLY place io.EOF means "done". Everywhere below it means the
		// last object was cut off mid-line, and returning it bare would end
		// the transcode cleanly and ship a CSV silently missing its tail --
		// which is the failure this file exists to refuse, arriving through
		// the loop that reads the file rather than the one that writes it.
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("expected a JSON object, got %v", tok)
	}
	row := make([]string, 0, len(header))
	for i := 0; ; i++ {
		tok, err = dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading key %d: %w", i, unexpectedEnd(err))
		}
		if d, ok := tok.(json.Delim); ok && d == '}' {
			if i != len(header) {
				return nil, fmt.Errorf("carries %d keys, want the %d of fact.Fact", i, len(header))
			}
			return row, nil
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected a key, got %v", tok)
		}
		if i >= len(header) {
			return nil, fmt.Errorf("carries key %q past the %d of fact.Fact", key, len(header))
		}
		if key != header[i] {
			return nil, fmt.Errorf("key %d is %q, want %q; the CSV's columns are fact.Fact's "+
				"declaration order and this line does not follow it", i, key, header[i])
		}
		val, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("reading the value of %q: %w", key, unexpectedEnd(err))
		}
		cell, err := csvCell(val)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		row = append(row, cell)
	}
}

// unexpectedEnd renames a mid-object EOF, so it can never be mistaken for the
// end of the store by a caller that stops on io.EOF.
func unexpectedEnd(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

// csvCell renders one JSON scalar. A number keeps its source literal; there is
// no numeric type here at all.
func csvCell(v any) (string, error) {
	switch t := v.(type) {
	case json.Number:
		return t.String(), nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		// The store publishes no nulls (fact.Fact has no omitempty and no
		// pointers), so one appearing means the file is not the store.
		return "", fmt.Errorf("value is null, which the fact store never publishes")
	default:
		return "", fmt.Errorf("value %v is not a scalar; a fact has no nested values", v)
	}
}

// pageIndex describes each shard in the terms the provenance view publishes.
//
// THE NOTE IS COMPOSED HERE, in the composition root, because it is a sentence
// about a FACT STORE and internal/export does not know what a fact is. It says
// what a reader would want before deciding to open a 75 KB file: which rules
// read this page, and which years they read.
func (a factAssets) pageIndex() []export.PageIndexEntry {
	out := make([]export.PageIndexEntry, 0, len(a.Pages))
	for _, p := range a.Pages {
		out = append(out, export.PageIndexEntry{
			Citation: export.Citation{DocID: p.DocID, Page: p.Page},
			Records:  p.Facts,
			Data:     p.Path,
			Bytes:    p.Bytes,
			Note:     noteFor(p),
		})
	}
	return out
}

// recordsBase says where each document's shards live, in the form a client
// appends pNNNN.jsonl to.
//
// It is the SAME rule pageIndex's Data uses, split rather than restated --
// shardBase is what shardPath calls -- so the base the page config publishes
// and the path the file was written at cannot drift apart. internal/export
// asserts that relationship rather than trusting it.
//
// Unlike page_text_base there is no local/remote fork: shards are always
// written into the output tree, so this is always site-relative.
func (a factAssets) recordsBase() map[string]string {
	out := make(map[string]string, len(a.Pages))
	for _, p := range a.Pages {
		out[p.DocID] = shardBase(p.DocID)
	}
	return out
}

// noteFor is one page's sentence: what was read off it, and for which years.
func noteFor(p factPage) string {
	years := make([]string, 0, len(p.Years))
	for _, y := range p.Years {
		years = append(years, strconv.Itoa(y))
	}
	rules := strings.Join(p.Rules, ", ")
	if len(p.Rules) > 4 {
		rules = strings.Join(p.Rules[:4], ", ") +
			fmt.Sprintf(" and %d more", len(p.Rules)-4)
	}
	return fmt.Sprintf("%s \u2014 fiscal years %s", rules, strings.Join(years, ", "))
}

// downloads are the whole-store artifacts the page offers.
func (a factAssets) downloads() []export.Download {
	return []export.Download{
		{
			Path:  factsCSVPath,
			Label: "Every figure as CSV",
			Note: "One row per figure, the columns of the record store verbatim. " +
				"Amounts are integer cents.",
			Bytes: a.CSVBytes,
		},
		{
			Path:  factsIndexPath,
			Label: "The index of pages, as JSON",
			Note: "Every page with its record count, size and SHA-256. Not needed to " +
				"resolve a citation \u2014 that path is computed from the locator.",
			Bytes: len(a.Files[factsIndexPath]),
		},
	}
}
