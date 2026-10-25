package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
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
