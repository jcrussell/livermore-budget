// Package vocab is the single definition of the closed vocabularies a fact
// carries: its Kind, its Basis and its Sign.
//
// Every package that names one of them -- the rule parser that refuses a
// value outside the set, the registry that refuses a taxonomy member outside
// it, the fact store, the projections and the checks -- reads the type, the
// constants and the set from here. Each set is one list: the constants, the
// Valid method and the list spelled for an error message all read it, so a
// sixth member cannot be added to one spelling and missed by another.
package vocab

import (
	"slices"
	"strings"
)

// Kind is what a fact represents in the flow model.
type Kind string

// The kinds of fact a rule can produce.
const (
	KindRevenue     Kind = "revenue"
	KindExpenditure Kind = "expenditure"
	KindTransferIn  Kind = "transfer_in"
	KindTransferOut Kind = "transfer_out"
	KindFundBalance Kind = "fund_balance"
)

// kinds is the closed set, in the order the constants declare it.
var kinds = []Kind{
	KindRevenue,
	KindExpenditure,
	KindTransferIn,
	KindTransferOut,
	KindFundBalance,
}

// Kinds returns the closed set of kinds, in declaration order. The returned
// slice is a copy: a caller that sorts or appends must not be able to move the
// vocabulary.
func Kinds() []Kind { return slices.Clone(kinds) }

// Valid reports whether k is one of [Kinds].
func (k Kind) Valid() bool { return slices.Contains(kinds, k) }

// KindList spells the closed set for an error message, comma separated in
// declaration order, so no message re-types the five values.
func KindList() string { return join(kinds) }

// Basis distinguishes a budgeted figure from an audited one. Mixing them in a
// single view is the most common way a civic budget chart misleads, so it is
// carried on every fact rather than assumed per document.
type Basis string

// The bases a figure can be stated on.
const (
	BasisAdopted   Basis = "adopted"
	BasisRevised   Basis = "revised"
	BasisActual    Basis = "actual"
	BasisAudited   Basis = "audited"
	BasisProjected Basis = "projected"
)

var bases = []Basis{BasisAdopted, BasisRevised, BasisActual, BasisAudited, BasisProjected}

// Bases is every basis a column may be of, in declaration order, as a copy.
func Bases() []Basis { return slices.Clone(bases) }

// Valid reports whether b is one of [Bases].
func (b Basis) Valid() bool { return slices.Contains(bases, b) }

// BasisList spells the closed set for an error message, comma separated in
// declaration order.
func BasisList() string { return join(bases) }

// Sign says how a row relates to its category, and it is never an instruction
// to negate: AmountCents is always the figure as the document printed it.
//
// Two things need saying and they are different. SignContra marks a row that
// REDUCES its category rather than adding to it. SignNetted marks a row the
// document prints against its KIND's direction. Both leave the amount alone.
type Sign string

const (
	// SignPositive is the default.
	SignPositive Sign = "positive"
	// SignContra marks a deduction booked as negative revenue — the ERAF and
	// RPTTF property-tax shifts on Budget Book p127 are ~26% of gross
	// property tax. Where the negative goes is the consumer's: a category-grain
	// view nets it into its parent, a view drawing the printed row carries it
	// as a negative value on that row's own link.
	SignContra Sign = "contra"
	// SignNetted marks a row the document prints with the OPPOSITE ORIENTATION
	// to its kind's convention, because it sits inside a block that sums to a
	// net figure.
	//
	// It is not SignContra one more time, and the difference is the one this
	// field exists to carry. A contra row is a deduction INSIDE its own
	// category: p127's ERAF reduces property tax, and summing it with its
	// siblings is exactly right. A netted row is the SAME quantity pointing the
	// other way: ACFR p41 prints Transfers (out) as (25.72) because its block
	// sums to a net Other Financing Sources (Uses), while Budget Book p66 prints
	// TRANSFER OUT as a positive magnitude in a uses column. Both are the money
	// leaving, both are published exactly as printed, and summing the two
	// together cancels rather than accumulates (fisc-fdxx).
	//
	// IT IS STILL NOT AN INSTRUCTION TO NEGATE. AmountCents remains the figure
	// as the document printed it; this says which way the document was facing.
	SignNetted Sign = "netted"
)

var signs = []Sign{SignPositive, SignContra, SignNetted}

// Signs is every sign a row may declare, as a copy; a row declaring none is
// positive.
func Signs() []Sign { return slices.Clone(signs) }

// Valid reports whether s is one of [Signs] or the empty string, which a rule
// file leaves for SignPositive.
func (s Sign) Valid() bool { return s == "" || slices.Contains(signs, s) }

func join[T ~string](set []T) string {
	s := make([]string, len(set))
	for i, v := range set {
		s[i] = string(v)
	}
	return strings.Join(s, ", ")
}
