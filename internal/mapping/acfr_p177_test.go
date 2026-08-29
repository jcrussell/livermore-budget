package mapping

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// ACFR p177, the ten-year schedule of outstanding debt by type. It is the page
// behind this project's most-cited claim: AGENTS.md, under "Review does not
// cover this project's main risks", names amount.TestLeadingMinusIsReallyPositive
// as the worked example of proving a reading with arithmetic rather than
// intuition. This file is that example's other half -- the test carries the
// arithmetic, and this reads the same row off the committed fixture.
//
// That test reconciles a row TRANSCRIBED INTO ITS OWN COMMENT. Nothing checked
// the transcription against the document, and the transcription is of what
// XBERG produced: a "-512,946" with the dash of an empty column glued to the
// front. The committed corpus does not contain that token, or any token of that
// shape -- a leading minus on a grouped number appears zero times across all 786
// extracted pages. The parser must still refuse one, which the amount package
// asserts; what changes here is where the evidence comes from.
//
// The resolver cannot read this page, and that is recorded rather than worked
// around: the FY2016 row prints its figures with a STANDALONE "$" before each
// one ("$ 60,193,384", two tokens), which is fisc-yun. The page is parsed here
// with strings.Fields and amount.Parse -- the real parser on every token, which
// is the load-bearing part -- and the row structure is a year in column one, so
// there is no row-identity judgment to get wrong.
const acfrDebtPage = 177

// acfrDebtRow is one year of the schedule as the page prints it: seven debt
// columns, the city's own total, and two ratios this test does not read.
type acfrDebtRow struct {
	year   int
	parts  []amount.Cents
	stated amount.Cents
}

var acfrYear = regexp.MustCompile(`^20\d\d$`)

// readACFRDebtSchedule parses every year row of the fixture.
func readACFRDebtSchedule(t *testing.T) []acfrDebtRow {
	t.Helper()

	path := fmt.Sprintf("../../testdata/pages/acfr-p%04d.txt", acfrDebtPage)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}

	var out []acfrDebtRow
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !acfrYear.MatchString(fields[0]) {
			continue
		}
		year := 0
		for _, r := range fields[0] {
			year = year*10 + int(r-'0')
		}

		// The FY2016 row prints a standalone "$" before each figure. Dropping
		// it here is what a rule cannot do (fisc-yun); it is dropped rather
		// than parsed because "$" is not an amount and never was one.
		var cents []amount.Cents
		for _, tok := range fields[1:] {
			if tok == "$" {
				continue
			}
			c, err := amount.Parse(tok, amount.Dollars)
			if err != nil {
				// The two ratio columns end the row: a percentage and a
				// per-capita figure the schedule does not total.
				break
			}
			cents = append(cents, c)
		}
		if len(cents) < 8 {
			t.Fatalf("FY%d: got %d figures, want at least 8 (seven debt columns "+
				"and the city's total)", year, len(cents))
		}
		out = append(out, acfrDebtRow{year: year, parts: cents[:7], stated: cents[7]})
	}
	if len(out) != 10 {
		t.Fatalf("got %d year rows, want 10; this is a ten-year schedule", len(out))
	}
	return out
}

// TestACFRDebtScheduleTiesExceptOneRow is the schedule checking our reading of
// it, and it is the reason the fixture is worth committing: nine of the ten
// rows tie to the city's own printed total EXACTLY, which is what makes the
// tenth a fact about the document rather than a suspicion about the parser.
//
// FY2024's printed total is short by 176,292 -- exactly its own Financed
// Purchases figure, the column the city started using that year. This is not
// the "<= $5, always the FY2023-24 Actual column" rounding that fisc-2sd
// declares for the Budget Book; it is a whole column missing from a total.
func TestACFRDebtScheduleTiesExceptOneRow(t *testing.T) {
	rows := readACFRDebtSchedule(t)

	const (
		shortYear                         = 2024
		financedPurchasesCol              = 4
		shortBy              amount.Cents = 17629200
	)
	for _, row := range rows {
		var sum amount.Cents
		for _, c := range row.parts {
			sum += c
		}
		diff := sum - row.stated
		switch {
		case row.year == shortYear:
			if diff != shortBy {
				t.Errorf("FY%d: rows sum to %s against a printed %s, a difference of %s; want %s",
					row.year, sum, row.stated, diff, shortBy)
			}
			// Naming the column is the whole claim. A difference that merely
			// happened to be 176,292 would prove nothing.
			if got := row.parts[financedPurchasesCol]; got != shortBy {
				t.Errorf("FY%d: the difference is %s but Financed Purchases is %s, "+
					"so the total is not simply missing that column",
					row.year, shortBy, got)
			}
		case diff != 0:
			t.Errorf("FY%d: rows sum to %s against a printed %s, off by %s",
				row.year, sum, row.stated, diff)
		}
	}
}

// TestACFRDebtRowIsNotCorruptedInTheCommittedCorpus is the flagship claim,
// moved off a transcription and onto the page.
//
// The FY2017 row is the one amount.TestLeadingMinusIsReallyPositive reconciles.
// Under poppler it carries seven figures, two of them dashes the city prints as
// published zeros, and the 512,946 stands alone: the token that test quotes as
// "-512,946" is not in the corpus. Both readings still hold and neither is
// weakened -- the arithmetic proves the sign, and a leading minus is still
// refused -- but the corruption is xberg's, not this document's.
func TestACFRDebtRowIsNotCorruptedInTheCommittedCorpus(t *testing.T) {
	rows := readACFRDebtSchedule(t)

	var row acfrDebtRow
	for _, r := range rows {
		if r.year == 2017 {
			row = r
		}
	}
	if row.year != 2017 {
		t.Fatal("no FY2017 row, which is the row the amount package reconciles")
	}

	// The two empty columns are read as published zeros, which is the reading
	// the glued dash destroyed: "absent is not zero" cuts both ways, and here
	// the document prints the dash.
	zeros := 0
	for _, c := range row.parts {
		if c == 0 {
			zeros++
		}
	}
	if zeros != 2 {
		t.Errorf("got %d zero columns in the FY2017 row, want 2 (the two the "+
			"city prints as dashes)", zeros)
	}

	const stated amount.Cents = 8264398000
	if row.stated != stated {
		t.Errorf("got a printed total of %s, want %s", row.stated, stated)
	}

	// The token itself, read straight off the page rather than out of a
	// comment. If a future extractor glues a dash onto it again, this fails
	// here and amount.Parse fails there.
	if got, want := row.parts[6], amount.Cents(51294600); got != want {
		t.Errorf("got %s in the column the glued dash landed on, want %s", got, want)
	}
	if _, err := amount.Parse("-512,946", amount.Dollars); err == nil {
		t.Error("Parse accepted the corrupted form; it must still fail closed")
	}
}
