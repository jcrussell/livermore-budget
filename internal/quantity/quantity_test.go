package quantity

import (
	"strings"
	"testing"
)

// Every token below is real: read off the Statistical Section pages named in
// docs/acfr-statistical-section.md, not invented to fit the grammar.
func TestPercentage(t *testing.T) {
	for _, tok := range []string{
		"2.5%",   // p177
		"0.00%",  // p179
		"9.50%",  // p171
		"100.0%", // p181's printed total
		"13.15%", // p192
		"1.28%",  // p182
	} {
		if err := Percentage(tok); err != nil {
			t.Errorf("Percentage(%q) = %v, want accepted", tok, err)
		}
	}
	for _, tok := range []string{
		"2.5",    // a bare figure claims nothing about being a ratio
		"%",      // no figure
		"NA",     // p185
		"(2)",    // p185's footnote-only cells
		"(0.5%)", // parenthesized negative; no page prints one, fail closed
		"1,234,56%",
		"-",
		"2.5%%",
	} {
		if err := Percentage(tok); err == nil {
			t.Errorf("Percentage(%q) accepted, want refused", tok)
		}
	}
}

func TestAmountPerUnit(t *testing.T) {
	for _, tok := range []string{
		"1,009", // p177 per capita
		"922",
		"$1,009",
		"32.50", // p186 meter charges
		"16.5",
		"2.8",
		"1,044", // p180
		"-",     // a printed zero rate is a cell
	} {
		if err := AmountPerUnit(tok); err != nil {
			t.Errorf("AmountPerUnit(%q) = %v, want accepted", tok, err)
		}
	}
	for _, tok := range []string{
		"exempt",    // p186's Zone 7 column
		"5600-6000", // p182's ranges
		"900-100",   // p182's range that is not even a range
		"1,234,56",  // lost decimal point
		"NA",
	} {
		if err := AmountPerUnit(tok); err == nil {
			t.Errorf("AmountPerUnit(%q) accepted, want refused", tok)
		}
	}
}

func TestNumber(t *testing.T) {
	for _, tok := range []string{
		"84,849", // p181's population total
		"395.00", // p183 FTEs
		"1.1277", // p174's 4dp rate — more decimals than dollars can hold
		"2.73",   // p189 coverage
		"34,662", // p188 DUEs
		"5",
	} {
		if err := Number(tok); err != nil {
			t.Errorf("Number(%q) = %v, want accepted", tok, err)
		}
	}
	for _, tok := range []string{
		"NA", "N/A", "(2)", // p185
		"5600-6000", "900-100", // p182
		"-",        // no page has yet printed a dash in a number column; fail closed
		"$5",       // a currency mark claims money
		"1,234,56", // lost decimal point
		"",
	} {
		if err := Number(tok); err == nil {
			t.Errorf("Number(%q) accepted, want refused", tok)
		}
	}
}

// TestErrorsNameTheGrammar: a resolution failure surfaces these messages
// verbatim, and a reader fixing a rule must be able to tell "the cell is not a
// percentage" from amount.Parse's "not a recognized number".
func TestErrorsNameTheGrammar(t *testing.T) {
	if err := Percentage("NA"); err == nil || !strings.Contains(err.Error(), "percentage") {
		t.Errorf("Percentage(\"NA\") = %v, want an error naming the grammar", err)
	}
	if err := Number("NA"); err == nil || !strings.Contains(err.Error(), "number") {
		t.Errorf("Number(\"NA\") = %v, want an error naming the grammar", err)
	}
}
