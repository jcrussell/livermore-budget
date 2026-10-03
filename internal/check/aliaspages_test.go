package check

import (
	"slices"
	"strings"
	"testing"
)

// TestAnAliasListsEveryPageARuleReadsItOn holds data/funds.yaml's alias pages
// to the rules that resolve by them. An alias's Pages is its only source note
// -- "this string appears on these pages" -- so a rule declaring
// row_labels_name_funds that reads a row by an alias on a page the alias does
// not list is a binding whose stated evidence never mentions where it was used.
func TestAnAliasListsEveryPageARuleReadsItOn(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	checked := 0
	for _, f := range s.Files {
		doc := s.Docs[f.DocID]
		for i := range f.Rules {
			r := &f.Rules[i]
			if !r.RowLabelsNameFunds {
				continue
			}
			for _, row := range r.Rows {
				if row.Skip || row.Fund == 0 {
					continue
				}
				label := row.PrintedLabel()
				fund, ok := s.Vocabulary.Fund(row.Fund)
				if !ok {
					t.Fatalf("%s row %q: fund %d is not in data/funds.yaml", r.ID, label, row.Fund)
				}
				ai := -1
				for j, a := range fund.Aliases {
					if a.Term == label {
						ai = j
					}
				}
				if ai < 0 {
					continue
				}
				for _, p := range r.Parts {
					text, err := doc.Page(p.Page)
					if err != nil {
						t.Fatalf("%s p%d: %v", r.ID, p.Page, err)
					}
					if !printsRowLabel(text, label) {
						continue
					}
					checked++
					if !slices.Contains(fund.Aliases[ai].Pages, p.Page) {
						t.Errorf("%s reads fund %d by alias %q on p%d, which the alias's pages %v do not list",
							r.ID, row.Fund, label, p.Page, fund.Aliases[ai].Pages)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no rule reads a row by an alias; this test checked nothing")
	}
	t.Logf("%d (rule, row, page) readings by alias checked", checked)
}

// printsRowLabel reports whether some line of a page's layout text begins with
// label as a whole word run: followed by nothing, by space, or by a figure.
func printsRowLabel(text, label string) bool {
	for line := range strings.SplitSeq(text, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), label)
		if ok && (rest == "" || rest[0] == ' ') {
			return true
		}
	}
	return false
}
