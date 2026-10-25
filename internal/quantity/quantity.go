// Package quantity recognizes the cell grammars that are not money: the
// percentage, per-unit-dollar and unitless-number cells the ACFR Statistical
// Section prints beside its amounts.
//
// A recognized cell is read so its row stays whole and is never published as
// a fact. Every shape not positively recognized is an error: p182's ranges
// ("5600-6000", "900-100"), p185's "NA" and p186's "exempt". No grammar takes
// a units argument: units scale money, and these are not money (p180).
package quantity

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// figure is a non-negative decimal figure: a grouped or ungrouped integer
// part with an optional fraction. Grouping is enforced for amount.Parse's
// reason ("1,234,56" is a lost decimal point).
var figure = regexp.MustCompile(`^([0-9]+|[0-9]{1,3}(,[0-9]{3})+)(\.[0-9]+)?$`)

// Percentage recognizes a figure with a trailing "%", as p177 prints "2.5%".
// No dash-zero, no parenthesized negative, no bare figure: it fails closed.
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
// per capita, per CCF, per meter size. It is amount.Parse at dollars; what it
// adds is that the figure is not city money and must not enter the store
// (p177's "$ 1,009" per capita).
func AmountPerUnit(s string) error {
	_, err := amount.Parse(s, amount.Dollars)
	return err
}

// Number recognizes a unitless figure: a count, an FTE, a rank, a coverage
// ratio, a rate per $1,000. Unlike AmountPerUnit it accepts any fraction
// length ("1.1277" is a p174 rate) and no currency mark, dash or parenthesis.
func Number(s string) error {
	if !figure.MatchString(s) {
		return fmt.Errorf("cannot read %q as a number", s)
	}
	return nil
}
