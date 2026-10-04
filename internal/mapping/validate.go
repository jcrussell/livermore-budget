package mapping

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/hint"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// validateRule refuses a rule that is malformed on its own, without the pages,
// and returns the first refusal it meets: which one a rule carrying several
// defects reports depends on the order of the calls below, and the tests pin
// those messages.
func validateRule(r *Rule, errf errFunc) error {
	if err := validateRuleShape(r, errf); err != nil {
		return err
	}

	// First, because the row checks below read cellPublishes, which reads
	// what this stores.
	if err := resolveRuleOmittedCells(r, errf); err != nil {
		return err
	}

	// The columns' categories and kinds are validated before any row, because
	// the sign check below reads a column-category row's kinds off them.
	byColumn := r.categoryOnColumns()
	if err := validateColumnClasses(r, byColumn, errf); err != nil {
		return err
	}

	names, err := validateRows(r, byColumn, errf)
	if err != nil {
		return err
	}
	if err := validateTotalRowNotARow(r, errf); err != nil {
		return err
	}
	if err := validateParts(r, names, errf); err != nil {
		return err
	}

	for _, check := range []func(*Rule, errFunc) error{
		validateLabelsFrom,
		validateTotalRowAbove,
		validateTotalRowTail,
		validatePrintedDecimals,
		validateTotalRowKinds,
		validateFundLabelling,
		validateRowLabelFunds,
		validateTotalSpansParts,
		validateSubtotals,
		// Last, so a rule is refused for its own shape before a missing grain.
		validateGrain,
	} {
		if err := check(r, errf); err != nil {
			return err
		}
	}
	return nil
}

// validateRuleShape refuses a rule whose kind or basis is not in the
// vocabulary, whose units are missing or unknown, or that names no part or no
// row.
func validateRuleShape(r *Rule, errf errFunc) error {
	if !r.Kind.Valid() {
		return errf(r.ID, "kind", "got %q, want one of %s", r.Kind, vocab.KindList())
	}
	if !r.Basis.Valid() {
		return errf(r.ID, "basis", "got %q, want one of %s", r.Basis, vocab.BasisList())
	}
	if r.Units == "" {
		return hint.With(
			errf(r.ID, "units", "is required"),
			"units come from the table's caption, not the token: \"71.8\" is "+
				"meaningless until you know the schedule is printed in millions")
	}
	if _, _, ok := unitsValid(r.Units); !ok {
		return errf(r.ID, "units", "got %q, want one of dollars, thousands, millions", r.Units)
	}
	if len(r.Parts) == 0 {
		return errf(r.ID, "parts", "is empty; a rule must name at least one page")
	}
	if len(r.Rows) == 0 {
		return errf(r.ID, "rows", "is empty")
	}
	return nil
}

// resolveRuleOmittedCells runs resolveOmittedCells over every part, in order,
// and refuses what it refuses.
func resolveRuleOmittedCells(r *Rule, errf errFunc) error {
	for i := range r.Parts {
		if err := resolveOmittedCells(r, &r.Parts[i], errf); err != nil {
			return err
		}
	}
	return nil
}

// validateColumnClasses runs validateColumnClass over every column of every
// part, in order, and refuses what it refuses.
func validateColumnClasses(r *Rule, byColumn bool, errf errFunc) error {
	for i := range r.Parts {
		for j := range r.Parts[i].Columns {
			if err := validateColumnClass(r, &r.Parts[i], j, byColumn, errf); err != nil {
				return err
			}
		}
	}
	return nil
}

// rowNames is what validateRows learns of a rule's rows and validateParts
// reads back when it holds an omitted_cells entry to naming one of them.
//
// index is keyed on Identity(), which is what a row IS within its rule;
// tailed collects the two-anchor rows by their first anchor alone, so a
// declaration that names only that anchor can be told "which one?" rather
// than "no such row". printed guards the other end: row_label is what a
// fact publishes and what its id is hashed from (see fact.MakeID), so two
// rows may share neither an identity nor a printed form.
//
// Except two SKIPPED rows: neither publishes, so no fact id is hashed from
// either, and rows are matched in the order the rule lists them, so each
// reads its own line. Budget Book p230 prints fund 611's projects twice,
// once under its federal grant and once under its state grant, on lines
// identical but for the figures. An omitted_cells entry naming such a row
// could not say which one, so validatePartOmittedCells refuses it.
type rowNames struct {
	index    map[string]bool
	tailed   map[string][]string
	printed  map[string]bool
	skipped  map[string]bool
	repeated map[string]bool
}

// add refuses a row whose identity an earlier row already has, unless both
// are skipped, and a row that prints as an earlier row does; it then records
// the row.
func (n *rowNames) add(r *Rule, row Row, errf errFunc) error {
	switch {
	case n.index[row.Identity()] && row.Skip && n.skipped[row.Identity()]:
		n.repeated[row.Identity()] = true
	case n.index[row.Identity()]:
		return hint.With(
			errf(r.ID, "rows", "duplicate row label %q", row.PrintedLabel()),
			"row labels are positional identities; two rows cannot share one "+
				"unless both are skip: true")
	case n.printed[row.PrintedLabel()]:
		return hint.With(
			errf(r.ID, "rows", "two rows print as %q", row.PrintedLabel()),
			"the two rows split that text differently between label and "+
				"label_tail, so they are two identities to the resolver "+
				"and one row_label to every reader of facts.jsonl, where "+
				"a fact's id is hashed from row_label")
	}
	if !n.index[row.Identity()] {
		n.skipped[row.Identity()] = row.Skip
	}
	n.index[row.Identity()] = true
	n.printed[row.PrintedLabel()] = true
	if row.LabelTail != "" {
		n.tailed[row.Label] = append(n.tailed[row.Label], row.PrintedLabel())
	}
	return nil
}

// validateRows refuses each row, in the order the rule lists them, for its
// anchors, its identity and its declarations, and returns what it learned of
// the rows' names for validateParts.
func validateRows(r *Rule, byColumn bool, errf errFunc) (*rowNames, error) {
	names := &rowNames{
		index:    map[string]bool{},
		tailed:   map[string][]string{},
		printed:  map[string]bool{},
		skipped:  map[string]bool{},
		repeated: map[string]bool{},
	}
	// A row's page must name exactly one part. Naming none would place the
	// row nowhere, so every part would read the page without it and nothing
	// would hold the count against the row; naming two would place it on both
	// and on neither. Counted here so the refusal is the row's, before
	// validateParts refuses the repeated page as its own defect.
	partsOnPage := map[int]int{}
	for i := range r.Parts {
		partsOnPage[r.Parts[i].Page]++
	}
	for i, row := range r.Rows {
		if err := validateRowAnchors(r, i, row, partsOnPage, errf); err != nil {
			return nil, err
		}
		if err := names.add(r, row, errf); err != nil {
			return nil, err
		}
		if !row.Sign.Valid() {
			return nil, errf(r.ID, "rows", "row %q: sign %q, want positive, contra or netted",
				row.Label, row.Sign)
		}
		if err := validateRowClass(r, row, byColumn, errf); err != nil {
			return nil, err
		}
		if err := validateRowQuantity(r, row, errf); err != nil {
			return nil, err
		}
		publishes := r.kindsOf(row)
		if err := validateUnpublishedRowDeclares(r, row, publishes, errf); err != nil {
			return nil, err
		}
		if err := checkCounterpart(r, row, errf); err != nil {
			return nil, err
		}
		if err := validateNettedSign(r, row, publishes, errf); err != nil {
			return nil, err
		}
	}
	return names, nil
}

// validateRowAnchors refuses a row with no label, a page that names no single
// part of the rule or that places nothing in a rule of one part, and a blank
// or untrimmed label_tail.
func validateRowAnchors(r *Rule, i int, row Row, partsOnPage map[int]int, errf errFunc) error {
	if row.Label == "" {
		return errf(r.ID, "rows", "row %d has no label", i)
	}
	if row.Page != 0 {
		switch n := partsOnPage[row.Page]; {
		case n == 0:
			return hint.With(
				errf(r.ID, "rows", "row %q: page %d is not a part of this rule",
					row.PrintedLabel(), row.Page),
				"page places a row on the one part that prints it; "+
					"omit it on a row every part prints")
		case n > 1:
			return errf(r.ID, "rows", "row %q: page %d is listed twice in parts, "+
				"so it names no single part", row.PrintedLabel(), row.Page)
		case len(r.Parts) == 1:
			return hint.With(
				errf(r.ID, "rows", "row %q: page places nothing in a rule of one part",
					row.PrintedLabel()),
				"every row of a one-part rule is on that part; omit page")
		}
	}
	if row.LabelTail != "" {
		// Blank first: a whitespace-only tail IS blank, and reporting it
		// as stray whitespace would send the author looking for a
		// character to trim rather than for the anchor they meant to name.
		if strings.TrimSpace(row.LabelTail) == "" {
			return errf(r.ID, "rows", "row %q: label_tail is blank", row.Label)
		}
		if strings.TrimSpace(row.LabelTail) != row.LabelTail {
			return errf(r.ID, "rows",
				"row %q: label_tail %q has leading or trailing whitespace",
				row.Label, row.LabelTail)
		}
	}
	return nil
}

// validateRowQuantity refuses a row quantity that declares the default or is
// not one of the quantities.
func validateRowQuantity(r *Rule, row Row, errf errFunc) error {
	if row.Quantity == "" {
		return nil
	}
	if row.Quantity == QuantityAmount {
		return hint.With(
			errf(r.ID, "rows", "row %q: quantity %q is the default",
				row.Label, row.Quantity),
			"an undeclared row already parses amounts; remove the declaration")
	}
	if !row.Quantity.valid() {
		return errf(r.ID, "rows", "row %q: quantity %q, want one of %s",
			row.Label, row.Quantity, quantityList())
	}
	return nil
}

// validateUnpublishedRowDeclares is the one refusal for a sign or a
// counterpart on a row that publishes no cell, held to kindsOf: the kinds of
// the cells cellPublishes admits. It runs above checkCounterpart, because a
// counterpart's own defects are moot on a row that should not carry one.
func validateUnpublishedRowDeclares(r *Rule, row Row, publishes []vocab.Kind, errf errFunc) error {
	if len(publishes) != 0 {
		return nil
	}
	var declared string
	switch {
	case row.Sign != "":
		declared = "sign " + string(row.Sign)
	case row.Counterpart != nil:
		declared = "counterpart"
	}
	if declared != "" {
		return hint.With(
			errf(r.ID, "rows", "row %q: %s on a row that publishes no cell",
				row.Label, declared),
			"a sign says how a row's facts are printed and a counterpart "+
				"fans one into two; this row publishes no cell, so remove "+
				"the declaration")
	}
	return nil
}

// validateNettedSign refuses sign netted on a row either end of which is not
// a transfer.
//
// SignNetted says a row is printed against its KIND's direction, so it
// is meaningless on a kind that has no direction, and
// fact-transfer-orientation-is-declared only witnesses transfers. Left
// unvalidated, `sign: netted` on a revenue row publishes facts nothing
// checks. Refused here rather than widened there: the check is right to
// be narrow, because a negative revenue or fund balance is a magnitude
// and not an orientation.
//
// BOTH ENDS, because fact.FromValues builds the counterpart leg from a
// COPY of this row -- it overrides Category and Kind and inherits
// everything else, Sign included. Inheriting it is right when both ends
// are transfers: one printed figure, one orientation, and a negative
// transfer_in is against its direction exactly as the transfer_out is.
// It is wrong the moment the far end is not a transfer, which is how a
// netted revenue fact reached the store past the near-row check alone.
//
// IT SITS BELOW BOTH KIND VALIDATORS -- row.Kind's above and
// checkCounterpart's, which refuses a counterpart kind that is not one of
// the five. Above either, {sign: netted, kind: income} is reported as a sign
// on kind "income" and a mistyped counterpart kind as `sign netted on kind
// "incom"` -- both naming the wrong field to whoever has to fix the YAML.
func validateNettedSign(r *Rule, row Row, publishes []vocab.Kind, errf errFunc) error {
	if row.Sign != vocab.SignNetted {
		return nil
	}
	ends := publishes
	if row.Counterpart != nil {
		ends = append(ends, row.Counterpart.Kind)
	}
	for _, k := range ends {
		if k == vocab.KindTransferIn || k == vocab.KindTransferOut {
			continue
		}
		return hint.With(
			errf(r.ID, "rows", "row %q: sign netted on kind %q", row.Label, k),
			"netted says the document prints this row against its kind's "+
				"direction, which only transfer_in and transfer_out have; "+
				"a counterpart leg inherits the row's sign")
	}
	return nil
}

// validateTotalRowNotARow refuses a total_row that is also one of the rule's
// rows, by its label or its printed form.
//
// The guard is against the row's LABEL, and deliberately not against
// rowNames.index: a two-anchor row's identity carries a \x1f, so a total_row --
// which is a plain string the author types and the resolver finds in the
// page text -- can never equal one, and testing the index silently
// disabled this check for every row carrying a label_tail (fisc-gtv). The
// printed form is tested too, because that is the other spelling an author
// might reach for.
func validateTotalRowNotARow(r *Rule, errf errFunc) error {
	if r.TotalRow == "" {
		return nil
	}
	for _, row := range r.Rows {
		if r.TotalRow != row.Label && r.TotalRow != row.PrintedLabel() {
			continue
		}
		return hint.With(
			errf(r.ID, "total_row", "%q is also listed in rows", r.TotalRow),
			"the total row is what the mapped rows are checked against; "+
				"including it in rows would double-count it")
	}
	return nil
}

// validateParts refuses each part, in the order the rule lists them: a part
// with no page or a page listed twice, then whatever its gap declarations,
// columns, block boundaries, anchors, omitted cells and stated-total deltas
// refuse. names is validateRows' record of the rule's rows.
func validateParts(r *Rule, names *rowNames, errf errFunc) error {
	pages := map[int]bool{}
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.Page <= 0 {
			return errf(r.ID, "parts", "part %d has no page", i)
		}
		if pages[p.Page] {
			return errf(r.ID, "parts", "page %d listed twice", p.Page)
		}
		pages[p.Page] = true
		for _, check := range []func(*Rule, *Part, errFunc) error{
			validatePartGaps,
			validatePartColumns,
			validateBlockBounds,
			validatePartAnchors,
		} {
			if err := check(r, p, errf); err != nil {
				return err
			}
		}
		if err := validatePartOmittedCells(r, p, names, errf); err != nil {
			return err
		}
		if err := validateStatedTotalDeltas(r, p, errf); err != nil {
			return err
		}
	}
	return nil
}

// validatePartGaps refuses what a part declares about the lines between its
// rows: wrapped_labels and headings by validateGapLines, a line declared as
// both, and an unmapped_text that is on a labels_from part, blank, untrimmed,
// without a note, not a figure, or listed twice.
func validatePartGaps(r *Rule, p *Part, errf errFunc) error {
	// A wrapped label is matched against the FULLY TRIMMED text of a gap,
	// so a declaration carrying its own whitespace could never match and
	// would fail later as a stale declaration rather than here as the typo
	// it is. Blank and duplicate entries are refused for the same reason.
	//
	// A LABELS_FROM PART CANNOT WRAP A LABEL, because it carries none.
	// Row identity there is positional: the part routes to positionalValues,
	// which never reads this field and never staleness-checks it, so a
	// declaration was ACCEPTED AND INERT -- the one shape a declaration in
	// this repository must not have. Every other one fails when it stops
	// being true: a stated_total_delta that now ties exactly, a placed
	// row's count that no longer matches, a wrapped label the page stopped
	// wrapping. Measured on the production mapping (fisc-ekj): adding a
	// fragment that appears nowhere on p67 built cleanly with byte-identical
	// facts.
	//
	// Refusing is right rather than honouring it. A wrapped label is a
	// property of the part that CARRIES the label, and this part has
	// delegated that to another page -- which is where the declaration
	// belongs.
	//
	// A WRAPPED LABEL IS A LABEL, and the amount refusal is what holds it
	// to that: without it, wrapped_labels: ["0.0"] on ACFR p41's revenue
	// block parses, resolves all ten rows and ties to the printed total
	// exactly, publishing the page's orphan figure under a declaration that
	// the page wrapped a label onto its own line, which it did not.
	// unmapped_text (fisc-hcus) is that figure's honest home, and is worth
	// nothing while the dishonest one still works.
	//
	// Every entry the committed rules declare is refused by amount.Parse,
	// which loading them re-measures, so this costs them nothing.
	if err := validateGapLines(r, p, "wrapped_labels", p.WrappedLabels, errf,
		"a wrapped label is a claim about the page that PRINTS the "+
			"label; declare it on that part, where it is checked",
		"wrapped_labels declares that the page wrapped a row's LABEL "+
			"onto its own line; a figure there belongs to no row and "+
			"is declared with unmapped_text, which says so"); err != nil {
		return err
	}
	// headings shares every arm above, for the same reasons: it is matched
	// the same way, read by the same gap test and staleness-checked the
	// same way.
	if err := validateGapLines(r, p, "headings", p.Headings, errf,
		"a heading is a claim about the page that prints it between "+
			"labelled rows; a label-less part reads no gaps",
		"headings declares a section heading printed between rows; a "+
			"figure there belongs to no row and is declared with "+
			"unmapped_text, which says so"); err != nil {
		return err
	}
	// One line, one claim: a line declared both a heading and a wrapped
	// label would say two incompatible things about what the page printed.
	for _, h := range p.Headings {
		if slices.Contains(p.WrappedLabels, h) {
			return errf(r.ID, fmt.Sprintf("parts[page %d].headings", p.Page),
				"%q is also declared in wrapped_labels; a line is a heading or "+
					"a wrapped label, not both", h)
		}
	}
	// unmapped_text is the mirror of the block above, and every arm is the
	// mirror of one of its arms: same trimmed matching, same blank and
	// duplicate refusals, same labels_from refusal for the same reason
	// (fisc-ekj -- a positional part reads no gaps, so the declaration
	// would be accepted and inert). The one INVERTED arm is amount.Parse:
	// a wrapped label must not be a figure and this must be one.
	if p.LabelsFrom != 0 && len(p.UnmappedText) > 0 {
		return hint.With(
			errf(r.ID, fmt.Sprintf("parts[page %d].unmapped_text", p.Page),
				"is declared on a part whose labels_from takes its row labels from page %d",
				p.LabelsFrom),
			"unmapped_text is checked against the gaps between a part's own "+
				"labelled rows; a label-less part reads none, so the "+
				"declaration would be accepted and never looked at")
	}
	seenUnmapped := map[string]bool{}
	for _, u := range p.UnmappedText {
		field := fmt.Sprintf("parts[page %d].unmapped_text", p.Page)
		if strings.TrimSpace(u.Text) == "" {
			return errf(r.ID, field, "has an entry with no text")
		}
		if strings.TrimSpace(u.Text) != u.Text {
			return errf(r.ID, field,
				"%q has leading or trailing whitespace; it is matched against "+
					"the trimmed text of the gap", u.Text)
		}
		if strings.TrimSpace(u.Note) == "" {
			return hint.With(
				errf(r.ID, field, "%q has no note", u.Text),
				"the note is why this is a declaration and not a silent skip; "+
					"say what the page prints there and why it belongs to no row")
		}
		if _, err := amount.Parse(u.Text, r.Units); err != nil {
			return hint.With(
				errf(r.ID, field, "%q is not a figure: %v", u.Text, err),
				"unmapped_text declares a FIGURE the page prints that belongs "+
					"to no row; text the page wrapped from a row's label is "+
					"declared with wrapped_labels, which says that instead")
		}
		if seenUnmapped[u.Text] {
			return errf(r.ID, field, "%q is listed twice", u.Text)
		}
		seenUnmapped[u.Text] = true
	}
	return nil
}

// validatePartColumns refuses a part with no columns, and a column with an
// unknown basis, a bad quantity by validateColumnQuantity, no fiscal_year, or
// the identity of an earlier column of the part.
//
// Columns are positional identities exactly as rows are, so duplicates
// must be rejected for the same reason. A repeated column is easy to
// write (four columns where only the fiscal year differs, and one
// does not get bumped) and impossible to detect downstream: the column
// COUNT still matches the page, so the value-count assertion passes
// and the second year's figures are emitted as conflicting facts for
// the first.
func validatePartColumns(r *Rule, p *Part, errf errFunc) error {
	if len(p.Columns) == 0 {
		return errf(r.ID, fmt.Sprintf("parts[page %d].columns", p.Page), "is empty")
	}
	cols := map[Column]bool{}
	for j, c := range p.Columns {
		if c.Basis != "" && !c.Basis.Valid() {
			return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d].basis", p.Page, j),
				"got %q", c.Basis)
		}
		if c.Quantity != "" {
			if err := validateColumnQuantity(r, p, j, c, errf); err != nil {
				return err
			}
			// A non-amount column never publishes, so like a skipped one
			// it needs no fiscal_year and no distinct identity.
			continue
		}
		if c.Skip {
			continue
		}
		if c.FiscalYear == 0 {
			return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
				"needs a fiscal_year (or skip: true)")
		}
		// KIND IS NOT IN THE KEY because it is not in the fact id: two
		// columns of one row differing only in kind hash to one id.
		key := c
		key.Kind = ""
		if cols[key] {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
					"duplicates an earlier column (fund_group=%q fund=%d fiscal_year=%d "+
						"basis=%q category=%q)",
					c.FundGroup, c.Fund, c.FiscalYear, c.Basis, c.Category),
				"columns are positional identities; check whether a fiscal_year, "+
					"fund_group or category was left unchanged when the column was "+
					"copied. A column's kind does not tell it apart: a fact's id is "+
					"not hashed over its kind")
		}
		cols[key] = true
	}
	return nil
}

// validateColumnQuantity refuses a column quantity c declares that is the
// default or not one of the quantities, and any non-amount column on a rule
// that declares a total_row.
func validateColumnQuantity(r *Rule, p *Part, j int, c Column, errf errFunc) error {
	if c.Quantity == QuantityAmount {
		return hint.With(
			errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
				"quantity %q is the default", c.Quantity),
			"an undeclared column already parses amounts; remove the declaration")
	}
	if !c.Quantity.valid() {
		return errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
			"quantity %q, want one of %s", c.Quantity, quantityList())
	}
	// A rule's total_row is read as one run of amounts, one per
	// column (see amountRun), and a table whose columns are not
	// all amounts prints no such line. Refused here so the
	// mismatch cannot surface later as a missing-anchor error.
	if r.TotalRow != "" {
		return hint.With(
			errf(r.ID, fmt.Sprintf("parts[page %d].columns[%d]", p.Page, j),
				"parses %s, but the rule declares total_row %q",
				c.Quantity, r.TotalRow),
			"a stated-totals line prints one amount per column; a "+
				"table with a non-amount column has no such line to read")
	}
	return nil
}

// validateBlockBounds refuses a section or stop_at that is a currency amount.
//
// A block boundary must not be a figure the totals check is meant to
// verify: anchoring the block on the number proves nothing, and the
// anchor vanishes the moment the city republishes with a revised
// total. Anchor on a label instead.
func validateBlockBounds(r *Rule, p *Part, errf errFunc) error {
	for _, field := range []struct{ name, val string }{
		{"section", p.Section}, {"stop_at", p.StopAt},
	} {
		if field.val == "" {
			continue
		}
		if _, err := amount.Parse(field.val, r.Units); err == nil {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].%s", p.Page, field.name),
					"%q is a currency amount", field.val),
				"anchor the block on a label such as \"TOTAL REVENUES:\"; "+
					"a figure changes when the document is revised, and using "+
					"a total as the boundary makes the totals check circular")
		}
	}
	return nil
}

// validatePartOmittedCells runs validateOmittedCells over p, holding each
// entry to naming one of the rows names records, and refuses an entry with no
// label, an untrimmed label_tail, or a name that matches no row or several.
//
// An omitted_cells entry must name EXACTLY ONE row. Naming none is a
// stale declaration; naming more than one would blank a cell of every
// row sharing a label, which is the silent mismapping the declaration
// exists to prevent, one row over.
func validatePartOmittedCells(r *Rule, p *Part, names *rowNames, errf errFunc) error {
	namesOneRow := func(field string, j int, o Row, notARow string) error {
		switch {
		case strings.TrimSpace(o.Label) == "":
			return errf(r.ID, field, "entry %d has no label", j)
		case strings.TrimSpace(o.LabelTail) != o.LabelTail:
			return errf(r.ID, field,
				"entry %d: label_tail %q has leading or trailing whitespace",
				j, o.LabelTail)
		}
		if names.repeated[o.Identity()] {
			return errf(r.ID, field, "%q names rows the rule lists more than once, "+
				"so it cannot say which one the page leaves blank", o.PrintedLabel())
		}
		if names.index[o.Identity()] {
			return nil
		}
		if o.LabelTail == "" && len(names.tailed[o.Label]) > 0 {
			return hint.With(
				errf(r.ID, field, "%q names %d rows, which differ only in "+
					"their label_tail: %q", o.Label, len(names.tailed[o.Label]),
					names.tailed[o.Label]),
				"a row named by two anchors is named by both: "+
					"- {label: ..., label_tail: ...}")
		}
		return hint.With(
			errf(r.ID, field, "%q is not one of this rule's rows", o.PrintedLabel()),
			notARow)
	}
	return validateOmittedCells(r, p, namesOneRow, errf)
}

// validateStatedTotalDeltas refuses a stated_total_deltas entry whose column
// is out of range, skipped, non-amount or already given a delta, whose
// delta_cents is zero, or that carries no note.
//
// A declared discrepancy is a claim about one column of one page, so
// every way of writing one that does not say that is refused here
// rather than reaching CheckTotals.
func validateStatedTotalDeltas(r *Rule, p *Part, errf errFunc) error {
	deltas := map[int]bool{}
	for j, d := range p.StatedTotalDeltas {
		field := fmt.Sprintf("parts[page %d].stated_total_deltas[%d]", p.Page, j)
		if d.Column < 1 || d.Column > len(p.Columns) {
			return errf(r.ID, field, "column %d is out of range; this part has %d columns",
				d.Column, len(p.Columns))
		}
		if p.Columns[d.Column-1].Skip {
			return hint.With(
				errf(r.ID, field, "column %d is skipped", d.Column),
				"a skipped column produces no facts and is never totalled, "+
					"so there is nothing for a delta to describe")
		}
		if q := p.Columns[d.Column-1].Quantity; q != "" {
			return hint.With(
				errf(r.ID, field, "column %d parses %s, not amounts", d.Column, q),
				"a non-amount column produces no facts and is never totalled, "+
					"so there is nothing for a delta to describe")
		}
		if deltas[d.Column] {
			return errf(r.ID, field, "column %d already has a delta", d.Column)
		}
		deltas[d.Column] = true
		if d.Cents == 0 {
			return hint.With(
				errf(r.ID, field, "delta_cents is zero"),
				"a zero delta is what an undeclared column already asserts; "+
					"remove the entry")
		}
		if strings.TrimSpace(d.Note) == "" {
			return hint.With(
				errf(r.ID, field, "note is required"),
				"the note is why this is a declaration and not a tolerance: "+
					"say what was checked and why the difference is the "+
					"document's rather than ours")
		}
	}
	return nil
}

// validateLabelsFrom refuses a labels_from that points at its own part, at a
// page that is not a part of the rule, or at a part that is itself a
// continuation.
//
// LabelsFrom must point at another part of the same rule, and that part
// must itself carry labels — a chain of continuations has no anchor.
func validateLabelsFrom(r *Rule, errf errFunc) error {
	for i := range r.Parts {
		p := &r.Parts[i]
		if p.LabelsFrom == 0 {
			continue
		}
		if p.LabelsFrom == p.Page {
			return errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
				"points at itself")
		}
		src := r.labelledPart(p)
		if src == nil {
			return hint.With(
				errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
					"page %d is not a part of this rule", p.LabelsFrom),
				"a continuation page borrows row labels from another page in "+
					"the same rule")
		}
		if src.LabelsFrom != 0 {
			return errf(r.ID, fmt.Sprintf("parts[page %d].labels_from", p.Page),
				"page %d is itself a continuation; labels must come from a page that has them",
				p.LabelsFrom)
		}
	}
	return nil
}

// validateFundLabelling refuses a rule declaring both row_labels_name_funds
// and row_labels_are_fund_numbers.
func validateFundLabelling(r *Rule, errf errFunc) error {
	if r.RowLabelsNameFunds && r.RowLabelsAreFundNumbers {
		return errf(r.ID, "row_labels_are_fund_numbers",
			"is declared with row_labels_name_funds; a label is a fund's name or its number, not both")
	}
	return nil
}
