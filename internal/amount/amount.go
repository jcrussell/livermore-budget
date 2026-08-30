// Package amount parses dollar figures out of extracted PDF text.
//
// It is deliberately strict and fail-closed. PDF text extraction corrupts
// numbers in ways that produce *plausible wrong values* rather than errors —
// a digit-splitting artifact turns 240,000 into "2 40,000", and a
// dash-as-zero in one column glues onto the next to make "-1,315,352" out of
// a positive number. A permissive parser reads both without complaint and
// publishes a confident, wrong figure with a working provenance link. So
// every shape this package does not positively recognize is an error.
//
// All arithmetic is in integer cents. Floating point is never used: these
// values are summed and compared against published totals, and float drift
// would make those comparisons meaningless.
package amount

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Cents is a monetary value in integer cents.
type Cents int64

// String renders Cents as a signed dollar figure with thousands separators.
func (c Cents) String() string {
	if c == Cents(math.MinInt64) {
		// Negating MinInt64 stays negative, which would corrupt the digit
		// grouping below. Parse can no longer produce this, but Cents is a
		// public type and arithmetic on it can.
		return "-$92,233,720,368,547,758.08"
	}
	neg := c < 0
	v := int64(c)
	if neg {
		v = -v
	}
	s := strconv.FormatInt(v/100, 10)
	// Insert thousands separators.
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("$%s.%02d", b.String(), v%100)
	if neg {
		return "-" + out
	}
	return out
}

// Units is the scale a source table is printed in. It comes from the mapping
// rule, never from the token: "71.8" is meaningless until you know the table
// is captioned "(in Millions)".
type Units string

// The scales these documents are printed in.
const (
	Dollars   Units = "dollars"
	Thousands Units = "thousands"
	Millions  Units = "millions"
)

// centsPer returns how many cents one unit represents, and the maximum number
// of decimal places that can be represented exactly at that scale.
func (u Units) centsPer() (mult int64, maxDecimals int, ok bool) {
	switch u {
	case Dollars:
		return 100, 2, true
	case Thousands:
		return 100_000, 5, true
	case Millions:
		return 100_000_000, 8, true
	}
	return 0, 0, false
}

// MaxDecimals is how many decimal places u can represent exactly in cents.
// Dollars 2, thousands 5, millions 8. It reports false for unknown units.
//
// Exported because a mapping rule declaring how many decimals its page PRINTS
// has to be bounded by how many the units can hold, and the alternative is a
// second copy of the table in internal/mapping.
func MaxDecimals(u Units) (int, bool) {
	_, max, ok := u.centsPer()
	return max, ok
}

// DigitCents is what the least significant printed digit of a figure is worth,
// in cents, for a table printed at u to decimals decimal places.
//
// THIS IS THE UNIT A DOCUMENT-DERIVED TOLERANCE IS BUILT FROM, and reading it
// off the caption instead is the mistake it exists to prevent: the ACFR MD&A is
// captioned "(in Millions)" and prints two decimals, so its unit is $10,000 and
// not $1,000,000 -- a hundredfold difference in how much slack a sum is given.
//
// It reports false for unknown units and for a decimals count the units cannot
// represent exactly, because 10^-decimals of a unit would not be a whole number
// of cents and every amount in this project is an integer number of them.
func DigitCents(u Units, decimals int) (Cents, bool) {
	mult, max, ok := u.centsPer()
	if !ok || decimals < 0 || decimals > max {
		return 0, false
	}
	for range decimals {
		mult /= 10
	}
	return Cents(mult), true
}

// Decimals is how many decimal places a token PRINTS, which is a claim about
// the page rather than about the value: "0.22" prints two and "18.4" prints
// one, though both are exact at millions.
//
// It reports false for a token carrying no digits at all -- an absent cell, or
// one of the dashes these documents spell zero with. Such a cell says nothing
// about the page's precision, so a caller measuring what a table prints must
// skip it rather than count it as zero decimals; counting it would let one dash
// in a column of hundredths claim the page prints whole units.
//
// It follows Parse's normalization exactly (whitespace, dashes, currency marks,
// parentheses) so the two cannot disagree about what a token is. It does NOT
// re-validate the number: a caller has already parsed the token, or is about to.
func Decimals(s string, u Units) (int, bool) {
	t := Normalize(s)
	if t == "" {
		return 0, false
	}
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, false
	}
	t = stripCurrency(t)
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		t = stripCurrency(strings.TrimSuffix(strings.TrimPrefix(t, "("), ")"))
	}
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, false
	}
	if !strings.ContainsFunc(t, unicode.IsDigit) {
		return 0, false
	}
	_, fracPart, hasDot := strings.Cut(t, ".")
	if !hasDot {
		return 0, true
	}
	if !fraction.MatchString(fracPart) {
		return 0, false
	}
	return len(fracPart), true
}

// ErrAbsent reports an empty cell. It is distinct from a parsed zero on
// purpose: in these documents "-" means the line exists and is zero, while an
// empty cell means the line does not apply to that column. Collapsing the two
// silently invents rows.
var ErrAbsent = errors.New("cell is absent")

// A ParseError explains why a token was rejected, including the token itself
// so the failure is actionable from a log line.
type ParseError struct {
	Token  string
	Reason string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("cannot parse amount %q: %s", e.Token, e.Reason)
}

var (
	// An integer part with no separators at all.
	ungrouped = regexp.MustCompile(`^[0-9]+$`)
	// An integer part with correctly placed thousands separators. Enforcing
	// the grouping matters: a permissive `[0-9,]+` accepts "1,234,56" — a
	// figure whose decimal point was lost during extraction — and stripping
	// its commas yields 123456, a silent 100x error.
	grouped = regexp.MustCompile(`^[0-9]{1,3}(,[0-9]{3})+$`)
	// A decimal fraction.
	fraction = regexp.MustCompile(`^[0-9]+$`)
	// Digits separated by whitespace — the digit-splitting artifact.
	splitDigits = regexp.MustCompile(`[0-9][^\S\n]+[0-9]`)
)

// stripCurrency removes surrounding whitespace and currency symbols. Applied
// both outside and inside the parentheses of a negative, because the symbol
// bleeds a cell to the right during extraction and lands on either side.
func stripCurrency(s string) string {
	for {
		t := strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "$"))
		if t == s {
			return t
		}
		s = t
	}
}

// zeroTokens are the several ways these documents write "zero". Normalize has
// already unified every dash character to "-", so these are the two lengths
// the printed rules come in.
var zeroTokens = map[string]bool{
	"-": true, "--": true, "0": false, // "0" parses normally
}

// Normalize canonicalizes a cell without interpreting it: non-breaking spaces
// become spaces, soft hyphens are dropped, the several dash and minus
// characters unify to "-", and whitespace collapses.
//
// Unifying the dashes is what makes the zero rule tractable: these documents
// print a zero cell as a hyphen, an en dash or a true minus sign depending on
// the schedule, and all three mean the same thing.
//
// Unicode NFKC is deliberately not applied. It would mean a golang.org/x/text
// dependency (byob-release.10), and the only characters in the extracted pages
// it would change are 45 ellipses and one trademark sign — all in prose, never
// in a cell this package is asked to parse. The non-breaking space it would
// also fold is handled explicitly below, because that one does land in cells.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case ' ', ' ', ' ': // non-breaking spaces
			b.WriteRune(' ')
		case '­': // soft hyphen
		case '‐', '‑', '‒', '–',
			'—', '―', '−': // dashes, minus
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.FieldsFunc(b.String(), unicode.IsSpace), " ")
}

// Parse converts a single extracted cell into Cents at the given units.
//
// Accepted: "1,234", "1,234.56", "(71.8)" for negative, "-" and "--" for
// zero, and a leading or trailing "$" with or without a space.
//
// Rejected, because each is a known corruption that would otherwise yield a
// wrong number: digits separated by whitespace ("2 40,000"), a leading minus
// sign (negatives are parenthesized here, so a leading "-" is almost always a
// dash-as-zero glued on from the previous column), more decimal places than
// the unit can represent exactly, and anything else.
//
// Returns ErrAbsent for an empty cell.
func Parse(s string, u Units) (Cents, error) {
	mult, maxDecimals, ok := u.centsPer()
	if !ok {
		return 0, &ParseError{Token: s, Reason: fmt.Sprintf("unknown units %q", string(u))}
	}

	t := Normalize(s)
	if t == "" {
		return 0, ErrAbsent
	}

	// Zero dashes, before any other interpretation.
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, nil
	}

	// Strip currency before testing for parentheses: the symbol lands outside
	// them ("$ (95,830,768)") as often as inside, and testing first would
	// reject every parenthesized negative that carries a bled "$".
	t = stripCurrency(t)

	negative := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		negative = true
		t = stripCurrency(strings.TrimSuffix(strings.TrimPrefix(t, "("), ")"))
	}

	if t == "" {
		return 0, &ParseError{Token: s, Reason: "no digits, only currency or bracket characters"}
	}
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, nil
	}

	if splitDigits.MatchString(t) {
		return 0, &ParseError{Token: s,
			Reason: "digits separated by whitespace, which is an extraction artifact " +
				"(e.g. \"2 40,000\" for 240,000); the value is ambiguous"}
	}
	if strings.HasPrefix(t, "-") {
		return 0, &ParseError{Token: s,
			Reason: "leading minus sign; negatives are parenthesized in these documents, " +
				"so this is usually a dash-as-zero glued on from the previous column"}
	}
	intPart, fracPart, hasDot := strings.Cut(t, ".")

	switch {
	case strings.Contains(intPart, ","):
		if !grouped.MatchString(intPart) {
			return 0, &ParseError{Token: s, Reason: "misplaced thousands separator, " +
				"which usually means a decimal point was lost during extraction " +
				"(e.g. \"1,234,56\" for 1,234.56)"}
		}
	case !ungrouped.MatchString(intPart):
		return 0, &ParseError{Token: s, Reason: "not a recognized number"}
	}
	if hasDot && !fraction.MatchString(fracPart) {
		return 0, &ParseError{Token: s, Reason: "not a recognized number"}
	}
	if len(fracPart) > maxDecimals {
		return 0, &ParseError{Token: s, Reason: fmt.Sprintf(
			"%d decimal places cannot be represented exactly in %s", len(fracPart), u)}
	}

	whole, err := strconv.ParseInt(strings.ReplaceAll(intPart, ",", ""), 10, 64)
	if err != nil {
		return 0, &ParseError{Token: s, Reason: "integer part out of range"}
	}
	// Bound the product, not just the parsed integer. strconv only checked
	// that the digits fit in an int64; multiplying by the unit scale can
	// still wrap, and a wrapped value is a confident wrong number of exactly
	// the kind this package exists to refuse.
	if whole > math.MaxInt64/mult {
		return 0, &ParseError{Token: s, Reason: fmt.Sprintf(
			"value overflows int64 cents at %s scale", u)}
	}
	cents := whole * mult

	if fracPart != "" {
		frac, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, &ParseError{Token: s, Reason: "fractional part out of range"}
		}
		// mult is divisible by 10^len(fracPart), checked above, so this is exact.
		scale := int64(1)
		for range fracPart {
			scale *= 10
		}
		add := frac * (mult / scale)
		if cents > math.MaxInt64-add {
			return 0, &ParseError{Token: s, Reason: fmt.Sprintf(
				"value overflows int64 cents at %s scale", u)}
		}
		cents += add
	}

	if negative {
		cents = -cents
	}
	return Cents(cents), nil
}

// ParseOrZero is Parse with absent cells treated as zero. Use it only where a
// rule has declared that a blank means zero for that table; the default is to
// keep the distinction.
func ParseOrZero(s string, u Units) (Cents, error) {
	c, err := Parse(s, u)
	if errors.Is(err, ErrAbsent) {
		return 0, nil
	}
	return c, err
}
