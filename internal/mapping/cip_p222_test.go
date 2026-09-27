package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

const p222RuleID = "p222-cip-funding-sources"

// p222Rule is the committed rule for Budget Book p222, with its file, so a
// test can mutate the declaration the corpus ships rather than a copy of it.
func p222Rule(t *testing.T) (*File, *Rule) {
	t.Helper()
	files, err := LoadDir(os.DirFS("../.."), "mappings")
	if err != nil {
		t.Fatalf("load the committed rule files: %v", err)
	}
	for _, f := range files {
		for i := range f.Rules {
			if f.Rules[i].ID == p222RuleID {
				return f, &f.Rules[i]
			}
		}
	}
	t.Fatalf("no committed rule %q", p222RuleID)
	return nil, nil
}

func p222Resolver(t *testing.T, f *File) *Resolver {
	t.Helper()
	r, err := NewResolver(budgetDoc(t, 222), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r
}

// TestP222TiesToItsPrintedTotals reads the CIP funding bridge and holds it to
// p222's own "Totals" line: 104,665,416, 47,084,623 and 58,988,871.
func TestP222TiesToItsPrintedTotals(t *testing.T) {
	f, rule := p222Rule(t)
	r := p222Resolver(t, f)
	values, _, err := r.Values(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	// 36 printed rows by three columns. A count is what stops a read that
	// found some other block from passing.
	if got, want := len(values), 36*3; got != want {
		t.Fatalf("got %d values, want %d", got, want)
	}
	res, err := r.CheckTotals(rule, &rule.Parts[0])
	if err != nil {
		t.Fatalf("CheckTotals: %v", err)
	}
	if res.Declared != 0 || res.Tolerated != 0 {
		t.Errorf("declared %d and tolerated %d columns, want p222 to tie exactly", res.Declared, res.Tolerated)
	}

	// The rows naming an operating fund are the transfers to the CIP that
	// p0071, p0073 and p0075 print as one column.
	transfers := map[int]amount.Cents{}
	for _, v := range values {
		if v.Row.Counterpart != nil {
			transfers[v.Column.FiscalYear] += v.Cents
		}
	}
	want := map[int]amount.Cents{2025: 9296875200, 2026: 3808673700, 2027: 5076225100}
	if diff := cmp.Diff(want, transfers); diff != "" {
		t.Errorf("transfers to the CIP by year (-want +got):\n%s", diff)
	}

	t.Run("a row dropped from the sum is refused by the printed total", func(t *testing.T) {
		f, rule := p222Rule(t)
		for i := range rule.Rows {
			if rule.Rows[i].Label == "825" {
				rule.Rows[i].Skip = true
			}
		}
		_, err := p222Resolver(t, f).CheckTotals(rule, &rule.Parts[0])
		if err == nil || !strings.Contains(err.Error(), "47,084,623") && !strings.Contains(err.Error(), "4708462300") {
			t.Fatalf("CheckTotals = %v, want a refusal naming a printed total Park Fees no longer reaches", err)
		}
	})
}

// TestP222ReadsTwoWrappedFragmentsInOneGap: 838's description ends and 839's
// begins on the two lines between their figures. Each is declared on its own,
// and a gap holding one undeclared line is refused whole.
func TestP222ReadsTwoWrappedFragmentsInOneGap(t *testing.T) {
	for _, tc := range []struct {
		name string
		drop string
		add  string
		want string
	}{
		{name: "one of the two lines undeclared", drop: "Housing & Human Services",
			want: "sits between rows"},
		{name: "a fragment the page does not print", add: "Grant (CDBG) Housing",
			want: "is declared but does not appear"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, rule := p222Rule(t)
			p := &rule.Parts[0]
			var kept []string
			for _, w := range p.WrappedLabels {
				if w != tc.drop {
					kept = append(kept, w)
				}
			}
			if tc.add != "" {
				kept = append(kept, tc.add)
			}
			p.WrappedLabels = kept
			_, _, err := p222Resolver(t, f).Values(rule, p)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Values = %v, want a refusal saying %q", err, tc.want)
			}
		})
	}
}

// TestALabelTailNamesFieldsAndNotTheSpacesBetweenThem: p222 prints the
// transferring fund and its description after the receiving fund, with a run
// of spaces the tail does not spell. The fields must be on one printed line.
func TestALabelTailNamesFieldsAndNotTheSpacesBetweenThem(t *testing.T) {
	for _, tc := range []struct {
		name, s, tail string
		at, n         int
	}{
		{"one field", "  600       Airport  660,000", "600", 2, 3},
		{"a run of spaces", "  600       Airport  660,000", "600 Airport", 2, 17},
		{"a tab", "600\tAirport", "600 Airport", 0, 11},
		{"a later match when the first is not followed by the rest", "600 x 600 Airport", "600 Airport", 6, 11},
		{"not across a line break", "600\n Airport", "600 Airport", -1, 0},
		// A field matches as a substring, as Label does; what follows it is
		// read as the row's figures and refused there.
		{"a field matches as a substring", "600 Airports", "600 Airport", 0, 11},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, n := findFields(tc.s, tc.tail)
			if diff := cmp.Diff([]int{tc.at, tc.n}, []int{at, n}); diff != "" {
				t.Errorf("findFields(%q, %q) (-want +got):\n%s", tc.s, tc.tail, diff)
			}
		})
	}
}
