package check

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// This file is the arithmetic behind pp.85-125's UPPER block, and like
// fundingsources_test.go it is deliberately NOT a second call into the machinery
// that publishes it: check.go's doctrine is that two functions over identical
// input inside one process cannot witness a wrong amount. The first test below
// reads the pages with a scanner of its own and compares what the city printed
// against figures typed out of pp.66-67.
//
// The rest guard the exception table, which is the part of this lane that a
// reader has to take on trust and which the first review pass found overstated.

// departmentwidePages are the ELEVEN pages that carry an Expenditures by
// Category block, in printed order. Fourteen pages print the DEPARTMENTWIDE
// EXPENDITURES WITH FUNDING SOURCES heading; p102, p120 and p125 are
// continuation pages carrying funding-source rows only, which is why they are
// in fundingPages and not here.
var departmentwidePages = []int{85, 89, 93, 97, 101, 105, 108, 111, 115, 119, 124}

// objectRow matches a printed object line: an optional division label, an object
// category, and the four figures. A bare "-" is a printed zero.
//
// THE LABEL IS SEPARATED BY \s+ AND NOT BY \s{2,}, which is the whole of what
// makes this match p101. A long division label runs right up to the object
// column: p101:11 prints "Community Development Wages & Benefits" with a SINGLE
// space, where every other page leaves a wide run. A two-space rule reads that
// line as no match at all and silently drops the whole division -- the first
// draft of this test did exactly that and lost Community Development Admin's
// four figures out of a sum it then reported as short.
//
// The figures are still separated by \s{2,}, which is what keeps the object name
// from swallowing the first column.
var objectRow = regexp.MustCompile(
	`^(.*?)\s+(Wages & Benefits|Services & Supplies|Capital Outlay|Debt Services|` +
		`Transfers Out|Division Total)\s{2,}(.*)$`)

func departmentwideCents(t *testing.T, tok string) amount.Cents {
	t.Helper()
	if tok == "-" {
		return 0
	}
	c, err := amount.Parse(tok, amount.Dollars)
	if err != nil {
		t.Fatalf("amount.Parse(%q): %v", tok, err)
	}
	return c
}

// readObjectFigures pulls the four column figures off one printed line.
func readObjectFigures(t *testing.T, tail string) []amount.Cents {
	t.Helper()
	var out []amount.Cents
	for _, f := range strings.Fields(tail) {
		out = append(out, departmentwideCents(t, f))
	}
	return out
}

// THE ARITHMETIC, read off the pages rather than out of the fact store.
//
// It asserts two things a mis-read column would break: every division's object
// rows sum to its own printed Division Total, and the eleven pages' object rows
// sum, per object category, to the four figures pp.66-67 print citywide.
//
// The FY2026-27 services-and-supplies figure typed below is the SPINE's, and the
// pages come to 250,000 less -- fisc-av0w. That is asserted as a difference
// here, so this test states the discrepancy independently of the check that
// declares it.
func TestThePrintedDepartmentwideRowsSumToTheSpineByObjectCategory(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doc, ok := s.Docs[budgetDoc]
	if !ok {
		t.Fatalf("no extraction for %s", budgetDoc)
	}

	// byCategory is the FY2025-26 and FY2026-27 columns per object category;
	// divisions counts the printed Division Total rows.
	byCat := map[string][2]amount.Cents{}
	divisions, transfers, rounded := 0, 0, 0
	for _, page := range departmentwidePages {
		text, err := doc.Page(page)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		block, _, cut := strings.Cut(text, "Total Department Expenditures")
		if !cut {
			t.Fatalf("p%d prints no Total Department Expenditures row", page)
		}
		running := [4]amount.Cents{}
		for _, line := range strings.Split(block, "\n") {
			m := objectRow.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			v := readObjectFigures(t, m[3])
			if len(v) != 4 {
				t.Fatalf("p%d: %q is followed by %d figures, want 4", page, m[2], len(v))
			}
			if m[2] == "Division Total" {
				divisions++
				// Five divisions miss their own printed total by exactly one
				// dollar, every one in the FY2023-24 Actual column, and those
				// five are declared as stated_total_deltas in the rule file.
				//
				// THE TOLERATED CELLS ARE COUNTED, NOT WAIVED. An earlier draft
				// allowed a $1 difference in column 0 of every division and said
				// in its comment that "a sixth goes red". It did not: a sixth
				// rounding division would have passed silently in the lane's one
				// independent witness. The count is asserted against the five
				// declarations below.
				for c := 0; c < 4; c++ {
					d := v[c] - running[c]
					if d == 0 {
						continue
					}
					if c == 0 && d == amount.Cents(100) {
						rounded++
						continue
					}
					t.Errorf("p%d Division Total column %d: rows sum to %s, the page "+
						"prints %s, a difference of %s", page, c, running[c], v[c], d)
				}
				running = [4]amount.Cents{}
				continue
			}
			if m[2] == "Transfers Out" {
				// The one row on these pages that is not an expenditure. It is
				// inside the printed Division Total, so it stays in `running`,
				// and it is OUT of the object-category sums below.
				transfers++
				for c := 0; c < 4; c++ {
					running[c] += v[c]
				}
				continue
			}
			for c := 0; c < 4; c++ {
				running[c] += v[c]
			}
			acc := byCat[m[2]]
			acc[0] += v[2]
			acc[1] += v[3]
			byCat[m[2]] = acc
		}
	}

	if divisions != 29 || transfers != 1 {
		t.Fatalf("read %d division blocks and %d Transfers Out rows, want 29 and 1; the "+
			"schedule's shape changed and every figure below is about different rows",
			divisions, transfers)
	}
	// EXACTLY the five the rule file declares, and no more. A sixth is either a
	// mis-read column or a rounding the rules have not declared, and both must
	// be seen rather than absorbed.
	if rounded != 5 {
		t.Errorf("%d division columns miss their printed total by $1, want 5; the rule file "+
			"declares five stated_total_deltas and a sixth would be tolerated here while "+
			"failing the build", rounded)
	}

	// Typed off pp.66-67, summed over the six fund groups. The FY2026-27
	// services-and-supplies entry is the spine's own figure; the pages come to
	// 250,000 less and that is asserted as such below.
	want := map[string][2]amount.Cents{
		"Wages & Benefits":    {amount.Cents(10774112300), amount.Cents(11029702000)},
		"Services & Supplies": {amount.Cents(13352230900), amount.Cents(13050208700)},
		"Capital Outlay":      {amount.Cents(384692400), amount.Cents(311683500)},
		"Debt Services":       {amount.Cents(898505600), amount.Cents(893895400)},
	}
	for cat, w := range want {
		got := byCat[cat]
		if got[0] != w[0] {
			t.Errorf("%s FY2025-26: the pages sum to %s, the spine publishes %s",
				cat, got[0], w[0])
		}
		short := w[1] - got[1]
		if cat == "Services & Supplies" {
			// fisc-av0w, stated as a difference so this test carries the
			// discrepancy rather than inheriting it from the check.
			if short != amount.Cents(25000000) {
				t.Errorf("%s FY2026-27: the pages are %s under the spine, want exactly "+
					"$250,000 (fisc-av0w)", cat, short)
			}
			continue
		}
		if short != 0 {
			t.Errorf("%s FY2026-27: the pages sum to %s, the spine publishes %s",
				cat, got[1], w[1])
		}
	}
	if len(byCat) != len(want) {
		t.Errorf("the pages print %d object categories, want %d", len(byCat), len(want))
	}
}

// NEITHER FIGURE IN A departmentwideException IS PRINTED, and this is what says
// so rather than leaving it to a comment.
//
// The first review pass over this lane found the type's doc claiming both were
// "read off a page rather than either being derived", copied from
// fundingSourcesException where it is true. It is not true here: this corpus
// prints no citywide object-category total. That claim reached a string
// `fisc verify` publishes on every run, which is the published-versus-derived
// invariant broken in published text.
//
// So the entry is grounded on its DIFFERENCE instead, and both halves of that
// arrangement are asserted here: the figures are absent from the extracted
// pages, and the difference equals a funding-sources exception whose figures are
// present.
func TestDepartmentwideExceptionFiguresAreNotPrintedAndTheirDifferenceIs(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	docIDs := make([]string, 0, len(s.Docs))
	for id := range s.Docs {
		docIDs = append(docIDs, id)
	}
	sort.Strings(docIDs)
	if len(docIDs) < 2 {
		t.Fatalf("the corpus holds %d extracted document(s); a claim about what THIS CORPUS "+
			"prints cannot be made from one", len(docIDs))
	}

	if len(departmentwideExceptions) == 0 {
		t.Fatal("no departmentwide exception is declared, so this test asserts nothing")
	}

	// THE SCAN COUNTS THE PAGES IT READ. An earlier draft looped to a hardcoded
	// 268 and skipped a page it could not open, so the absence assertion below
	// was vacuously true against an unreadable corpus.
	//
	// IT READS EVERY DOCUMENT THE SUBJECT LOADS AND NOT JUST THE BUDGET BOOK. The
	// string it substantiates says this corpus prints no citywide
	// object-category total, and a scan of one document cannot say that.
	//
	// "Every document the subject loads" is TWO of the three that are extracted:
	// the budget book at 268 pages and the ACFR at 195. The CIP is extracted and
	// is not loaded here, because no rule file maps it, so this test cannot
	// speak for its 323 pages. That is a real limit on the claim rather than a
	// gap in the loop, and it is why the assertion below counts pages rather
	// than naming a corpus size.
	scanned := 0
	for _, e := range departmentwideExceptions {
		for _, c := range []amount.Cents{e.spineCents, e.detailCents} {
			// %s renders cents as a grouped dollar figure, which is how the
			// pages print money.
			needle := strings.TrimSuffix(strings.TrimPrefix(c.String(), "$"), ".00")
			for _, id := range docIDs {
				d := s.Docs[id]
				for page := 1; page <= d.PageCount(); page++ {
					text, err := d.Page(page)
					if err != nil {
						t.Fatalf("page %d of %s: %v", page, id, err)
					}
					scanned++
					if strings.Contains(text, needle) {
						t.Errorf("exception %s carries %s and %s p%04d PRINTS it; if a page "+
							"really does publish this figure the entry should assert it "+
							"directly, and the type comment saying no page does is now wrong",
							e.key(), c, id, page)
						break
					}
				}
			}
		}

		want, n := printedDiscrepancy(e.year, e.basis)
		if n != 1 {
			t.Errorf("exception %s is grounded by %d funding-sources exceptions, want exactly "+
				"1; none means it has lost its grounding, more than one means the column is "+
				"ambiguous and the two have opposite repairs", e.key(), n)
			continue
		}
		if e.discrepancy() != want {
			t.Errorf("exception %s holds %s apart and funding-sources-tie-to-spine holds %s; "+
				"they are the same discrepancy on two axes and must agree",
				e.key(), e.discrepancy(), want)
		}
	}

	// EXACT RATHER THAN A FLOOR, and computed from the documents rather than
	// typed: two figures per entry over every page of every document loaded. A
	// floor would let a scan that stopped early pass, and a typed page count
	// would go stale the day a document is added -- which is the same defect as
	// the hardcoded 268 this replaces.
	want := 0
	for _, id := range docIDs {
		want += s.Docs[id].PageCount()
	}
	want *= 2 * len(departmentwideExceptions)
	if scanned != want {
		t.Errorf("the absence scan read %d pages, want %d -- two figures per exception over "+
			"every page of %d document(s); a scan that stopped early proves less than the "+
			"string it substantiates claims", scanned, want, len(docIDs))
	}
}

// EVERY SCOPE IN THE COMMITTED STORE MUST BE PAIRED WITH THE SPINE, one way or
// the other.
//
// This is the general form of the second finding the first review pass made:
// this lane added a scope and did not add it to reconciledScopes, so a
// projection co-selecting it and the spine would have fallen to
// projection-scopes-are-disjoint's weakest branch -- which ADVISES adding the
// pair to disjointScopes, and following that advice doubles the city's
// expenditure with every check green.
//
// Pinning the one missing pair would not have caught the next one. Reading the
// scopes off the committed facts does: a scope enters the store and this goes
// red until someone says which of the two things it is.
//
// ONE SCOPE IS NAMED AS AN OPEN GAP RATHER THAN DECLARED, and how that came
// about is the reason it is written this way. This test first reported
// acfr-general-fund-summary as undeclared, correctly, and the fix was to declare
// the pair disjoint because the two share no cell key. Measured with sharedKeys
// that is true -- zero shared (kind, category, fund_group, fund, fiscal year,
// basis) addresses. Measured with the key internal/project's netCells actually
// merges on, (kind, category, fund_group), they share FIFTEEN. So the
// declaration asserted a safety that does not hold, and it was WORSE than the
// gap it replaced: ARM 3 stops reporting a pair once it is declared. It is
// reverted, the pair is listed here, and fisc-tlbp owns the mismatch.
//
// The list is a declaration in the sense internal/check/vacuity.go means: an
// entry costs a bead and a reason, and an entry that stops being needed goes red
// rather than sitting inert.
//
// THE SPINE PAIRS ONLY, AND THAT BOUNDARY IS DELIBERATE. Measured over the seven
// scopes the store carries: 21 pairs, 6 declared, 15 not. Several of the 15 are
// wrong in the way fisc-tlbp is about -- department-funding-sources and
// departmentwide-expenditures are the two blocks of the same eleven pages and
// are the same money, yet share zero sharedKeys addresses, so ARM 2 would
// confirm a `disjoint` declaration on them. Ten of the 15 predate the
// departmentwide lane. Closing them is a decision about what disjointScopes
// asserts rather than fifteen entries typed at the bottom of it, so this test
// covers the pairs where a doubling reaches the PUBLISHED SPINE and fisc-tlbp
// owns the rest. It is named for that boundary rather than quietly stopping at
// it.
var undeclaredScopePairs = map[string]string{
	acfrGeneralFundScope: "fisc-tlbp: the two disagree under one key and agree under the " +
		"other, so neither `reconciled` nor `disjoint` is honestly assertable until that " +
		"bead decides which claim disjointScopes makes",
}

func TestEveryCommittedScopeIsPairedWithTheSpine(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	scopes := map[string]bool{}
	for i := range s.Facts {
		if sc := s.Facts[i].Scope; sc != project.PublishedScope {
			scopes[sc] = true
		}
	}
	if len(scopes) == 0 {
		t.Fatal("the committed store holds no non-spine scope, so this test asserts nothing")
	}

	for sc := range scopes {
		pair := pairOf(project.PublishedScope, sc)
		_, reconciled := reconciledScopes[pair]
		_, disjoint := disjointScopes[pair]
		reason, known := undeclaredScopePairs[sc]

		if known && (reconciled || disjoint) {
			t.Errorf("scope %q is listed in undeclaredScopePairs (%s) AND declared against "+
				"the spine; delete the entry now that the pair is settled", sc, reason)
			continue
		}
		switch {
		case reconciled && disjoint:
			t.Errorf("scope %q is declared BOTH reconciled with the spine and disjoint from "+
				"it; those are opposite claims about the same money", sc)
		case known:
			// A named gap, not a pass. Nothing to assert beyond the entry
			// existing, which the arm above holds to being still needed.
		case !reconciled && !disjoint:
			t.Errorf("scope %q is in the committed store and is paired with the spine in "+
				"neither reconciledScopes nor disjointScopes, and is not a named gap. A "+
				"projection selecting both falls to projection-scopes-are-disjoint's "+
				"undeclared branch, whose advice is to declare them disjoint -- which is "+
				"the doubling every detail scope here exists to prevent. Declare it, or "+
				"add it to undeclaredScopePairs with the bead that will", sc)
		}
	}

	// An entry naming a scope the store no longer carries reconciles nothing
	// while looking like a tracked gap.
	for sc := range undeclaredScopePairs {
		if !scopes[sc] {
			t.Errorf("undeclaredScopePairs names %q and the committed store has no such "+
				"scope; delete the entry", sc)
		}
	}
}

// THE GROUNDING ARM, DRIVEN THROUGH Run.
//
// TestDepartmentwideExceptionFiguresAreNotPrintedAndTheirDifferenceIs calls
// printedDiscrepancy directly, which tests the helper and NOT the check's use of
// it. Measured: deleting all three grounding branches out of Run left
// `go test ./internal/check/...` entirely green. That is the shape AGENTS.md
// names -- green because the gate fired, not because the defect was prevented --
// arriving in a guard added by a review pass to close a review finding, which is
// the shape the same file names one paragraph later.
//
// So this drives the arm the way the report does: it perturbs
// fundingSourcesExceptions, the table that grounds the entry, and asserts Run
// reports it. Deleting any one branch makes the matching subtest fail.
func TestTheDepartmentwideGroundingArmCanFail(t *testing.T) {
	base, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(departmentwideExceptions) == 0 {
		t.Fatal("no departmentwide exception is declared, so no grounding arm can run")
	}

	// swapGrounding replaces the table this check grounds against for the length
	// of one subtest. The exception table under test is left alone: what is
	// being proved is that the check notices when its GROUND moves.
	swapGrounding := func(t *testing.T, with []fundingSourcesException) {
		t.Helper()
		was := fundingSourcesExceptions
		fundingSourcesExceptions = with
		t.Cleanup(func() { fundingSourcesExceptions = was })
	}
	run := func(t *testing.T) Result {
		t.Helper()
		res, err := (&departmentwideTiesToSpine{}).Run(t.Context(), base)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return res
	}
	saysOneOf := func(t *testing.T, res Result, want string) {
		t.Helper()
		for _, f := range res.Findings {
			if strings.Contains(f.Detail, want) {
				return
			}
		}
		t.Errorf("no finding says %q; got %d finding(s): %s",
			want, len(res.Findings), res.Summary)
	}

	// The committed tables pass, or nothing below means anything.
	if res := run(t); res.Status != StatusPass {
		t.Fatalf("the committed corpus does not pass: %s", res.Summary)
	}

	t.Run("the ground is deleted", func(t *testing.T) {
		swapGrounding(t, nil)
		res := run(t)
		if res.Status != StatusFail {
			t.Errorf("removing every funding-sources exception left the check %s, so an "+
				"entry whose only grounding is gone goes on being reported as reconciled: %s",
				res.Status, res.Summary)
		}
		saysOneOf(t, res, "no funding-sources exception names this column any more")
	})

	t.Run("the ground is ambiguous", func(t *testing.T) {
		e := fundingSourcesExceptions[0]
		second := e
		second.fundGroup = "probe"
		swapGrounding(t, []fundingSourcesException{e, second})
		res := run(t)
		if res.Status != StatusFail {
			t.Errorf("two funding-sources exceptions in one column left the check %s, so "+
				"the entry is grounded on an arbitrary one of them: %s",
				res.Status, res.Summary)
		}
		// The advice must be the OPPOSITE of the deleted case. Getting this
		// backwards is what the third review pass found.
		saysOneOf(t, res, "Do NOT delete this entry")
	})

	// THE TWO REGRESSION PINS, which are what stop the exception hiding a later
	// mapping error. Pass four proved the GROUNDING arm and left these two: with
	// either deleted, `go test ./internal/check/...` stayed green.
	//
	// They are driven by moving the FACTS rather than the table, because that is
	// the direction a real defect arrives from -- a rule re-read, a division
	// dropped -- and it is the direction the pins exist to catch.
	runFacts := func(t *testing.T, mutate func(f *fact.Fact) bool) Result {
		t.Helper()
		s := *base
		s.Facts = append([]fact.Fact(nil), base.Facts...)
		hits := 0
		for i := range s.Facts {
			if mutate(&s.Facts[i]) {
				hits++
			}
		}
		if hits == 0 {
			t.Fatal("the mutation matched no fact, so this subtest proves nothing")
		}
		res, err := (&departmentwideTiesToSpine{}).Run(t.Context(), &s)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		return res
	}

	t.Run("the detail side moves off its pin", func(t *testing.T) {
		res := runFacts(t, func(f *fact.Fact) bool {
			if f.Scope != departmentwideScope || f.FiscalYear != 2027 ||
				f.Category != "services-and-supplies" {
				return false
			}
			f.AmountCents += 100
			return true
		})
		if res.Status != StatusFail {
			t.Errorf("moving the exception's own side by a dollar left the check %s, so the "+
				"held-apart cell absorbs a mapping error instead of reporting it: %s",
				res.Status, res.Summary)
		}
		saysOneOf(t, res, "what pp.85-125's rows sum to in this category")
	})

	t.Run("the spine cell is corrected", func(t *testing.T) {
		// If p0067 were reissued and the corpus republished, the cell would tie
		// on its own and this entry must be DELETED rather than re-pointed.
		res := runFacts(t, func(f *fact.Fact) bool {
			if f.Scope != spineScope || f.Kind != mapping.KindExpenditure ||
				f.FiscalYear != 2027 || f.Category != "services-and-supplies" ||
				f.FundGroup != "internal-service" {
				return false
			}
			f.AmountCents -= 25000000
			return true
		})
		if res.Status != StatusFail {
			t.Errorf("correcting the spine cell left the check %s, so a stale exception "+
				"survives its own retirement: %s", res.Status, res.Summary)
		}
		saysOneOf(t, res, "delete this exception")
	})

	t.Run("the ground moves", func(t *testing.T) {
		e := fundingSourcesExceptions[0]
		e.printedCents += 100
		swapGrounding(t, []fundingSourcesException{e})
		res := run(t)
		if res.Status != StatusFail {
			t.Errorf("re-pointing the funding-sources exception by a dollar left the check "+
				"%s, so the two axes can drift apart: %s", res.Status, res.Summary)
		}
		saysOneOf(t, res, "the same discrepancy seen on two axes")
	})
}
