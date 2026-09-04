package registry

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// p76Fixture is the real extracted page, not a synthetic one: the labels this
// test resolves have to be the strings poppler actually produced, spacing and
// truncation included. It is a copy of
// data/extracted/livermore-budget-fy2026-2027/pages/p0076.txt.
const p76Fixture = "../../testdata/pages/budget-p0076.txt"

// budgetPages is the extracted budget book, which alias pages cite. Reading it
// is what makes "the city prints this on page N" a checkable claim rather than
// an assertion; internal/mapping's tests read the same tree.
const budgetPages = "../../data/extracted/livermore-budget-fy2026-2027/pages"

func realRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := Load(os.DirFS(realData))
	if err != nil {
		t.Fatalf("Load(%s): %v", realData, err)
	}
	return r
}

// p76 prints each transfer as "Transfer From <source>  to <destination>",
// with the source omitted on a continuation row that shares the row above.
// Both patterns stop at the two-space gap that ends a column, and the
// destination is anchored to the start of a row so that the footnote
// "4. Advance to cover fund balance deficit" is not read as a fund.
var (
	p76Source      = regexp.MustCompile(`Transfer From (\S.*?)\s\s+`)
	p76Destination = regexp.MustCompile(`^\s*(?:Transfer From \S.*?\s\s+)?to (\S.*?)\s\s+`)
)

// transferLabels reads every fund label p76 prints, in first-seen order.
func transferLabels(t *testing.T) (sources, destinations []string) {
	t.Helper()
	b, err := os.ReadFile(p76Fixture)
	if err != nil {
		t.Fatalf("read %s: %v", p76Fixture, err)
	}
	srcSeen, dstSeen := map[string]bool{}, map[string]bool{}
	for _, line := range strings.Split(string(b), "\n") {
		if m := p76Source.FindStringSubmatch(line); m != nil && !srcSeen[m[1]] {
			srcSeen[m[1]] = true
			sources = append(sources, m[1])
		}
		if m := p76Destination.FindStringSubmatch(line); m != nil && !dstSeen[m[1]] {
			dstSeen[m[1]] = true
			destinations = append(destinations, m[1])
		}
	}
	return sources, destinations
}

// The acceptance for fisc-8dz: every fund p76 names in prose resolves to one
// fund number, or fails naming itself. Nothing here is a near match.
//
// The five ambiguous pairs are the point. "Low Income Hsng", "Traffic Imp
// Fee", "Host Comm Impact", "Measure D" and "State Gas Tax" each head an
// operating fund AND its CIP twin, so the expected numbers below (200 not
// 812, 510 not 823, 282 not 820, 550 not 828, 560 not 834) are the assertion:
// change any alias in funds.yaml to its twin and this test says so.
func TestP76LabelsResolveOrFailByName(t *testing.T) {
	r := realRegistry(t)

	wantSources := map[string]int{
		"Low Income Hsng":  200, // not 812, CIP Low Income Housing
		"Home Grant":       225,
		"State Grant":      240,
		"Downtown Revital": 280,
		"Host Comm Impact": 282, // not 820, CIP Host Community Impact Fee
		"CASP Fee":         286,
		"Open Space":       300, // not 471, Doolan Canyon Open Space
		"Downtown LMD":     310,
		"Other LMDs":       311,
		"Traffic Imp Fee":  510, // not 823, CIP Traffic Impact Fee
		"Measure D":        550, // not 828, CIP County Measure D
		"State Gas Tax":    560, // not 834, CIP State Gas Tax
		"General Fund":     100,
		"Wastewater":       620,
		"Water":            640,
		"Water Replace":    642,
	}
	wantDestinations := map[string]int{
		"General Fund":                 100,
		"Horizons":                     210,
		"Import Mitigation Fee":        288,
		"Downtown LMD":                 310,
		"2020 COPS Series A":           400,
		"2020 COPS Series B":           401,
		"2022 COPS":                    402,
		"Doolan Canyon Preserve Endow": 470,
		"Stormwater":                   610,
		"Wastewater Replacement":       622,
		"Water":                        640,
		"Water Replacement":            642,
	}
	// The one label on p76 that names no fund. "LAVWMA / Wastewater
	// Connection" is printed across two lines and names a joint powers
	// authority, not a budgeted fund; the whole point of the channel is that
	// it says so instead of picking Wastewater Connection Fees (623).
	wantUnresolved := []string{"LAVWMA / Wastewater"}

	sources, destinations := transferLabels(t)
	if got, want := len(sources), len(wantSources); got != want {
		t.Errorf("p76 prints %d distinct source labels %q, want %d", got, sources, want)
	}
	if got, want := len(destinations), len(wantDestinations)+len(wantUnresolved); got != want {
		t.Errorf("p76 prints %d distinct destination labels %q, want %d", got, destinations, want)
	}

	for _, tt := range []struct {
		side   string
		labels []string
		want   map[string]int
	}{
		{"source", sources, wantSources},
		{"destination", destinations, wantDestinations},
	} {
		got := map[string]int{}
		var unresolved []string
		for _, label := range tt.labels {
			f, err := r.FundByLabel(label)
			if err != nil {
				var unknown *unknownFundError
				if !errors.As(err, &unknown) {
					t.Errorf("FundByLabel(%q) error is %T, want *UnknownFundError", label, err)
					continue
				}
				if unknown.Label != label {
					t.Errorf("UnknownFundError.Label = %q, want the printed label %q", unknown.Label, label)
				}
				unresolved = append(unresolved, label)
				continue
			}
			got[label] = f.Number
		}
		if diff := cmp.Diff(tt.want, got); diff != "" {
			t.Errorf("%s labels mismatch (-want +got):\n%s", tt.side, diff)
		}
		if tt.side == "destination" {
			if diff := cmp.Diff(wantUnresolved, unresolved); diff != "" {
				t.Errorf("unresolved destination labels mismatch (-want +got):\n%s", diff)
			}
		} else if len(unresolved) > 0 {
			t.Errorf("unresolved source labels %q, want none", unresolved)
		}
	}
}

// An unresolved label must fail by name, and the message must be usable by
// whoever has to fix funds.yaml.
func TestFundByLabelFailsByName(t *testing.T) {
	r := realRegistry(t)

	const label = "LAVWMA / Wastewater"
	_, err := r.FundByLabel(label)
	if err == nil {
		t.Fatalf("FundByLabel(%q) = nil error, want one", label)
	}
	want := `no fund is named "LAVWMA / Wastewater" in funds.yaml; a printed label resolves only by an exact match on a fund's name or a declared alias`
	if got := err.Error(); got != want {
		t.Errorf("FundByLabel(%q) error =\n%q\nwant\n%q", label, got, want)
	}
	var unknown *unknownFundError
	if !errors.As(err, &unknown) {
		t.Fatalf("error is %T, want *UnknownFundError", err)
	}
}

// Every way of being close is still a miss. A label the city did not print is
// not resolved by trimming it, by case-folding it, or by taking the fund whose
// name it starts.
func TestFundByLabelIsExact(t *testing.T) {
	r := realRegistry(t)

	for _, tt := range []struct {
		label string
		want  int
	}{
		{"County Measure D", 550},     // the appendix name
		{"Measure D", 550},            // p76's abbreviation
		{"CIP County Measure D", 828}, // the twin, by its own name
		{"Police Evidence", 211},      // p136's rename
		{"Asset Seizure - County", 211},
		{"Community Beneift Fund", 291}, // the appendix typo, stated not fixed
		{"Community Benefit Fund", 291},
	} {
		f, err := r.FundByLabel(tt.label)
		if err != nil {
			t.Errorf("FundByLabel(%q): %v", tt.label, err)
			continue
		}
		if f.Number != tt.want {
			t.Errorf("FundByLabel(%q) = fund %d (%q), want %d", tt.label, f.Number, f.Name, tt.want)
		}
	}

	for _, label := range []string{
		"measure d",               // case folded
		"Measure",                 // a prefix of the alias
		"Measure D Fund",          // the alias plus a word
		" Measure D",              // untrimmed
		"CIP Measure D",           // a plausible name for the twin that nothing prints
		"Low Income Housing",      // 200's name minus "Fund", 812's minus "CIP"
		"Community Benefit Fund ", // trailing space
	} {
		if f, err := r.FundByLabel(label); err == nil {
			t.Errorf("FundByLabel(%q) = fund %d (%q), want an error", label, f.Number, f.Name)
		}
	}
}

// An alias asserts that the city prints this string on these pages. The
// assertion is checked here against the extracted text, so an alias invented
// at a desk fails rather than resolving a label the document never used.
func TestFundAliasesArePrintedOnTheirPages(t *testing.T) {
	r := realRegistry(t)

	pages := map[int]string{}
	readPage := func(t *testing.T, n int) string {
		t.Helper()
		if body, ok := pages[n]; ok {
			return body
		}
		b, err := os.ReadFile(fmt.Sprintf("%s/p%04d.txt", budgetPages, n))
		if err != nil {
			t.Fatalf("read page %d: %v", n, err)
		}
		pages[n] = string(b)
		return pages[n]
	}

	var count int
	for _, f := range r.Funds() {
		for _, a := range f.Aliases {
			count++
			for _, p := range a.Pages {
				if !strings.Contains(readPage(t, p), a.Term) {
					t.Errorf("fund %d alias %q is not printed on p%04d", f.Number, a.Term, p)
				}
			}
		}
	}
	// A bare guard against an alias block being dropped wholesale by an edit
	// that still parses.
	if count < 20 {
		t.Errorf("funds.yaml declares %d aliases, want at least 20", count)
	}
}

// Funds() and Fund() hand out copies: a consumer that trimmed an alias must
// not change what the next lookup sees.
func TestFundsAreCopies(t *testing.T) {
	r := realRegistry(t)

	// Snapshot before mutating, so the comparison below is against what the
	// registry held rather than against the post-mutation value of the same
	// field. Fund() already copies, which is the property under test, so this
	// snapshot survives the clobbering loop.
	before, ok := r.Fund(211)
	if !ok {
		t.Fatal("Fund(211) not found")
	}

	funds := r.Funds()
	for i := range funds {
		for j := range funds[i].Aliases {
			funds[i].Aliases[j].Term = "clobbered"
			funds[i].Aliases[j].Pages[0] = -1
		}
	}
	again, ok := r.Fund(211)
	if !ok {
		t.Fatal("Fund(211) not found")
	}
	if diff := cmp.Diff(before.Aliases, again.Aliases); diff != "" {
		t.Errorf("fund 211 aliases mismatch after mutating a copy (-want +got):\n%s", diff)
	}
	// Pin the fields the mutation targets, so the diff above cannot pass by
	// both sides having been clobbered identically.
	if got := again.Aliases[0]; got.Term != "Police Evidence" || got.Pages[0] != 136 {
		t.Errorf("fund 211 alias 0 = %q pages %v, want \"Police Evidence\" pages [136]", got.Term, got.Pages)
	}
	if f, err := r.FundByLabel("Police Evidence"); err != nil || f.Number != 211 {
		t.Errorf("FundByLabel(\"Police Evidence\") = %d, %v after mutating a copy, want 211", f.Number, err)
	}
}
