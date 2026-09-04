package check

import (
	"context"
	"errors"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// factsSorted asserts the committed fact store is in canonical order.
type factsSorted struct{}

var _ Check = (*factsSorted)(nil)

func (*factsSorted) ID() string { return "facts-sorted" }
func (*factsSorted) Tier() int  { return 1 }
func (*factsSorted) Full() bool { return false }
func (*factsSorted) Description() string {
	return "facts.jsonl is in the canonical order, so a rebuild diffs against it line by line"
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

var _ Check = (*factIDsUnique)(nil)

func (*factIDsUnique) ID() string { return "fact-ids-unique" }
func (*factIDsUnique) Tier() int  { return 1 }
func (*factIDsUnique) Full() bool { return false }
func (*factIDsUnique) Description() string {
	return "no two facts share an id, which would mean two rules assert a figure for the same cell"
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
// fields produce.
//
// THIS IS THE INVARIANT fisc-28h RESTS ON. That decision hashed row_label's
// PUBLISHED form, mapping.Row.PrintedLabel(), rather than the resolver's
// internal Identity(), and the deciding argument was exactly this: a reader
// holding one line of the audit trail must be able to re-derive its id from
// that line. Identity() carries a \x1f of its own inside a tuple joined on
// \x1f and cannot be recovered from row_label — "A B" could be one anchor or
// two — so under it the id would have addressed something the record does not
// carry.
//
// WHAT IT CATCHES, AND WHAT IT DOES NOT, because the difference is easy to
// overstate. It catches drift between the store and the tuple: a hand-edited
// line, a fact assembled with one field and hashed over another, an id carried
// over from a row that was since re-worded. It does NOT catch MakeID itself
// being re-pointed at a different tuple — both sides would move together and
// recompute cleanly after a rebuild. That case is guarded structurally instead:
// Run below can only pass fields the RECORD publishes, so re-pointing MakeID at
// something unpublished, Row.Identity() being the live temptation, stops
// compiling here and forces the decision into the open rather than letting it
// land silently.
//
// Until this check the property was asserted only over synthetic rows in
// internal/fact's own tests, never over the committed store.
//
// IT IS NOT fact-ids-unique ONE MORE TIME. Uniqueness says no two facts claim
// one cell; this says each fact's id names the cell it actually carries. Both
// hold today and neither implies the other: renaming every row_label in lockstep
// keeps the ids unique and makes all of them wrong.
type factIDsRecompute struct{}

var _ Check = (*factIDsRecompute)(nil)

func (*factIDsRecompute) ID() string { return "fact-ids-recompute" }
func (*factIDsRecompute) Tier() int  { return 1 }
func (*factIDsRecompute) Full() bool { return false }
func (*factIDsRecompute) Description() string {
	return "every fact's id is what fact.MakeID produces from that fact's own published fields, " +
		"so one line of the audit trail re-derives its own address"
}

// Run recomputes over the seven fields the record publishes, and no others.
//
// Reading them off the FACT rather than off the mapping is the whole point. A
// version that re-resolved the rule would be asking whether the builder is
// self-consistent, which it is by construction; this asks whether the published
// line is, which is what an auditor holding facts.jsonl and nothing else can
// check.
func (*factIDsRecompute) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	for _, f := range s.Facts {
		want := fact.MakeID(f.DocID, f.RuleID, f.RowPath, f.RowLabel, f.ColumnPath, f.FiscalYear, f.Basis)
		if f.ID == want {
			continue
		}
		findings = append(findings, finding(f.ID,
			"%s p%d %q: the published fields (rule %q, row_path %q, column_path %q, FY%d %s) "+
				"hash to %s; an id that does not recompute addresses nothing",
			f.DocID, f.Page, f.RowLabel, f.RuleID, f.RowPath, f.ColumnPath,
			f.FiscalYear, f.Basis, want))
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
// says.
//
// This is the only check in the tier that is INDEPENDENT of the code that
// produced the figure. Everything downstream — a link's value, a headline total,
// the counts — is derived by internal/project from the same fact slice this
// package hands it, so those checks compare one function's sum against another's
// over identical input and both move together: 61 amounts perturbed by up to
// $1.2M passed every one of them. fact.Token is the verbatim text the parser read,
// so re-parsing it is a second witness to the figure, and a corrupted
// amount_cents has nowhere to hide.
//
// It needs no PDF. The token travels in the fact, which is the point of carrying
// it: an auditor comparing amount_cents against the document needs to see what
// the parser actually read, not a re-rendering of what it concluded.
type factTokenReparses struct{}

var _ Check = (*factTokenReparses)(nil)

func (*factTokenReparses) ID() string { return "fact-token-reparses" }
func (*factTokenReparses) Tier() int  { return 1 }
func (*factTokenReparses) Full() bool { return false }
func (*factTokenReparses) Description() string {
	return "every fact's amount_cents is what amount.Parse makes of its own token at its own units"
}

// Run compares Parse(token, units) against amount_cents directly, with no
// reference to Sign.
//
// That is deliberate and it is the thing to get right here. fact.Fact's doc
// comment is explicit that amount_cents is the figure AS THE DOCUMENT PRINTED IT
// and that nothing re-signs it between the parser and the record: a contra row
// the city parenthesizes, Budget Book p127's ERAF "(15,857,875)", arrives already
// negative and carries sign: contra as a statement about how the row relates to
// its category, not as an instruction to negate. So a contra fact must re-parse to
// exactly the negative value it carries, and folding Sign into this comparison
// would make the check reject correct facts — and, worse, accept a positive
// amount_cents on a parenthesized token.
//
// An empty token fails rather than passing or being skipped. amount.Parse reports
// ErrAbsent for it, and absent is not zero: a fact is one figure the city printed,
// so a fact with no token has no printed figure behind it, cannot be re-derived,
// and cannot be cited. No fact in the store has one today, and this is the answer
// if one ever does.
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
// claims.
//
// This is the project's central claim reduced to one comparison. A citation that
// names a page sends a reader to a page of numbers; the offset is what puts them
// on the figure, and if it is wrong the citation still resolves — to the wrong
// cell, silently. internal/fact's own test makes this assertion over a
// hand-written fixture; this makes it over the whole committed store, against the
// extraction the facts were built from.
//
// It reads page text out of Subject.Docs, so it needs the committed extraction
// and still no PDF and no Python.
type factOffsetPointsAtToken struct{}

var _ Check = (*factOffsetPointsAtToken)(nil)

func (*factOffsetPointsAtToken) ID() string { return "fact-offset-points-at-token" }
func (*factOffsetPointsAtToken) Tier() int  { return 1 }
func (*factOffsetPointsAtToken) Full() bool { return false }
func (*factOffsetPointsAtToken) Description() string {
	return "the extracted page text at every fact's offset is that fact's token"
}

// Run bounds-checks before it slices: a fact whose offset runs off the end of its
// page is exactly the failure this check is for, and it must be reported rather
// than panicked.
//
// Pages are cached here rather than on the Subject because corpus.Doc reads a page
// on every call and this check asks for the same two pages 240 times.
func (*factOffsetPointsAtToken) Run(_ context.Context, s *Subject) (Result, error) {
	pages := newPageCache(s.Docs)
	var findings []Finding

	for _, f := range s.Facts {
		text, err := pages.text(f.DocID, f.Page)
		if err != nil {
			findings = append(findings, finding(f.ID, "%v", err))
			continue
		}
		// Compared as a remaining-length subtraction rather than as
		// f.Offset+len(f.Token) > len(text), because the offset comes out of a
		// file a hostile or corrupted writer controls: MaxInt64 plus a token
		// length wraps negative, passes the bound, and panics in the slice
		// below. Subtraction cannot overflow once the offset is known to be
		// within the text.
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
			len(s.Facts), pages.read, plural(pages.read, "page", "pages")),
		nothing:  "the fact store is empty",
		findings: findings,
	}.result(), nil
}

// factCitationsAreDeclared asserts that where two facts cite one printed figure,
// they are a row and its declared counterpart.
//
// ONE PRINTED FIGURE IS NORMALLY ONE FACT, and the exception is declared rather
// than inferred. Budget Book p76 prints "Transfer From Low Income Hsng to
// General Fund 257,012", which is one movement with two ends: money leaving fund
// 200 and the same money arriving at fund 100. fact.Fact carries one fund, so the
// pair needs two facts, and both cite the same doc_id, page, offset and token
// because one printed figure IS the provenance for both directions. Row.Counterpart
// is where a rule says so.
//
// WHAT IT CATCHES is the hazard fisc-2x7y measured on ACFR p41, the first page in
// this corpus read by two rules over one section anchor. Give one of the rows the
// second rule declares `skip: true` a category and a matching kind instead, and
// the store publishes that page's single printed 0.53 twice -- 106,000,000 cents
// of ACFR transfers where the page prints 53,000,000. Every other check is
// satisfied: fact.MakeID hashes rule_id so the ids differ and fact-ids-unique
// holds; both re-parse their own token and both offsets land on it, because it IS
// the same printed figure; no detail-ties-to-spine check spans that scope and none
// can, since it is FY2025 audited against a spine printing no audited column; and
// projection-scopes-are-disjoint compares scopes, while both facts are in one.
//
// SO THE DISCRIMINATOR CANNOT BE THE ADDRESS ALONE. A rule refusing every shared
// (doc_id, page, offset) would redden the 44 addresses p76 legitimately shares.
// What separates them is that the p76 pair comes from ONE rule and ONE row, and
// that row declares a counterpart; the ACFR hazard is two different rules.
//
// The row is found by its printed label, which is what the fact publishes. Where a
// rule prints the same label on two rows, one of them declaring a counterpart is
// enough to satisfy this -- the alternative is to make row_label unique, which the
// documents do not oblige.
type factCitationsAreDeclared struct{}

var _ Check = (*factCitationsAreDeclared)(nil)

func (*factCitationsAreDeclared) ID() string { return "fact-citations-are-declared" }
func (*factCitationsAreDeclared) Tier() int  { return 1 }
func (*factCitationsAreDeclared) Full() bool { return false }
func (*factCitationsAreDeclared) Description() string {
	return "two facts cite one printed figure only as a row and its declared counterpart"
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
			len(s.Facts), len(order), declared, plural(declared, "figure", "figures")),
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
// points the other way says so.
//
// TRANSFER IS THE ONE KIND WHOSE MEANING IS A DIRECTION, which is why this is
// restricted to it rather than being a rule about negative amounts. A negative
// revenue or fund balance is a magnitude: Budget Book p127 prints "Prior Year -
// Unsecured (20,033)", an ordinary revenue row that was negative that year, and
// six spine fund_balance facts are negative because a balance can fall. Neither
// changes what the row MEANS. A negative transfer_out does: the kind already
// says which way the money goes, so a figure pointing the other way is the
// document expressing the same direction with the opposite orientation, and
// that is indistinguishable from money actually flowing back.
//
// WHAT IT CATCHES is fisc-fdxx. ACFR p41 prints Transfers (out) as (25.72)
// because its block sums to a net Other Financing Sources (Uses); Budget Book
// p66 prints TRANSFER OUT as a positive magnitude in a uses column. Both are
// published exactly as printed -- that is the invariant and neither document is
// misread -- so summing transfer_out across the two cancels rather than
// accumulates. Before SignNetted, Fact.Sign read "positive" under BOTH
// conventions, so a consumer had no way to tell and nothing in this package
// could see it: the two scopes share no key, no detail-ties-to-spine check
// spans them and none can, and fact-kind-matches-category is satisfied by both.
//
// WHAT IT DOES NOT DO, because the gap should be stated rather than discovered.
// It makes the convention LEGIBLE; it does not stop a consumer summing across
// conventions. Nothing does today, and nothing needs to: no projection selects
// acfr-general-fund-summary. When one does, the check that refuses the mixture
// can read this field, which it could not have done before.
type factTransferOrientationIsDeclared struct{}

var _ Check = (*factTransferOrientationIsDeclared)(nil)

func (*factTransferOrientationIsDeclared) ID() string { return "fact-transfer-orientation-is-declared" }
func (*factTransferOrientationIsDeclared) Tier() int  { return 1 }
func (*factTransferOrientationIsDeclared) Full() bool { return false }
func (*factTransferOrientationIsDeclared) Description() string {
	return "a transfer printed against its kind's direction declares sign: netted, and one declaring it is so printed"
}

func (*factTransferOrientationIsDeclared) Run(_ context.Context, s *Subject) (Result, error) {
	var findings []Finding
	transfers, netted := 0, 0
	for _, f := range s.Facts {
		if f.Kind != mapping.KindTransferIn && f.Kind != mapping.KindTransferOut {
			continue
		}
		transfers++
		switch {
		case f.AmountCents < 0 && f.Sign != mapping.SignNetted:
			findings = append(findings, finding(f.ID,
				"%s p%d prints %q for row %q, a %s of %d cents, and the row declares sign %q; "+
					"a transfer against its kind's direction must declare %q or a consumer cannot "+
					"tell it from money flowing the other way",
				f.DocID, f.Page, f.Token, f.RowLabel, f.Kind, f.AmountCents, f.Sign, mapping.SignNetted))
		case f.AmountCents >= 0 && f.Sign == mapping.SignNetted:
			findings = append(findings, finding(f.ID,
				"%s p%d prints %q for row %q as %d cents, which already runs with its kind %s, "+
					"but the row declares sign %q",
				f.DocID, f.Page, f.Token, f.RowLabel, f.AmountCents, f.Kind, mapping.SignNetted))
		case f.Sign == mapping.SignNetted:
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
