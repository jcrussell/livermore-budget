package export

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/project"
)

// seriesSums is each column's sum of the points of every series of kind, by
// "FY basis"; a column of no such point is absent, not zero.
func seriesSums(t *testing.T, raw []byte, kind string) map[string]int64 {
	t.Helper()
	var doc struct {
		Series []struct {
			Kind   string `json:"kind"`
			Points []struct {
				FiscalYear  int    `json:"fiscal_year"`
				Basis       string `json:"basis"`
				AmountCents int64  `json:"amount_cents"`
			} `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	sums := map[string]int64{}
	for _, s := range doc.Series {
		if s.Kind != kind {
			continue
		}
		for _, p := range s.Points {
			sums[fmt.Sprintf("FY%d %s", p.FiscalYear, p.Basis)] += p.AmountCents
		}
	}
	return sums
}

// TestTheAuditedRevenueRowsLessTheSpendingRowsAreThePrintedExcess: in every
// column of the changes document the change page charts, its revenue rows
// summed less its expenditure rows summed is the excess of revenues over
// expenditures the report prints, to the cent. The rows are the page's and
// the excess a printed figure, so no difference is computed on the page.
// Mutation: drop a revenue series, or a column's excess, from the document
// and its columns miss.
func TestTheAuditedRevenueRowsLessTheSpendingRowsAreThePrintedExcess(t *testing.T) {
	built, err := cachedBuildAll(repoRootForTest(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := built.Projections[project.ChangesProjection]
	if !ok {
		t.Fatalf("no %s document was built", project.ChangesProjection)
	}
	revenue, spending := seriesSums(t, raw, "revenue"), seriesSums(t, raw, "expenditure")
	excess := seriesSums(t, raw, "fund_balance")
	if len(excess) < 2 {
		t.Fatalf("the document prints the excess in %d columns, so nothing below holds the sums to it", len(excess))
	}
	cols := maps.Clone(excess)
	maps.Copy(cols, revenue)
	maps.Copy(cols, spending)
	for _, col := range slices.Sorted(maps.Keys(cols)) {
		r, rok := revenue[col]
		s, sok := spending[col]
		e, eok := excess[col]
		if !rok || !sok || !eok {
			t.Errorf("%s: revenue rows %t, expenditure rows %t, a printed excess %t; a column the page charts has all three", col, rok, sok, eok)
			continue
		}
		if r-s != e {
			t.Errorf("%s: revenue rows %d less expenditure rows %d is %d, and the report prints %d", col, r, s, r-s, e)
		}
	}
	t.Logf("%d columns, each revenue less spending equal to the printed excess", len(excess))
}
