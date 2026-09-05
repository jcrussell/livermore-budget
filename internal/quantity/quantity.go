// Package quantity recognizes the cell grammars that are not money: the
// percentage, per-unit-dollar and unitless-number cells the ACFR Statistical
// Section prints beside its amounts (fisc-9tn4).
//
// It lives beside internal/amount, never inside it: amount stays strict about
// money, and a cell these grammars recognize is read so its row stays whole
// and is never published as a fact. Like amount, every shape not positively
// recognized is an error — p182's ranges ("5600-6000", and one printed
// "900-100"), p185's "NA" and p186's "exempt" are the tokens this package
// exists to refuse.
//
// None of these grammars takes a units argument, deliberately: p180 declares
// thousands for its one amount column while its per-capita column is plain
// dollars and its population column is a count. Units scale money; these are
// not money.
package quantity

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// figure is a non-negative decimal figure: a grouped or ungrouped integer
// part with an optional fraction. Grouping is enforced for amount.Parse's
// reason — a permissive [0-9,]+ reads "1,234,56", a figure whose decimal
// point extraction lost, as 123456.
var figure = regexp.MustCompile(`^([0-9]+|[0-9]{1,3}(,[0-9]{3})+)(\.[0-9]+)?$`)

// Percentage recognizes a figure with a trailing "%", as p177 prints "2.5%".
// No dash-zero, no parenthesized negative, no bare figure: a percentage
// column printing any of those is a page this grammar has not seen, and it
// fails closed.
func Percentage(s string) error {
	body, ok := strings.CutSuffix(s, "%")
	if !ok {
		return fmt.Errorf("cannot read %q as a percentage: no trailing %%", s)
	}
	if !figure.MatchString(body) {
		return fmt.Errorf("cannot read %q as a percentage: %q is not a figure", s, body)
	}
	return nil
}

// AmountPerUnit recognizes a dollar-shaped cell whose unit is per-something —
// per capita, per CCF, per meter size. It is amount.Parse at dollars, because
// the token IS well-formed money; what the quantity carries is the thing the
// money grammar cannot: that the figure is not city money and must not enter
// a store whose invariant is integer cents of it. p177's "$ 1,009" per capita
// is the hazard the whole channel exists for.
func AmountPerUnit(s string) error {
	_, err := amount.Parse(s, amount.Dollars)
	return err
}

// Number recognizes a unitless figure: a count, an FTE, a rank, a coverage
// ratio, a rate per $1,000. One grammar for all of them, deliberately — every
// number cell is read-not-published, so the only boundary that guards
// anything is amount-versus-everything-else. Unlike AmountPerUnit it accepts
// any fraction length ("1.1277" is a real p174 rate) and no currency mark,
// dash or parenthesis.
func Number(s string) error {
	if !figure.MatchString(s) {
		return fmt.Errorf("cannot read %q as a number", s)
	}
	return nil
}
