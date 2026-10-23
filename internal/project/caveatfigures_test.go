package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// The figures the caveats below quote as literals, each re-measured here from
// the store, the registry, the mapping or the committed p76 page, so a caveat
// cannot go on stating a figure the corpus has stopped printing. Mutation:
// change any quoted figure in its caveat and its subtest goes red.

func caveatNamed(t *testing.T, cs []Caveat, id string) string {
	t.Helper()
	for _, c := range cs {
		if c.ID == id {
			return c.Summary + " " + c.Text
		}
	}
	t.Fatalf("no caveat %q", id)
	return ""
}

func TestTheCaveatsQuoteWhatTheCorpusPrints(t *testing.T) {
	transfers := transfersByFundCaveats()

	t.Run("p76's grand totals and the General Fund's legs", func(t *testing.T) {
		text := caveatNamed(t, transfers, "one-figure-is-two-ribbons")
		ends := caveatNamed(t, transfers, "a-fund-is-drawn-once-per-end")
		for _, year := range []int{2026, 2027} {
			d := committedDocs(t, year)[TransfersByFundProjection]
			if d == nil {
				t.Fatalf("FY%d: no %s document", year, TransfersByFundProjection)
			}
			var all int64
			receives, pays := 0, 0
			for _, l := range d.Links {
				all += l.ValueCents
				if l.Target == PrefixFund+"100" {
					receives++
				}
				if l.Source == PrefixFund+"100" {
					pays++
				}
			}
			if all%2 != 0 || !strings.Contains(text, amount.Cents(all/2).Dollars()) {
				t.Errorf("FY%d: the ribbons sum to %s and the caveat does not quote half of it: %q",
					year, amount.Cents(all).Dollars(), text)
			}
			if want := fmt.Sprintf("receives %s transfers and pays %s", numberWord(t, receives), numberWord(t, pays)); !strings.Contains(ends, want) {
				t.Errorf("FY%d: the General Fund %s, which the caveat does not say: %q", year, want, ends)
			}
		}
	})

	t.Run("p76's historical columns miss its total", func(t *testing.T) {
		text := caveatNamed(t, transfers, "only-the-budget-columns-are-published")
		sums, total := p76Columns(t)
		for i, want := range []string{"$6,858,051", "$5,000,000"} {
			if got := amount.Cents(total[i] - sums[i]).Dollars(); got != want || !strings.Contains(text, want) {
				t.Errorf("column %d misses its printed total by %s; the caveat quotes %s", i+1, got, want)
			}
		}
		for i := 2; i < 4; i++ {
			if sums[i] != total[i] {
				t.Errorf("budget column %d misses its printed total by %d cents, and the caveat says only the historical ones do", i+1, total[i]-sums[i])
			}
		}
	})

	t.Run("the departments' one-dollar misses", func(t *testing.T) {
		text := caveatNamed(t, departmentFundingCaveats(), "two-of-the-four-columns-tie-to-no-citywide-total")
		depts, missing, other := departmentDeltas(t)
		want := fmt.Sprintf("%s of the %s departments miss", capital(numberWord(t, missing)), numberWord(t, depts))
		if !strings.Contains(text, want) || other != 0 {
			t.Errorf("the mapping declares %d one-dollar misses in FY2023-24 actual of %d departments and %d other deltas; the caveat says %q",
				missing, depts, other, text)
		}
	})

	t.Run("the permanent funds", func(t *testing.T) {
		store, err := committedStore()
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, f := range store.reg.Funds() {
			if f.Type == "permanent" {
				n++
			}
		}
		if want := fmt.Sprintf("(%d funds)", n); !strings.Contains(caveatPermanentFunds.Text, want) {
			t.Errorf("data/funds.yaml types %d funds permanent; the caveat says %q", n, caveatPermanentFunds.Text)
		}
	})
}

func numberWord(t *testing.T, n int) string {
	t.Helper()
	words := []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}
	if n < 0 || n >= len(words) {
		t.Fatalf("no word for %d", n)
	}
	return words[n]
}

func capital(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

// p76Columns sums each of p76's four columns over its rows, and reads its
// printed grand total, off the committed fixture.
func p76Columns(t *testing.T) (sums, total [4]int64) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "pages", "budget-p0076.txt"))
	if err != nil {
		t.Fatal(err)
	}
	rows, totals := 0, 0
	for _, line := range strings.Split(string(body), "\n") {
		fs := strings.Fields(line)
		if n := len(fs); n > 0 && strings.HasPrefix(fs[n-1], "(") && strings.HasSuffix(fs[n-1], ")") {
			fs = fs[:n-1]
		}
		if len(fs) < 4 {
			continue
		}
		var cells [4]int64
		ok := true
		for i, tok := range fs[len(fs)-4:] {
			c, err := amount.Parse(tok, amount.Dollars)
			if err != nil {
				ok = false
				break
			}
			cells[i] = int64(c)
		}
		if !ok {
			continue
		}
		if len(fs) == 4 && strings.HasPrefix(fs[0], "$") {
			total, totals = cells, totals+1
			continue
		}
		rows++
		for i := range cells {
			sums[i] += cells[i]
		}
	}
	if totals != 1 || rows == 0 {
		t.Fatalf("read %d rows and %d grand totals off p76, want rows and one total", rows, totals)
	}
	return sums, total
}

// departmentDeltas counts the departments the mapping reads funding sources
// for, the stated-total deltas of one dollar in their FY2023-24 actual
// column, and any other delta.
func departmentDeltas(t *testing.T) (depts, oneDollar, other int) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "mappings", "livermore-budget-fy2026-2027.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Rules []struct {
			Scope string `yaml:"scope"`
			Rows  []struct {
				Department string `yaml:"department"`
			} `yaml:"rows"`
			Parts []struct {
				Columns []struct {
					FiscalYear int    `yaml:"fiscal_year"`
					Basis      string `yaml:"basis"`
				} `yaml:"columns"`
				Deltas []struct {
					Column int   `yaml:"column"`
					Cents  int64 `yaml:"delta_cents"`
				} `yaml:"stated_total_deltas"`
			} `yaml:"parts"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(body, &file); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range file.Rules {
		if r.Scope != "department-funding-sources" {
			continue
		}
		for _, row := range r.Rows {
			seen[row.Department] = true
		}
		for _, p := range r.Parts {
			for _, d := range p.Deltas {
				c := p.Columns[d.Column-1]
				if c.FiscalYear == 2024 && c.Basis == "actual" && (d.Cents == 100 || d.Cents == -100) {
					oneDollar++
				} else {
					other++
				}
			}
		}
	}
	return len(seen), oneDollar, other
}
