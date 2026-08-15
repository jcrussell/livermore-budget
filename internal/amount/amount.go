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
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// Cents is a monetary value in integer cents.
type Cents int64

// String renders Cents as a signed dollar figure with thousands separators.
func (c Cents) String() string {
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
	// A bare number, optionally with a decimal fraction. Anchored: partial
	// matches are rejections.
	numeric = regexp.MustCompile(`^[0-9][0-9,]*(\.[0-9]+)?$`)
	// Digits separated by whitespace — the digit-splitting artifact.
	splitDigits = regexp.MustCompile(`[0-9][^\S\n]+[0-9]`)
)

// zeroTokens are the several ways these documents write "zero". The escaped
// form appears because the extractor emits markdown.
var zeroTokens = map[string]bool{
	"-": true, "--": true, `\-`: true, `\--`: true, "0": false, // "0" parses normally
}

// Normalize canonicalizes a cell without interpreting it: NFKC width folding,
// non-breaking and soft-hyphen removal, unification of the several dash and
// minus characters, and whitespace collapse. Exported because the extraction
// artifacts hash normalized text and the two must agree.
func Normalize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ', r == ' ', r == ' ': // non-breaking spaces
			b.WriteRune(' ')
		case r == '­': // soft hyphen
		case r == '‐', r == '‑', r == '‒', r == '–',
			r == '—', r == '―', r == '−': // dashes, minus
			b.WriteRune('-')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.FieldsFunc(b.String(), unicode.IsSpace), " ")
}

// Parse converts a single extracted cell into Cents at the given units.
//
// Accepted: "1,234", "1,234.56", "(71.8)" for negative, "-" and "\-" for
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

	negative := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		negative = true
		t = strings.TrimSuffix(strings.TrimPrefix(t, "("), ")")
		t = strings.TrimSpace(t)
	}

	// Currency symbols bleed one cell to the right during extraction, so a
	// stray "$" on either end is expected and carries no meaning.
	t = strings.TrimSpace(strings.Trim(strings.TrimSpace(t), "$"))

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
	if !numeric.MatchString(t) {
		return 0, &ParseError{Token: s, Reason: "not a recognized number"}
	}

	intPart, fracPart, _ := strings.Cut(t, ".")
	intPart = strings.ReplaceAll(intPart, ",", "")
	if len(fracPart) > maxDecimals {
		return 0, &ParseError{Token: s, Reason: fmt.Sprintf(
			"%d decimal places cannot be represented exactly in %s", len(fracPart), u)}
	}

	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, &ParseError{Token: s, Reason: "integer part out of range"}
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
		cents += frac * (mult / scale)
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
