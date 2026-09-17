package export

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheStructureShipsOnTheAssetChannelAndIsMeasured is two things. It pins
// that buildAll puts the structure document where the site will serve it and
// that the document carries a view for every summing projection and none for
// a series; and under -v it re-measures what fisc-kbuo asks for -- the bytes
// a reader would pay for the headline view and for the first drill, raw and
// gzipped, with and without provenance -- off this build rather than off a
// figure copied from anywhere.
func TestTheStructureShipsOnTheAssetChannelAndIsMeasured(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	raw, ok := built.Files[structurePath]
	if !ok {
		t.Fatalf("buildAll ships no %s; the asset channel carries %d files", structurePath, len(built.Files))
	}
	var doc structure.Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode %s: %v", structurePath, err)
	}
	if doc.SchemaVersion != structure.DocumentSchemaVersion {
		t.Errorf("schema_version %d, want %d", doc.SchemaVersion, structure.DocumentSchemaVersion)
	}
	views := map[string]structure.ViewDecl{}
	for _, v := range doc.Views {
		views[v.Name] = v
		if len(v.Facts) == 0 {
			t.Errorf("view %q admits no fact and was shipped anyway", v.Name)
		}
	}
	for _, want := range []string{export.PrimaryProjection, project.FundFlowsProjection} {
		if _, ok := views[want]; !ok {
			t.Errorf("no view named %q; the document carries %v", want, keysOf(views))
		}
	}
	// A series publishes no total and is not a view. Its scopes name no cut,
	// so had documentViews let one through, Build would have refused the whole
	// document rather than this test finding it here; the assertion is that
	// the series arm exists at all.
	for _, series := range []string{project.TrendsProjection, project.ChangesProjection, project.FundBalancesProjection} {
		if _, ok := views[series]; ok {
			t.Errorf("view %q is a series and publishes no total", series)
		}
	}
	if t.Failed() {
		return
	}

	// THE MEASUREMENT. Whole structure against the two slices a reader would
	// be sent under candidate (b) of fisc-kbuo, each with its provenance and
	// without, beside the document the reader pays for today.
	today := built.Projections[project.FundFlowsProjection]
	t.Logf("today: data/%s.json raw %d gzip %d (best %d)", project.FundFlowsProjection,
		len(today), gzipped(t, today, gzip.DefaultCompression), gzipped(t, today, gzip.BestCompression))
	rows := []struct {
		label string
		doc   structure.Document
	}{{"whole structure", doc}}
	for _, name := range []string{export.PrimaryProjection, project.FundFlowsProjection} {
		sl, err := doc.Slice(name)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, struct {
			label string
			doc   structure.Document
		}{name + " slice", sl})
	}
	for _, r := range rows {
		b, err := json.Marshal(r.doc)
		if err != nil {
			t.Fatal(err)
		}
		bare := withoutProvenance(t, b)
		t.Logf("%-24s %d facts: raw %d gzip %d (best %d); without provenance raw %d gzip %d",
			r.label, len(r.doc.Facts),
			len(b), gzipped(t, b, gzip.DefaultCompression), gzipped(t, b, gzip.BestCompression),
			len(bare), gzipped(t, bare, gzip.DefaultCompression))
	}
}

func keysOf(m map[string]structure.ViewDecl) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func gzipped(t *testing.T, b []byte, level int) int {
	t.Helper()
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Len()
}

// withoutProvenance is the document with each fact's id and locator dropped:
// what candidate (d) of fisc-kbuo would ship if provenance were fetched on
// pin. Re-encoded from maps, so key order is Go's sorted one rather than the
// struct's; the byte count is a measurement of content, not of layout.
func withoutProvenance(t *testing.T, b []byte) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	facts, _ := m["facts"].([]any)
	for _, f := range facts {
		rec, _ := f.(map[string]any)
		for _, k := range []string{"id", "doc_id", "page", "offset", "token"} {
			delete(rec, k)
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
