package export

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/export"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/structure"
)

// TestTheStructureIsPartitionedByYearAndIsMeasured is two things. It pins
// that buildAll produces one structure per fiscal year and no monolith beside
// them, and that the site publishes none of them; that each part declares its year and
// carries only that year's facts, with the views re-indexed into them and a
// view admitting none of that year omitted rather than shipped empty; that
// the parts together carry every fact of the whole document exactly once;
// and that the whole carries a view for every summing projection and none
// for a series. Under -v it re-measures what fisc-kbuo decided to ship --
// each part, raw and gzipped, beside the whole and its scaffolding -- off
// this build rather than off a figure copied from anywhere.
//
// THE VACUITY GUARDS ARE THE POINT, as TestTheRungArtifactIsWhatGoComputes
// says of its own. A partition of one part is inert, and the omission arm
// never fires unless some part omits a view while another carries every
// one; each guard names the shape it refuses and where the committed corpus
// supplies the opposite, and logs which years did.
func TestTheStructureIsPartitionedByYearAndIsMeasured(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	built, err := buildAll(root)
	if err != nil {
		t.Fatalf("buildAll: %v", err)
	}
	_, facts, err := readFactStore(root)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := loadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	whole, err := structureOf(reg, facts)
	if err != nil {
		t.Fatalf("structureOf: %v", err)
	}
	if whole.FiscalYear != 0 {
		t.Errorf("the unpartitioned document declares fiscal year %d, and nothing ships it", whole.FiscalYear)
	}

	// THE LATTICE IS BUILT AND IS NOT PUBLISHED. It reached the reader's
	// download through the asset channel and nothing in site/, tools/ or
	// docs/ ever named it; result.Structure is where it lives now, so this
	// arm asks the asset channel to carry NONE of it rather than asking it to
	// carry the parts and not the monolith.
	for rel := range built.Files {
		if strings.HasPrefix(rel, "structure") {
			t.Errorf("the site publishes %s, and nothing a reader has can read it", rel)
		}
	}
	if _, ok := built.Structure["structure.json"]; ok {
		t.Error("buildAll builds structure.json, the unpartitioned document nothing reads")
	}
	years := []int{}
	for _, f := range whole.Facts {
		if !slices.Contains(years, f.FiscalYear) {
			years = append(years, f.FiscalYear)
		}
	}
	slices.Sort(years)
	for rel := range built.Structure {
		if !slices.ContainsFunc(years, func(y int) bool { return structurePath(y) == rel }) {
			t.Errorf("%s was built and is no fiscal year's structure; the whole spans %v", rel, years)
		}
	}

	views := map[string]structure.ViewDecl{}
	for _, v := range whole.Views {
		views[v.Name] = v
		if len(v.Facts) == 0 {
			t.Errorf("view %q admits no fact and was built anyway", v.Name)
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

	// EACH PART, against the whole restricted to its year.
	carried := map[string]int{}
	var omitting, carryingAll []int
	parts := map[int]structure.Document{}
	for _, year := range years {
		path := structurePath(year)
		raw, ok := built.Structure[path]
		if !ok {
			t.Errorf("buildAll builds no %s; the lattice carries %d part(s)", path, len(built.Structure))
			continue
		}
		var part structure.Document
		if err = json.Unmarshal(raw, &part); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		parts[year] = part
		if part.SchemaVersion != structure.DocumentSchemaVersion {
			t.Errorf("%s: schema_version %d, want %d", path, part.SchemaVersion, structure.DocumentSchemaVersion)
		}
		if part.FiscalYear == 0 || part.FiscalYear != year {
			t.Errorf("%s declares fiscal_year %d", path, part.FiscalYear)
		}
		for i, f := range part.Facts {
			carried[f.ID]++
			if f.FiscalYear != part.FiscalYear {
				t.Errorf("%s: fact %d (%s) is of %d", path, i, f.ID, f.FiscalYear)
			}
		}
		for _, name := range []string{"levels", "cuts", "identities"} {
			var diff string
			switch name {
			case "levels":
				diff = cmp.Diff(whole.Levels, part.Levels)
			case "cuts":
				diff = cmp.Diff(whole.Cuts, part.Cuts)
			case "identities":
				diff = cmp.Diff(whole.Identities, part.Identities)
			}
			if diff != "" {
				t.Errorf("%s: %s is not the whole's (-whole +part):\n%s", path, name, diff)
			}
		}
		byName := map[string]structure.ViewDecl{}
		for _, v := range part.Views {
			if _, known := views[v.Name]; !known {
				t.Errorf("%s ships view %q, which the whole does not carry", path, v.Name)
			}
			if len(v.Facts) == 0 {
				t.Errorf("%s: view %q admits no fact of %d and was shipped anyway", path, v.Name, year)
			}
			byName[v.Name] = v
		}
		for _, v := range whole.Views {
			want := []string{}
			for _, at := range v.Facts {
				if whole.Facts[at].FiscalYear == year {
					want = append(want, whole.Facts[at].ID)
				}
			}
			pv, present := byName[v.Name]
			switch {
			case len(want) == 0 && present:
				t.Errorf("%s ships view %q, which admits no fact of %d", path, v.Name, year)
			case len(want) == 0:
			case !present:
				t.Errorf("%s omits view %q, which admits %d facts of %d", path, v.Name, len(want), year)
			default:
				got := []string{}
				for _, at := range pv.Facts {
					if at < 0 || at >= len(part.Facts) {
						t.Errorf("%s: view %q names fact %d, outside 0..%d", path, v.Name, at, len(part.Facts)-1)
						continue
					}
					got = append(got, part.Facts[at].ID)
				}
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("%s: view %q (-whole restricted to %d +part):\n%s", path, v.Name, year, diff)
				}
			}
		}
		if len(part.Views) < len(whole.Views) {
			omitting = append(omitting, year)
		} else {
			carryingAll = append(carryingAll, year)
		}
	}
	for _, f := range whole.Facts {
		if carried[f.ID] != 1 {
			t.Errorf("%s is carried by %d parts", f.ID, carried[f.ID])
		}
	}
	for id, n := range carried {
		if n > 1 {
			t.Errorf("%s is carried by %d parts", id, n)
		}
	}
	// THE PARTITION HAS TO BE ONE. A store of one fiscal year would ship one
	// part that is the whole under another name, and nothing above would
	// have compared a re-indexed view to anything.
	if len(parts) < 2 {
		t.Errorf("the whole spans %v, one fiscal year, so the partition is inert", years)
	}
	// THE OMISSION ARM HAS TO FIRE AND HAS TO NOT FIRE. The corpus supplies
	// both today: the spine's and transfers-by-fund's schedules print for the
	// two budget years only, while fund-flows and the department views span
	// every published year.
	if len(omitting) == 0 || len(carryingAll) == 0 {
		t.Errorf("parts omitting a view: %v; parts carrying every view: %v; both are needed for the omission to be witnessed", omitting, carryingAll)
	}
	if t.Failed() {
		return
	}
	t.Logf("parts omitting a view: %v; parts carrying every view: %v", omitting, carryingAll)

	// THE MEASUREMENT. What candidate (a) of fisc-kbuo ships, now per year --
	// each part with its provenance, and the scaffolding every part repeats
	// -- beside the whole and the one document the reader pays for today.
	// The per-view slices this block once measured went with Document.Slice
	// when (a) was decided.
	today := built.Projections[project.FundFlowsProjection]
	t.Logf("today: data/%s.json raw %d gzip %d (best %d)", project.FundFlowsProjection,
		len(today), gzipped(t, today, gzip.DefaultCompression), gzipped(t, today, gzip.BestCompression))
	b, err := json.Marshal(whole)
	if err != nil {
		t.Fatal(err)
	}
	bare := withoutProvenance(t, b)
	t.Logf("whole structure, %d facts: raw %d gzip %d (best %d); without provenance raw %d gzip %d",
		len(whole.Facts),
		len(b), gzipped(t, b, gzip.DefaultCompression), gzipped(t, b, gzip.BestCompression),
		len(bare), gzipped(t, bare, gzip.DefaultCompression))
	scaffold, err := json.Marshal(structure.Document{
		SchemaVersion: whole.SchemaVersion, Levels: whole.Levels, Cuts: whole.Cuts, Identities: whole.Identities,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scaffolding alone: raw %d gzip %d", len(scaffold), gzipped(t, scaffold, gzip.DefaultCompression))
	for _, year := range years {
		raw := built.Structure[structurePath(year)]
		t.Logf("%s, %d facts, %d views: raw %d gzip %d (best %d)", structurePath(year),
			len(parts[year].Facts), len(parts[year].Views),
			len(raw), gzipped(t, raw, gzip.DefaultCompression), gzipped(t, raw, gzip.BestCompression))
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
