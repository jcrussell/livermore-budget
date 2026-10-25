// Package amount parses dollar figures out of extracted PDF text.
//
// It is strict and fail-closed: extraction corrupts numbers into plausible
// wrong values ("2 40,000" for 240,000; a dash-as-zero glued on to make
// "-1,315,352"), so every shape not positively recognized is an error.
//
// All arithmetic is in integer cents; floating point is never used.
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

// Dollars is String without a zero cents part: whole dollars with thousands
// separators, keeping the cents only when a figure has some.
func (c Cents) Dollars() string { return strings.TrimSuffix(c.String(), ".00") }

// String renders Cents as a signed dollar figure with thousands separators.
func (c Cents) String() string {
	if c == Cents(math.MinInt64) {
		// Negating MinInt64 stays negative; Parse cannot produce it, but
		// arithmetic on Cents can.
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

// AllUnits is every scale a source table may be printed in.
func AllUnits() []Units { return []Units{Dollars, Thousands, Millions} }

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
func MaxDecimals(u Units) (int, bool) {
	_, max, ok := u.centsPer()
	return max, ok
}

// DigitCents is what the least significant printed digit of a figure is worth,
// in cents, for a table printed at u to decimals decimal places.
//
// THIS IS THE UNIT A DOCUMENT-DERIVED TOLERANCE IS BUILT FROM, not the caption:
// the ACFR MD&A is "(in Millions)" at two decimals, so its unit is $10,000.
// It reports false for unknown units and for a decimals count the units cannot
// represent as whole cents.
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

// Decimals is how many decimal places a token PRINTS, a claim about the page
// rather than the value: "0.22" prints two and "18.4" one.
//
// It reports false for a token with no digits (absent, a bare "$", or a
// dash): counting one as zero decimals would let one dash in a column of
// hundredths claim the page prints whole units. It follows Parse's
// normalization, and reports false for a fraction longer than u can hold, so
// it never counts a token Parse rejects for its decimals. It does not
// otherwise re-validate the number.
func Decimals(s string, u Units) (int, bool) {
	_, maxDecimals, ok := u.centsPer()
	if !ok {
		return 0, false
	}
	t := stripCurrency(normalize(s))
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		t = stripCurrency(strings.TrimSuffix(strings.TrimPrefix(t, "("), ")"))
	}
	// No zeroTokens test: a dash and a digit-free token both say nothing
	// about precision, and this arm answers for both.
	if !strings.ContainsFunc(t, unicode.IsDigit) {
		return 0, false
	}
	_, fracPart, hasDot := strings.Cut(t, ".")
	if !hasDot {
		return 0, true
	}
	if !fraction.MatchString(fracPart) || len(fracPart) > maxDecimals {
		return 0, false
	}
	return len(fracPart), true
}

// ErrAbsent reports an empty cell, distinct from a parsed zero: "-" means the
// line exists and is zero, an empty cell that it does not apply. Collapsing
// the two invents rows.
var ErrAbsent = errors.New("cell is absent")

// parseError explains why a token was rejected, including the token itself.
type parseError struct {
	Token  string
	Reason string
}

func (e *parseError) Error() string {
	return fmt.Sprintf("cannot parse amount %q: %s", e.Token, e.Reason)
}

var (
	// An integer part with no separators at all.
	ungrouped = regexp.MustCompile(`^[0-9]+$`)
	// An integer part with correctly placed thousands separators: `[0-9,]+`
	// would accept "1,234,56", a lost decimal point, as 123456.
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

// zeroTokens are the ways these documents write "zero", after normalize has
// unified every dash character to "-".
var zeroTokens = map[string]bool{
	"-": true, "--": true, "0": false, // "0" parses normally
}

// normalize canonicalizes a cell without interpreting it: non-breaking spaces
// become spaces, soft hyphens are dropped, the several dash and minus
// characters unify to "-" (all print a zero cell), and whitespace collapses.
//
// Unicode NFKC is not applied: it would need golang.org/x/text
// (byob-release.10), and the only extracted characters it changes are in
// prose, never in a cell.
func normalize(s string) string {
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
// Rejected, as known corruptions: digits separated by whitespace ("2 40,000"),
// a leading minus (negatives are parenthesized here, so it is usually a glued
// dash-as-zero), more decimal places than the unit holds exactly, and
// anything else.
//
// Returns ErrAbsent for an empty cell.
func Parse(s string, u Units) (Cents, error) {
	mult, maxDecimals, ok := u.centsPer()
	if !ok {
		return 0, &parseError{Token: s, Reason: fmt.Sprintf("unknown units %q", string(u))}
	}

	t := normalize(s)
	if t == "" {
		return 0, ErrAbsent
	}

	// Zero dashes, before any other interpretation.
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, nil
	}

	// Strip currency before testing for parentheses: the symbol lands outside
	// them ("$ (95,830,768)") as often as inside.
	t = stripCurrency(t)

	negative := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		negative = true
		t = stripCurrency(strings.TrimSuffix(strings.TrimPrefix(t, "("), ")"))
	}

	if t == "" {
		return 0, &parseError{Token: s, Reason: "no digits, only currency or bracket characters"}
	}
	if isZero, known := zeroTokens[t]; known && isZero {
		return 0, nil
	}

	if splitDigits.MatchString(t) {
		return 0, &parseError{Token: s,
			Reason: "digits separated by whitespace, which is an extraction artifact " +
				"(e.g. \"2 40,000\" for 240,000); the value is ambiguous"}
	}
	if strings.HasPrefix(t, "-") {
		return 0, &parseError{Token: s,
			Reason: "leading minus sign; negatives are parenthesized in these documents, " +
				"so this is usually a dash-as-zero glued on from the previous column"}
	}
	intPart, fracPart, hasDot := strings.Cut(t, ".")

	switch {
	case strings.Contains(intPart, ","):
		if !grouped.MatchString(intPart) {
			return 0, &parseError{Token: s, Reason: "misplaced thousands separator, " +
				"which usually means a decimal point was lost during extraction " +
				"(e.g. \"1,234,56\" for 1,234.56)"}
		}
	case !ungrouped.MatchString(intPart):
		return 0, &parseError{Token: s, Reason: "not a recognized number"}
	}
	if hasDot && !fraction.MatchString(fracPart) {
		return 0, &parseError{Token: s, Reason: "not a recognized number"}
	}
	if len(fracPart) > maxDecimals {
		return 0, &parseError{Token: s, Reason: fmt.Sprintf(
			"%d decimal places cannot be represented exactly in %s", len(fracPart), u)}
	}

	whole, err := strconv.ParseInt(strings.ReplaceAll(intPart, ",", ""), 10, 64)
	if err != nil {
		return 0, &parseError{Token: s, Reason: "integer part out of range"}
	}
	// Bound the product, not just the parsed integer: multiplying by the unit
	// scale can still wrap.
	if whole > math.MaxInt64/mult {
		return 0, &parseError{Token: s, Reason: fmt.Sprintf(
			"value overflows int64 cents at %s scale", u)}
	}
	cents := whole * mult

	if fracPart != "" {
		frac, err := strconv.ParseInt(fracPart, 10, 64)
		if err != nil {
			return 0, &parseError{Token: s, Reason: "fractional part out of range"}
		}
		// mult is divisible by 10^len(fracPart), checked above, so this is exact.
		scale := int64(1)
		for range fracPart {
			scale *= 10
		}
		add := frac * (mult / scale)
		if cents > math.MaxInt64-add {
			return 0, &parseError{Token: s, Reason: fmt.Sprintf(
				"value overflows int64 cents at %s scale", u)}
		}
		cents += add
	}

	if negative {
		cents = -cents
	}
	return Cents(cents), nil
}
