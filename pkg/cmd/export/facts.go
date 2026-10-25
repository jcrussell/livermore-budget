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

// factsDir is top-level, not under data/: export.assetPath rejects data/ and
// vendor/ keys, since docs/sankey-contract.md reserves data/ for projections.
// Its tree mirrors PageTextDir's, <doc-id>/pages/pNNNN.
const factsDir = "facts"

// factsCSVPath and factsIndexPath are the two whole-store artifacts.
const (
	factsCSVPath   = factsDir + "/facts.csv"
	factsIndexPath = factsDir + "/index.json"
)

// factPage is one shard: the facts of one (doc_id, page), and where both halves
// of that page's provenance live.
type factPage struct {
	DocID string `json:"doc_id"`
	Page  int    `json:"page"`
	Facts int    `json:"facts"`
	Bytes int    `json:"bytes"`
	// SHA256 is over the shard as shipped.
	SHA256 string `json:"sha256"`
	// Path is site-relative and equals shardPath(doc_id, page). There is no
	// text_path: export.Write picks local or remote page text after this index
	// is built, so this side cannot know it.
	Path string `json:"path"`
	// Rules and Years are read off the facts, never declared.
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
	// CSVBytes is printed by the download link.
	CSVBytes int
}

// shardPath is the one place the locator-to-URL rule is spelled. A link
// computes it from (doc_id, page), which survives a rebuild where fact ids
// (hashing rule_id) do not.
func shardPath(docID string, page int) string {
	return shardBase(docID) + shardFile(page)
}

// shardBase and shardFile are shardPath split where a client composes it: the
// base goes into the page config once, the file is computed per locator. The
// rule stays here, with whoever produced the records.
func shardBase(docID string) string {
	return fmt.Sprintf("%s/%s/pages/", factsDir, docID)
}

func shardFile(page int) string { return export.RecordsFile(page) }

// buildFactAssets shards the committed store by (doc_id, page), transcodes it
// to CSV, and describes both. It reconciles every run, as
// reconcileSeriesCounts does: fact.less opens on DocID then Page, so each shard
// is a contiguous run and the shards concatenated must equal raw byte for byte.
// raw and facts must be the same store (see readFactStore).
func buildFactAssets(raw []byte, facts []fact.Fact, version string) (factAssets, error) {
	if len(facts) == 0 {
		return factAssets{}, fmt.Errorf("the fact store is empty; nothing to publish")
	}
	// Sorted first: every claim below rests on the order.
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

		// A pair recurring after a gap means less and this grouping disagree
		// on their key; refused rather than overwriting the earlier shard.
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

	// The equality, reported with the offset and page it falls in.
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

// distinctYears is every fiscal year a page carries a figure for. A band
// names every year it spans, so a page printing 2031-2035 as one figure lists
// 2031 through 2035 rather than 2031 alone.
func distinctYears(group []fact.Fact) []int {
	seen := map[int]bool{}
	out := []int{}
	for i := range group {
		f := &group[i]
		last := f.FiscalYear
		if b := fact.BandOf(f); b != 0 {
			last = b
		}
		for y := f.FiscalYear; y <= last; y++ {
			if !seen[y] {
				seen[y] = true
				out = append(out, y)
			}
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

// factsIndex enumerates the shards. It is not on the resolution path, which
// computes shardPath; it serves the provenance page, enumeration without a
// directory listing, and hash checks.
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

// factCSVHeader is every fact.Fact JSON tag in declaration order, read by
// reflection: a literal list drifts, and a marshalled zero Fact would shrink
// silently the day a field gained omitempty.
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

// factCSVNullable is the set of columns the store may publish null in: the
// pointer fields of fact.Fact, read off the type so a new pointer field cannot
// go unlearned.
func factCSVNullable() map[string]bool {
	t := reflect.TypeOf(fact.Fact{})
	out := map[string]bool{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if tag == "" || tag == "-" || f.Type.Kind() != reflect.Pointer {
			continue
		}
		out[tag] = true
	}
	return out
}

// factsCSV transcodes the store's JSONL into CSV, value for value. Token() with
// UseNumber keeps each number's literal text, so no float64 exists on the money
// path, and streaming keeps the file's key order where a map would lose it.
// There is no amount_dollars column: it would be a float or a second spelling.
func factsCSV(raw []byte) ([]byte, error) {
	header := factCSVHeader()
	nullable := factCSVNullable()
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, fmt.Errorf("write the CSV header: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	for line := 1; ; line++ {
		row, err := csvRow(dec, header, nullable)
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

// csvRow reads one JSON object and returns its values in header order,
// checking the key sequence per line: csv.Writer enforces no field count, so a
// missing key would ship a short row silently.
func csvRow(dec *json.Decoder, header []string, nullable map[string]bool) ([]string, error) {
	tok, err := dec.Token()
	if err != nil {
		// The only place io.EOF means done; below, a bare EOF would end the
		// transcode cleanly and ship a CSV missing its tail.
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
		cell, err := csvCell(val, nullable[key])
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

// csvCell renders one JSON scalar; a number keeps its source literal. A null is
// an empty cell only in a nullable column (fund: null is no fund, 0 is a fund).
func csvCell(v any, nullable bool) (string, error) {
	switch t := v.(type) {
	case json.Number:
		return t.String(), nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		if nullable {
			return "", nil
		}
		return "", fmt.Errorf("value is null, which the fact store publishes in no column but %s",
			describeNullable())
	default:
		return "", fmt.Errorf("value %v is not a scalar; a fact has no nested values", v)
	}
}

// describeNullable lists the nullable columns for a refusal.
func describeNullable() string {
	var names []string
	for name := range factCSVNullable() {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// pageIndex describes each shard for the provenance view. The note is composed
// here because internal/export does not know what a fact is.
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

// recordsBase says where each document's shards live, as a base a client
// appends pNNNN.jsonl to. It calls shardBase, so it cannot drift from the path
// the shard was written at; shards are always local, so it is site-relative.
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
