package mapping

import (
	"strings"
	"testing"
)

// The shape these tests are about, as Budget Book pp.80-81 print it: a labelled
// page, then a page continuing the same rows with nothing but figures, whose
// first row is "$"-prefixed exactly as its total is.
//
//	p2  Principal Total
//	    $   110
//	         60
//	    $   170
//
// "$" first occurs on the first ROW, so stop_at "$" alone ends the block before
// any row; the total is the second "$". 110 + 60 = 170.
const stopAtOrdinalRules = `schema_version: 1
doc_id: ordinal-doc

rules:
  - id: debt
    kind: expenditure
    basis: adopted
    scope: debt-service-by-issue
    grain: category
    units: dollars
    total_row: "Total"
    rows:
      - {label: "Alpha", category: debt-services/principal}
      - {label: "Beta", category: debt-services/principal}
    parts:
      - page: 1
        section: "DEBT\n"
        stop_at: "Total  "
        columns:
          - {fiscal_year: 2025}
      - page: 2
        labels_from: 1
        section: "Total\n"
        stop_at: "$"
        #ORDINAL
        columns:
          - {fiscal_year: 2026}
`

func stopAtOrdinalDoc() map[int]string {
	return map[int]string{
		1: "DEBT\n" +
			"  Alpha      $   100\n" +
			"  Beta           50\n" +
			"  Total      $   150\n",
		2: "Principal Total\n" +
			"$   110\n" +
			"     60\n" +
			"$   170\n" +
			"FOOTER\n",
	}
}

func stopAtOrdinalResolver(t *testing.T, ordinal string) (*Resolver, *Rule, error) {
	t.Helper()
	src := strings.Replace(stopAtOrdinalRules, "        #ORDINAL", ordinal, 1)
	f, err := parse(strings.NewReader(src), "ordinal.yaml")
	if err != nil {
		return nil, nil, err
	}
	r, err := NewResolver(inlineDoc(t, f.DocID, stopAtOrdinalDoc()), f)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	return r, &f.Rules[0], nil
}

// TestStopAtOrdinalEndsTheBlockAtTheCountedOccurrence reads the label-less page
// whole and ties it to the "$" total the ordinal names.
//
// It also holds the positional read to dropping detached currency marks: the
// first row's "$" is inside the block, and counted as a value it makes three
// tokens where two rows want two.
//
// Mutations: resolve stop_at by its first occurrence whatever the ordinal says,
// and the block is empty, so the read fails; drop the dropCurrencyMarks call
// from positionalValues, and it reads three values for two.
func TestStopAtOrdinalEndsTheBlockAtTheCountedOccurrence(t *testing.T) {
	r, rule, err := stopAtOrdinalResolver(t, "        stop_at_ordinal: 2")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	p := &rule.Parts[1]
	values, _, err := r.Values(rule, p)
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	var got []string
	for _, v := range values {
		got = append(got, v.Row.Label+"="+v.Token)
	}
	if want := "Alpha=110 Beta=60"; strings.Join(got, " ") != want {
		t.Errorf("p2 read %q, want %q", strings.Join(got, " "), want)
	}
	if _, err := r.CheckTotals(rule, p); err != nil {
		t.Errorf("CheckTotals: %v; 110 + 60 should tie to the printed $170", err)
	}
}

// TestStopAtWithoutTheOrdinalStopsAtTheFirstRow is why the field exists: the
// first "$" on such a page is a row's, so the unadorned anchor reads nothing.
func TestStopAtWithoutTheOrdinalStopsAtTheFirstRow(t *testing.T) {
	r, rule, err := stopAtOrdinalResolver(t, "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, _, err := r.Values(rule, &rule.Parts[1]); err == nil {
		t.Fatal("stop_at \"$\" with no ordinal read the page; its first \"$\" is the first " +
			"row's, so the block should be empty and the count refused")
	}
}

// TestAStopAtOrdinalPastThePageIsRefused: an ordinal counting more occurrences
// than the page prints names no line, and says how many there are.
func TestAStopAtOrdinalPastThePageIsRefused(t *testing.T) {
	r, rule, err := stopAtOrdinalResolver(t, "        stop_at_ordinal: 3")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, _, err = r.Values(rule, &rule.Parts[1])
	if err == nil || !strings.Contains(err.Error(), "occurs 2 times after the block's start") {
		t.Errorf("stop_at_ordinal 3 over two \"$\": got %v, want a refusal counting 2", err)
	}
}

func TestStopAtOrdinalIsRefusedWhereItCannotMean(t *testing.T) {
	for _, tc := range []struct {
		name, ordinal, from, to, want string
	}{
		{"negative", "        stop_at_ordinal: -1", "", "", "ordinals count from 1"},
		{"no stop_at", "        stop_at_ordinal: 2", `        stop_at: "$"` + "\n", "", "stop_at is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(stopAtOrdinalRules, "        #ORDINAL", tc.ordinal, 1)
			if tc.from != "" {
				if !strings.Contains(src, tc.from) {
					t.Fatalf("the mutation %q matches nothing", tc.from)
				}
				src = strings.Replace(src, tc.from, tc.to, 1)
			}
			_, err := parse(strings.NewReader(src), "ordinal.yaml")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("parse: got %v, want an error containing %q", err, tc.want)
			}
		})
	}
}
