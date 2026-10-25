package mapping

import (
	"slices"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// p76PublishedScope is the scope the published p76 rules carry, decided by
// fisc-aes: p76 and pp.66-67 are the same money, so a rule at the spine's scope
// would double the city's transfers with every check still green.
const p76PublishedScope = "transfers-by-fund"

// p76Page is Budget Book p76, "SUMMARY OF TRANSFERS".
const p76Page = 76

// p76StatedFY2026 is the FY2025-26 figure of the grand total p76 prints for
// itself, the bare "$" row beneath its last section.
const p76StatedFY2026 amount.Cents = 2152599700

// loadPublishedP76 is the published rules cut to p76's scope.
func loadPublishedP76(t *testing.T) *File {
	t.Helper()
	f, err := Load(publishedSpine)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f.Rules = slices.DeleteFunc(f.Rules, func(r Rule) bool {
		return r.Scope != p76PublishedScope
	})
	if len(f.Rules) == 0 {
		t.Fatalf("%s has no %s rule", publishedSpine, p76PublishedScope)
	}
	return f
}

// TestP76ReadsUnderTheColumnGuardWithAHeaderlessColumn is fisc-wfi's evidence,
// and it is on the page rather than on a fixture of my own construction.
//
// p76 is the first part in the corpus with a column the page prints no header
// over, and until this it had no column_headers at all -- so the guard had
// never run on this page in either direction. Three things are asserted, and
// the third is the one that makes the first two mean anything.
func TestP76ReadsUnderTheColumnGuardWithAHeaderlessColumn(t *testing.T) {
	load := loadPublishedP76
	readAll := func(t *testing.T, f *File) error {
		t.Helper()
		r, err := NewResolver(budgetDoc(t, p76Page), f)
		if err != nil {
			return err
		}
		for i := range f.Rules {
			rule := &f.Rules[i]
			if _, _, err := r.Values(rule, &rule.Parts[0]); err != nil {
				return err
			}
		}
		return nil
	}

	// (1) The guard is DECLARED. Every part carries five entries for five
	// columns, the last of them null.
	f := load(t)
	for i := range f.Rules {
		p := &f.Rules[i].Parts[0]
		if len(p.ColumnHeaders) != len(p.Columns) {
			t.Fatalf("rule %s: %d headers for %d columns", f.Rules[i].ID,
				len(p.ColumnHeaders), len(p.Columns))
		}
		last := p.ColumnHeaders[len(p.ColumnHeaders)-1]
		if !last.Unheaded {
			t.Errorf("rule %s: last header is %s, want null -- the page prints "+
				"nothing over the footnote-marker column", f.Rules[i].ID, last)
		}
		if !p.Columns[len(p.Columns)-1].Skip {
			t.Errorf("rule %s: the headerless column is not skipped", f.Rules[i].ID)
		}
	}

	// (2) And the whole page still reads. All 22 rows place their four figures
	// in the four bands the four printed headers describe; the marker, which
	// has no band, is carried by the column count and the same-line check.
	if err := readAll(t, f); err != nil {
		t.Fatalf("p76 does not read under its own column guard: %v", err)
	}

	// (3) EXACTLY ONE COLUMN IS UNBANDED, and it is the skipped one. This is
	// the claim the null entry is really making, and a passing read alone does
	// not distinguish it from a guard that quietly checks nothing: a band map
	// of all -1 would also let p76 through.
	r, err := NewResolver(budgetDoc(t, p76Page), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	g, err := r.guard(&f.Rules[0], &f.Rules[0].Parts[0])
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if g == nil {
		t.Fatal("no guard was built for a part declaring column_headers")
	}
	if want := []int{0, 1, 2, 3, -1}; !slices.Equal(g.band, want) {
		t.Errorf("band map = %v, want %v: the four printed headers give four bands "+
			"and only the footnote column is unplaceable", g.band, want)
	}

	// (4) THE HEADER LIST IS STILL A LIVE CLAIM ABOUT THE PAGE. Swapping two
	// entries describes p76's columns in an order it does not print them in,
	// and the read must refuse rather than shift every figure one band. Note
	// what this catches is the ORDER claim in matchHeaders; the placement claim
	// is (3)'s band map plus the read in (2).
	swapped := load(t)
	for i := range swapped.Rules {
		h := swapped.Rules[i].Parts[0].ColumnHeaders
		h[2], h[3] = h[3], h[2]
	}
	if err := readAll(t, swapped); err == nil {
		t.Fatal("swapping two column headers changed nothing; the guard is not running " +
			"on this page and the null entry silently disabled it")
	} else if !strings.Contains(err.Error(), "column_headers") {
		t.Errorf("error %q does not point at column_headers", err)
	}

	// (5) And a null in the MIDDLE is refused rather than quietly declining a
	// check the grid could have made. Asserted here as well as in parse_test
	// because this is the page the rule was written for.
	mid := load(t)
	for i := range mid.Rules {
		p := &mid.Rules[i].Parts[0]
		p.Columns[1], p.Columns[4] = p.Columns[4], p.Columns[1]
		p.ColumnHeaders[1], p.ColumnHeaders[4] = p.ColumnHeaders[4], p.ColumnHeaders[1]
	}
	if err := mid.Validate(); err == nil {
		t.Error("a null between two headers was accepted; it sits in a gap with known " +
			"bounds that the grid would have checked")
	}
}
