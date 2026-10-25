package check

import (
	"context"
	"errors"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// factsSorted asserts the committed fact store is in canonical order.
type factsSorted struct{}

func (*factsSorted) meta() meta {
	return meta{id: "facts-sorted", tier: 1,
		description: "facts.jsonl is in the canonical order, so a rebuild diffs against it line by line"}
}

// Run defers to fact.CheckSorted rather than restating the order.
//
// The order is defined once, next to the Sort that produces it, and this check
// exists to assert it about the FILE rather than about a slice in memory: `fisc
// build` sorts and then checks, so a fact store that has been hand-edited, or
// written by a fisc whose order differed, is caught here and nowhere else.
func (*factsSorted) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	if err := fact.CheckSorted(s.Facts); err != nil {
		findings = append(findings, finding(factsFile, "%v", err))
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held:     fmt.Sprintf("%d facts, in canonical order", len(s.Facts)),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factIDsUnique asserts no two facts claim the same identity.
type factIDsUnique struct{}

func (*factIDsUnique) meta() meta {
	return meta{id: "fact-ids-unique", tier: 1,
		description: "no two facts share an id, which would mean two rules assert a figure for the same cell"}
}

// Run defers to fact.CheckUniqueIDs, which also explains what a collision means:
// never a hash accident to work around, always two rules claiming one cell.
func (*factIDsUnique) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	if err := fact.CheckUniqueIDs(s.Facts); err != nil {
		findings = append(findings, finding(factsFile, "%v", err))
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held:     fmt.Sprintf("%d facts, no id claimed twice", len(s.Facts)),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factIDsRecompute asserts every published id is the id its own published
// fields produce, so one line of the audit trail re-derives its own id
// (fisc-28h).
//
// It catches drift between the store and the tuple. It cannot catch MakeID
// re-pointed at a different tuple, since both sides move together; that is
// guarded structurally: Run passes only fields the record publishes, so
// re-pointing MakeID at Row.Identity() stops compiling here. Not implied by
// fact-ids-unique: renaming every row_label in lockstep keeps ids unique and
// makes all of them wrong.
type factIDsRecompute struct{}

func (*factIDsRecompute) meta() meta {
	return meta{id: "fact-ids-recompute", tier: 1,
		description: "every fact's id is what fact.MakeIDThrough produces from that fact's own published fields, " +
			"its band end included where one is set, so one line of the audit trail re-derives its own address"}
}

// Run recomputes over the seven fields the record publishes, and the band end
// where the record sets one, read off the fact rather than the mapping.
func (*factIDsRecompute) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	for _, f := range s.Facts {
		want := fact.MakeIDThrough(f.DocID, f.RuleID, f.RowPath, f.RowLabel, f.ColumnPath, f.FiscalYear, f.FiscalYearThrough, f.Basis)
		if f.ID == want {
			continue
		}
		findings = append(findings, finding(f.ID,
			"%s p%d %q: the published fields (rule %q, row_path %q, column_path %q, %s) "+
				"hash to %s; an id that does not recompute addresses nothing",
			f.DocID, f.Page, f.RowLabel, f.RuleID, f.RowPath, f.ColumnPath,
			fact.BandLabel(f.FiscalYear, fact.BandOf(&f), f.Basis), want))
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held: fmt.Sprintf("%d facts, each id recomputed from the seven fields the record publishes",
			len(s.Facts)),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factTokenReparses asserts every published amount is what its own source text
// says: fact.Token is the verbatim text the parser read, so this witnesses the
// figure independently of the code that produced it.
type factTokenReparses struct{}

func (*factTokenReparses) meta() meta {
	return meta{id: "fact-token-reparses", tier: 1,
		description: "every fact's amount_cents is what amount.Parse makes of its own token at its own units"}
}

// Run compares Parse(token, units) against amount_cents directly, with no
// reference to Sign: amount_cents is the figure as printed, so a parenthesized
// contra row must re-parse to the negative it carries. Folding Sign in would
// accept a positive amount_cents on a parenthesized token.
//
// An empty token fails: absent is not zero, and a fact with no token has no
// printed figure behind it.
func (*factTokenReparses) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	for _, f := range s.Facts {
		got, err := amount.Parse(f.Token, f.Units)
		switch {
		case errors.Is(err, amount.ErrAbsent):
			findings = append(findings, finding(f.ID,
				"%s p%d %q carries amount_cents %s and an empty token, so nothing printed "+
					"stands behind the figure (absent is not zero)",
				f.DocID, f.Page, f.RowLabel, amount.Cents(f.AmountCents)))
		case err != nil:
			findings = append(findings, finding(f.ID,
				"%s p%d %q: token %q no longer parses at %s units: %v",
				f.DocID, f.Page, f.RowLabel, f.Token, f.Units, err))
		case int64(got) != f.AmountCents:
			findings = append(findings, finding(f.ID,
				"%s p%d %q: token %q is %s at %s units but the fact carries %s (off by %s)",
				f.DocID, f.Page, f.RowLabel, f.Token, got, f.Units,
				amount.Cents(f.AmountCents), amount.Cents(f.AmountCents-int64(got))))
		}
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held:     fmt.Sprintf("%d facts, each amount re-derived from its own token", len(s.Facts)),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factOffsetPointsAtToken asserts every provenance pointer points at the thing it
// claims. A wrong offset still resolves, to the wrong cell, silently.
type factOffsetPointsAtToken struct{}

func (*factOffsetPointsAtToken) meta() meta {
	return meta{id: "fact-offset-points-at-token", tier: 1,
		description: "the extracted page text at every fact's offset is that fact's token"}
}

// Run bounds-checks before it slices, so an offset off the end of its page is
// reported rather than panicked. Pages are cached because corpus.Doc reads a
// page on every call.
func (*factOffsetPointsAtToken) Run(_ context.Context, s *Subject) (Result, error) {
	pages := newPageCache(s.Docs)
	var findings []Finding

	for _, f := range s.Facts {
		text, err := pages.text(f.DocID, f.Page)
		if err != nil {
			findings = append(findings, finding(f.ID, "%v", err))
			continue
		}
		// A subtraction, not f.Offset+len(f.Token): a corrupted MaxInt64
		// offset would wrap negative, pass the bound and panic below.
		if f.Offset < 0 || f.Offset > len(text) || len(f.Token) > len(text)-f.Offset {
			findings = append(findings, finding(f.ID,
				"offset %d plus a %d-byte token runs past the end of %s p%d, which is %d bytes",
				f.Offset, len(f.Token), f.DocID, f.Page, len(text)))
			continue
		}
		if got := text[f.Offset : f.Offset+len(f.Token)]; got != f.Token {
			findings = append(findings, finding(f.ID,
				"%s p%d at offset %d is %q, but the fact cites token %q for row %q",
				f.DocID, f.Page, f.Offset, got, f.Token, f.RowLabel))
		}
	}
	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held: fmt.Sprintf("%d facts, each offset landing on its own token in %d %s",
			len(s.Facts), pages.read, english.Plural(pages.read, "page", "pages")),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factCitationsAreDeclared asserts that where two facts cite one printed figure,
// they are a row and its declared counterpart (Row.Counterpart): one printed
// transfer with two ends, as on Budget Book p76, is two facts at one address.
//
// It catches two rules publishing one printed figure (fisc-2x7y, ACFR p41),
// which every other check passes. The address alone cannot discriminate, since
// p76 shares addresses legitimately; one rule and one row declaring a
// counterpart does. The row is found by printed label, unambiguous because
// validateRule refuses two rows of one rule printing the same label.
type factCitationsAreDeclared struct{}

func (*factCitationsAreDeclared) meta() meta {
	return meta{id: "fact-citations-are-declared", tier: 1,
		description: "two facts cite one printed figure only as a row and its declared counterpart"}
}

func (*factCitationsAreDeclared) Run(_ context.Context, s *Subject) (Result, error) {
	rules := map[string]*mapping.Rule{}
	for _, f := range s.Files {
		for i := range f.Rules {
			rules[f.Rules[i].ID] = &f.Rules[i]
		}
	}

	type citation struct {
		doc    string
		page   int
		offset int
	}
	// Grouped in the store's own order, and reported in it, so a finding names
	// the same fact twice across runs.
	order := []citation{}
	byCitation := map[citation][]fact.Fact{}
	for _, f := range s.Facts {
		c := citation{f.DocID, f.Page, f.Offset}
		if _, seen := byCitation[c]; !seen {
			order = append(order, c)
		}
		byCitation[c] = append(byCitation[c], f)
	}

	var findings []Finding
	declared := 0
	for _, c := range order {
		group := byCitation[c]
		if len(group) < 2 {
			continue
		}
		where := fmt.Sprintf("%s p%d offset %d", c.doc, c.page, c.offset)
		if len(group) > 2 {
			findings = append(findings, finding(where,
				"%d facts cite this one printed figure; a counterpart pair is two",
				len(group)))
			continue
		}
		a, b := group[0], group[1]
		if a.RuleID != b.RuleID {
			findings = append(findings, finding(where,
				"rules %q and %q both publish token %q here, and a counterpart is "+
					"declared on a ROW, so two rules cannot be one",
				a.RuleID, b.RuleID, a.Token))
			continue
		}
		if a.RowLabel != b.RowLabel {
			findings = append(findings, finding(where,
				"rule %q publishes token %q here for rows %q and %q; a counterpart "+
					"pair is one row's two ends",
				a.RuleID, a.Token, a.RowLabel, b.RowLabel))
			continue
		}
		if a.AmountCents != b.AmountCents {
			findings = append(findings, finding(where,
				"rule %q publishes %d and %d cents for one printed token %q",
				a.RuleID, a.AmountCents, b.AmountCents, a.Token))
			continue
		}
		rule, ok := rules[a.RuleID]
		if !ok {
			findings = append(findings, finding(where,
				"two facts cite this figure under rule %q, which no rule file declares",
				a.RuleID))
			continue
		}
		if !declaresCounterpart(rule, a.RowLabel) {
			findings = append(findings, finding(where,
				"rule %q publishes token %q twice for row %q, which declares no "+
					"counterpart, so one printed figure is published as two facts",
				a.RuleID, a.Token, a.RowLabel))
			continue
		}
		declared++
	}

	return conclusion{
		subjects: len(s.Facts),
		unit:     "facts",
		held: fmt.Sprintf("%d facts over %d printed figures; %d %s shared by a row and its declared counterpart",
			len(s.Facts), len(order), declared, english.Plural(declared, "figure", "figures")),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// declaresCounterpart reports whether any row of rule printing label declares one.
func declaresCounterpart(rule *mapping.Rule, label string) bool {
	for i := range rule.Rows {
		if rule.Rows[i].PrintedLabel() == label && rule.Rows[i].Counterpart != nil {
			return true
		}
	}
	return false
}

// factTransferOrientationIsDeclared asserts a transfer whose printed figure
// points the other way says so. Restricted to transfers because theirs is the
// one kind whose meaning is a direction; a negative revenue or balance is a
// magnitude.
//
// It catches fisc-fdxx: ACFR p41 prints Transfers (out) negative and Budget
// Book p66 positive, both as printed, so summing across them cancels. It makes
// the convention legible; it does not stop a consumer summing across
// conventions.
type factTransferOrientationIsDeclared struct{}

func (*factTransferOrientationIsDeclared) meta() meta {
	return meta{id: "fact-transfer-orientation-is-declared", tier: 1,
		description: "a transfer printed against its kind's direction declares sign: netted, and one declaring it is so printed"}
}

func (*factTransferOrientationIsDeclared) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	transfers, netted := 0, 0
	for _, f := range s.Facts {
		if f.Kind != vocab.KindTransferIn && f.Kind != vocab.KindTransferOut {
			continue
		}
		transfers++
		switch {
		case f.AmountCents < 0 && f.Sign != vocab.SignNetted:
			findings = append(findings, finding(f.ID,
				"%s p%d prints %q for row %q, a %s of %d cents, and the row declares sign %q; "+
					"a transfer against its kind's direction must declare %q or a consumer cannot "+
					"tell it from money flowing the other way",
				f.DocID, f.Page, f.Token, f.RowLabel, f.Kind, f.AmountCents, f.Sign, vocab.SignNetted))
		// > 0 and not >= 0: a netted row's `-` cells publish a zero, and zero
		// runs with every direction, so >= would redden a correctly declared
		// multi-column row.
		case f.AmountCents > 0 && f.Sign == vocab.SignNetted:
			findings = append(findings, finding(f.ID,
				"%s p%d prints %q for row %q as %d cents, which already runs with its kind %s, "+
					"but the row declares sign %q",
				f.DocID, f.Page, f.Token, f.RowLabel, f.AmountCents, f.Kind, vocab.SignNetted))
		case f.AmountCents < 0:
			// Counted here rather than on `Sign == netted`, so the summary says
			// what it means: a zero declares no direction, and counting one as
			// "printed against its kind's direction" would contradict the arm
			// above on a line fisc verify prints every run.
			netted++
		}
	}
	return conclusion{
		subjects: transfers,
		unit:     "transfer facts",
		held: fmt.Sprintf("%d transfer facts, %d printed against their kind's direction and declaring it",
			transfers, netted),
		nothing:  "the fact store carries no transfers",
		findings: findings,
	}.result(), nil
}

// pageCache reads each page of each document at most once.
type pageCache struct {
	docs  map[string]*corpus.Doc
	cache map[pageKey]string
	// read is how many distinct pages were read, for the summary: it is the part
	// of the corpus this check actually opened.
	read int
}

type pageKey struct {
	doc  string
	page int
}

func newPageCache(docs map[string]*corpus.Doc) *pageCache {
	return &pageCache{docs: docs, cache: map[pageKey]string{}}
}

// text returns one page's extracted text, or an error saying why a fact citing it
// cannot be checked.
func (c *pageCache) text(docID string, page int) (string, error) {
	key := pageKey{doc: docID, page: page}
	if t, ok := c.cache[key]; ok {
		return t, nil
	}
	doc, ok := c.docs[docID]
	if !ok {
		// The rule files decide which extractions are opened, and every fact is
		// produced by a rule, so this is a fact whose document no rule file maps:
		// a hand-edited store, or a rule file deleted without a rebuild.
		return "", fmt.Errorf("no extraction is loaded for document %q, so the page this fact "+
			"cites cannot be read; no rule file under %s maps that document", docID, mappingsDir)
	}
	t, err := doc.Page(page)
	if err != nil {
		return "", fmt.Errorf("read the page this fact cites: %w", err)
	}
	c.cache[key] = t
	c.read++
	return t, nil
}
