package amount

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The accepted shapes. Every token here was observed in the extracted
// artifacts under data/extracted/.
func TestParseAccepts(t *testing.T) {
	tests := []struct {
		name  string
		token string
		units Units
		want  Cents
	}{
		{"plain thousands separator", "64,143,762", Dollars, 6_414_376_200},
		{"no separator", "170000", Dollars, 17_000_000},
		{"decimal dollars", "1,234.56", Dollars, 123_456},
		{"single digit", "2", Dollars, 200},

		{"dash is zero", "-", Dollars, 0},
		{"em dash is zero", "—", Dollars, 0},
		{"double dash is zero", "--", Dollars, 0},

		{"parenthesized is negative", "(71.8)", Millions, -7_180_000_000},
		{"parenthesized integer", "(1,034,154)", Dollars, -103_415_400},
		{"ERAF contra revenue", "(15,857,875)", Dollars, -1_585_787_500},

		{"leading dollar sign", "$157,873,470", Dollars, 15_787_347_000},
		{"dollar sign with space", "$ 27,145,882", Dollars, 2_714_588_200},
		{"trailing dollar sign from cell bleed", "27,799,694 $", Dollars, 2_779_969_400},

		{"millions scale", "342.8", Millions, 34_280_000_000},
		{"millions integer", "502", Millions, 50_200_000_000},
		{"thousands scale", "1.5", Thousands, 150_000},

		{"non-breaking space is collapsed", "1,234 ", Dollars, 123_400},

		// The currency symbol bleeds a cell to the right during extraction and
		// lands on either side of a parenthesized negative. 23 such cells sit in
		// the committed corpus and were previously all rejected.
		{"negative with leading dollar sign", "$(1,234)", Dollars, -123_400},
		{"negative with trailing dollar sign", "(1,234) $", Dollars, -123_400},
		{"negative with spaced dollar sign", "$ (95,830,768)", Dollars, -9_583_076_800},
		{"negative with trailing spaced sign", "(2,894,745) $", Dollars, -289_474_500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.token, tt.units)
			if err != nil {
				t.Fatalf("Parse(%q, %s) returned error: %v", tt.token, tt.units, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Parse(%q, %s) mismatch (-want +got):\n%s", tt.token, tt.units, diff)
			}
		})
	}
}

// The corruption catalogue. Each of these would otherwise yield a plausible
// wrong number, which is worse than an error.
func TestParseRejects(t *testing.T) {
	tests := []struct {
		name  string
		token string
		units Units
		why   string
	}{
		{
			name: "digits split by whitespace", token: "2 40,000", units: Dollars,
			why: "CIP p40 renders 240,000 this way; a tolerant parser reads 40,000",
		},
		{
			name: "digits split mid-group", token: "1,4 50,000", units: Dollars,
			why: "CIP p40 renders 1,450,000 this way",
		},
		{
			name: "separator split from digits", token: "150 ,000", units: Dollars,
			why: "CIP p40 renders 150,000 this way",
		},
		{
			name: "leading minus from glued dash", token: "-1,315,352", units: Dollars,
			why: "ACFR p54: an em-dash-as-zero in the prior column glues on; true value is positive",
		},
		{
			name: "leading minus from glued dash, second instance", token: "-512,946", units: Dollars,
			why: "ACFR p177, see TestLeadingMinusIsReallyPositive",
		},
		{
			name: "too many decimals for the unit", token: "1.234", units: Dollars,
			why: "cannot be represented exactly in cents",
		},
		{
			name: "two values glued together", token: "2,894,745 3,102,881", units: Dollars,
			why: "adjacent cells merged during extraction",
		},
		{name: "bare currency symbol", token: "$", units: Dollars, why: "no digits"},
		{name: "text", token: "Property Taxes", units: Dollars, why: "not a number"},
		{name: "footnote marker glued to value", token: "19,250(9)", units: Dollars,
			why: "Budget Book p76 interleaves footnote markers"},
		{name: "unknown units", token: "1,000", units: Units("billions"), why: "unknown scale"},

		// Misplaced grouping means a decimal point was lost. Stripping the
		// commas would silently yield 100x the intended value.
		{name: "lost decimal point", token: "1,234,56", units: Dollars,
			why: "1,234.56 with the point dropped; a permissive parser reads $123,456"},
		{name: "trailing comma", token: "1,234,", units: Dollars, why: "malformed grouping"},
		{name: "short group", token: "1,23,456", units: Dollars, why: "malformed grouping"},

		// int64 cents overflow. strconv bounds only the integer part, so the
		// product can still wrap -- and a wrapped value is negative, which is
		// the worst possible silent answer.
		{name: "overflows at dollar scale", token: "92233720368547759", units: Dollars,
			why: "wraps to a negative value"},
		{name: "overflows at millions scale", token: "999999999999999", units: Millions,
			why: "wraps"},
		// 92233720368547758 * 100 = 9223372036854775800, which fits; adding 99
		// cents does not. The integer-part check alone would let this through.
		{name: "overflows via the fraction", token: "92233720368547758.99", units: Dollars,
			why: "integer part fits, the addition wraps"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.token, tt.units)
			if err == nil {
				t.Fatalf("Parse(%q, %s) = %v, want an error (%s)", tt.token, tt.units, got, tt.why)
			}
			if errors.Is(err, ErrAbsent) {
				t.Fatalf("Parse(%q) returned ErrAbsent, want a parse rejection", tt.token)
			}
			var pe *parseError
			if !errors.As(err, &pe) {
				t.Fatalf("got %T (%v), want a *ParseError naming the token", err, err)
			}
			if pe.Token != tt.token {
				t.Errorf("got ParseError.Token %q, want %q", pe.Token, tt.token)
			}
		})
	}
}

// TestBareCurrencyMarkStaysARejection pins the one rejection this package has
// been under pressure to give up.
//
// 236 of the corpus's 786 pages print a marked figure as two whitespace-
// delimited tokens — "Total Uses 6/30/24 $ 123,228,190" — so a reader that
// takes the next N tokens after a row label took marks where it wanted figures
// and failed. internal/mapping's dropCurrencyMarks now drops the mark in the
// resolver, where the caller knows it is looking for N amounts; this package's
// rejection is what makes that the only place the mark can be handled. Accepting a bare "$" as something — zero, absent,
// a token to skip — is the obvious way to make that go away and the wrong one:
// it would mean a cell whose figure the extractor lost, leaving only the mark
// it was printed with, reads as a value rather than as the failure it is.
//
// The fix for fisc-yun DID land in the reader, which knows how many amounts it
// is looking for and can see a whole line: mapping.dropCurrencyMarks drops a
// token that is a currency mark and nothing else, in the labelled read. This
// paragraph read "whatever eventually fixes fisc-yun belongs in the reader"
// until the ACFR lane found several such sentences outliving that bead's close.
// The rule here is unchanged and is what made that fix possible: this parser
// sees one token and must keep refusing this one.
func TestBareCurrencyMarkStaysARejection(t *testing.T) {
	for _, token := range []string{"$", "$$", " $ "} {
		if got, err := Parse(token, Dollars); err == nil {
			t.Errorf("Parse(%q) = %s, want a refusal: a currency mark is not a figure", token, got)
		} else if errors.Is(err, ErrAbsent) {
			t.Errorf("Parse(%q) reported ErrAbsent, want a parse rejection: the cell "+
				"is not empty, it is unreadable", token)
		}
	}
	// The mark is only unreadable on its own. A cell that carries the mark AND
	// the figure reads fine, which is what makes refusing the bare one a narrow
	// rule rather than a hostile parser.
	if got, err := Parse("$ 123,228,190", Dollars); err != nil || got != 12_322_819_000 {
		t.Errorf("Parse(%q) = %s, %v; want the marked figure to read", "$ 123,228,190", got, err)
	}
}

// Absent and zero mean different things, and conflating them invents rows.
func TestAbsentIsNotZero(t *testing.T) {
	for _, token := range []string{"", "   ", " "} {
		got, err := Parse(token, Dollars)
		if !errors.Is(err, ErrAbsent) {
			t.Errorf("Parse(%q) = (%v, %v), want ErrAbsent", token, got, err)
		}
	}

	// A dash, by contrast, is a real zero.
	got, err := Parse("-", Dollars)
	if err != nil || got != 0 {
		t.Errorf("Parse(\"-\") = (%v, %v), want (0, nil)", got, err)
	}

	// ParseOrZero opts out, but only where a rule says blanks mean zero.
	got, err = parseOrZero("", Dollars)
	if err != nil || got != 0 {
		t.Errorf("ParseOrZero(\"\") = (%v, %v), want (0, nil)", got, err)
	}
}

// Exactness is the whole point: these values are summed and compared against
// published totals, so no float rounding may creep in.
func TestSumsAreExact(t *testing.T) {
	// Budget Book p66, General Fund FY2025-26 revenues.
	tokens := []string{
		"64,143,762", "23,800,196", "4,328,164", "6,130,207", "8,488,168",
		"76,360", "2,180,708", "41,086,606", "386,500", "7,252,799",
	}
	var total Cents
	for _, tok := range tokens {
		c, err := Parse(tok, Dollars)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tok, err)
		}
		total += c
	}

	stated, err := Parse("$157,873,470", Dollars)
	if err != nil {
		t.Fatalf("Parse stated total: %v", err)
	}
	if diff := cmp.Diff(stated, total); diff != "" {
		t.Errorf("mapped rows do not sum to the document's stated total (-want +got):\n%s", diff)
	}
}

func TestCentsString(t *testing.T) {
	tests := []struct {
		in   Cents
		want string
	}{
		{0, "$0.00"},
		{123_456, "$1,234.56"},
		{15_787_347_000, "$157,873,470.00"},
		{-103_415_400, "-$1,034,154.00"},
		{5, "$0.05"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Cents(%d).String() = %q, want %q", int64(tt.in), got, tt.want)
		}
	}
}

// TestLeadingMinusIsReallyPositive is the arithmetic proof behind rejecting a
// leading minus sign, taken from ACFR p177 (a ten-year debt schedule):
//
//	['2017','56,386,950','2,440,343','','10,300,691','13,003,050','','','-512,946','','82,643,980',...]
//
// Two columns before the "-512,946" are empty. The dash belongs to one of
// them as a zero and glued onto the next value during extraction. Summing the
// row proves the sign: only the POSITIVE reading reconciles to the printed
// total. A parser that accepted the token at face value would be off by
// twice the value and would still tie to nothing, silently.
//
// THE ROW ABOVE IS A TRANSCRIPTION, AND IT IS XBERG'S. The committed corpus
// holds no token of that shape -- a leading minus on a grouped number occurs
// zero times across all 786 extracted pages, and poppler reads this row with
// the two dashes standing alone as the published zeros they are. The rule this
// test defends is unchanged and still earns its place: fail closed, because the
// glued form is unreadable rather than merely unusual, and a re-extraction
// could produce it again. What changed is where the evidence lives.
// internal/mapping.TestACFRDebtRowIsNotCorruptedInTheCommittedCorpus reads the
// same row off the committed page, and its companion reconciles all ten rows of
// the schedule against the city's own totals. This package stays corpus-free on
// purpose; the transcription below is the input shape, not a claim about a file.
func TestLeadingMinusIsReallyPositive(t *testing.T) {
	row := []string{"56,386,950", "2,440,343", "10,300,691", "13,003,050", "512,946"}
	var sum Cents
	for _, tok := range row {
		c, err := Parse(tok, Dollars)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tok, err)
		}
		sum += c
	}
	stated, err := Parse("82,643,980", Dollars)
	if err != nil {
		t.Fatalf("Parse stated total: %v", err)
	}
	if diff := cmp.Diff(stated, sum); diff != "" {
		t.Fatalf("the positive reading should reconcile exactly (-want +got):\n%s", diff)
	}

	// And the negative reading does not reconcile, which is why the token
	// must be rejected rather than guessed at.
	negative := sum - 2*Cents(51_294_600)
	if negative == stated {
		t.Error("the negative reading also reconciles; the premise of this test is wrong")
	}

	if _, err := Parse("-512,946", Dollars); err == nil {
		t.Error("Parse accepted a leading minus; it must fail closed")
	}
}

// TestNoOverflowIsSilent is the property the package doc promises: no input
// produces a wrong number without an error. Overflow is the sharpest case,
// because a wrapped int64 comes back NEGATIVE — so a huge positive figure
// would publish as a large negative one with a working provenance link.
func TestNoOverflowIsSilent(t *testing.T) {
	for _, u := range []Units{Dollars, Thousands, Millions} {
		for _, tok := range []string{
			"92233720368547759",
			"999999999999999999",
			"92233720368547758.99",
			"9,223,372,036,854,775,807",
		} {
			got, err := Parse(tok, u)
			if err == nil && got < 0 {
				t.Errorf("Parse(%q, %s) = %v with no error: overflow wrapped silently",
					tok, u, got)
			}
		}
	}
}

// TestNormalizeUnifiesDashes guards the transform the zero rule rests on: the
// documents print a zero cell as a hyphen, an en dash or a true minus sign
// depending on the schedule, and zeroTokens only lists the "-" spelling.
func TestNormalizeUnifiesDashes(t *testing.T) {
	if got, want := normalize("– — − -"), "- - - -"; got != want {
		t.Errorf("Normalize(en/em/minus/hyphen) = %q, want %q", got, want)
	}
	// A backslash is no longer stripped from anything: there is no markdown in
	// the corpus, so `\-` is an unrecognized token rather than a zero.
	if _, err := Parse(`\-`, Dollars); err == nil {
		t.Error(`Parse("\\-") = nil error; an unknown token must fail closed`)
	}
}

// The three helpers a document-derived tolerance is built from (fisc-1wr.2).
//
// THEY ARE TESTED HERE AND NOT ONLY THROUGH internal/mapping, because review
// found them at 0.0% coverage from this package: every assertion about them was
// an assertion about one page of one document, so deleting Decimals' dash-skip
// arms left the whole suite green. A helper whose only witness is one caller's
// happy path is a helper nobody can change safely.

func TestMaxDecimals(t *testing.T) {
	for _, tc := range []struct {
		u    Units
		want int
		ok   bool
	}{
		{Dollars, 2, true},
		{Thousands, 5, true},
		{Millions, 8, true},
		{Units("furlongs"), 0, false},
	} {
		got, ok := MaxDecimals(tc.u)
		if got != tc.want || ok != tc.ok {
			t.Errorf("MaxDecimals(%q) = %d, %v; want %d, %v", string(tc.u), got, ok, tc.want, tc.ok)
		}
	}
}

// TestDigitCents is the unit a tolerance is sized from, and the ACFR case is
// the one that matters: "(in Millions)" printed to two decimals is $10,000 and
// not $1,000,000. Getting it wrong by reading the caption is a hundredfold
// wider bound.
func TestDigitCents(t *testing.T) {
	for _, tc := range []struct {
		u        Units
		decimals int
		want     Cents
		ok       bool
	}{
		{Millions, 2, 1_000_000, true},   // $10,000 -- ACFR p41
		{Millions, 0, 100_000_000, true}, // $1,000,000 -- the caption's reading
		{Millions, 3, 100_000, true},     // $1,000 -- one decimal finer
		{Millions, 8, 1, true},           // one cent, the finest millions can hold
		{Thousands, 2, 1_000, true},
		{Dollars, 2, 1, true},
		{Millions, 9, 0, false},  // finer than the units can represent exactly
		{Dollars, 3, 0, false},   // likewise
		{Millions, -1, 0, false}, // not a count of decimal places
		{Units("furlongs"), 2, 0, false},
	} {
		got, ok := DigitCents(tc.u, tc.decimals)
		if got != tc.want || ok != tc.ok {
			t.Errorf("DigitCents(%q, %d) = %v, %v; want %v, %v",
				string(tc.u), tc.decimals, got, ok, tc.want, tc.ok)
		}
	}
}

// TestDecimals covers the arms a tolerance rests on, and two of them are the
// reason it is not just strings.Cut on a dot.
//
// THE DASH ARMS ARE THE LOAD-BEARING ONES. These documents spell a zero cell
// "-", which says nothing about how finely the page prints. Counting one as
// zero decimals would let a single dash in a column of hundredths claim the
// page prints whole units, and at millions that is a hundredfold wider bound.
func TestDecimals(t *testing.T) {
	for _, tc := range []struct {
		token string
		u     Units
		want  int
		ok    bool
	}{
		{"0.22", Millions, 2, true},
		{"18.45", Millions, 2, true},
		{"157.20", Millions, 2, true},
		{"1", Millions, 0, true},
		{"(25.72)", Millions, 2, true},  // parenthesised negative
		{"$ 60.42", Millions, 2, true},  // currency mark outside
		{"$(95.83)", Millions, 2, true}, // and inside
		{"1,234.5", Dollars, 1, true},   // grouped
		{"", Millions, 0, false},        // absent cell
		{"-", Millions, 0, false},       // the dash these documents spell zero with
		{"--", Millions, 0, false},      // and its other length
		{"(-)", Millions, 0, false},     // a dash inside parentheses
		{"$", Millions, 0, false},       // a bare currency mark
		{"()", Millions, 0, false},      // no digits at all
		{"Miscellaneous", Millions, 0, false},
		// A fraction finer than the units can hold. Parse REJECTS this token,
		// so reporting a decimal count for it would put the two in
		// disagreement about what the token is -- which Decimals' own doc
		// comment promises cannot happen.
		{"1.234", Dollars, 0, false},
		{"1.23456789", Millions, 8, true},   // exactly what millions can hold
		{"1.234567891", Millions, 0, false}, // one place finer
		{"1.2.3", Millions, 0, false},       // not a fraction
		{"0.22", Units("furlongs"), 0, false},
	} {
		got, ok := Decimals(tc.token, tc.u)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Decimals(%q, %q) = %d, %v; want %d, %v",
				tc.token, string(tc.u), got, ok, tc.want, tc.ok)
		}
	}
}

// TestDecimalsAgreesWithParseOnPrecision is the property the two share, asserted
// as a property rather than case by case: whenever Parse accepts a token at some
// units, Decimals reports a count no finer than those units can hold -- and
// whenever Decimals reports one, Parse does not reject the token FOR ITS
// PRECISION.
func TestDecimalsAgreesWithParseOnPrecision(t *testing.T) {
	tokens := []string{"0.22", "18.45", "1", "-", "--", "(25.72)", "$ 60.42",
		"1,234.5", "1.234", "1.234567891", "", "$", "2.5%"}
	for _, u := range []Units{Dollars, Thousands, Millions} {
		max, _ := MaxDecimals(u)
		for _, tok := range tokens {
			d, ok := Decimals(tok, u)
			if !ok {
				continue
			}
			if d > max {
				t.Errorf("Decimals(%q, %q) = %d, finer than %s can hold (%d)",
					tok, string(u), d, string(u), max)
			}
			if _, err := Parse(tok, u); err != nil {
				var pe *parseError
				if errors.As(err, &pe) && strings.Contains(pe.Reason, "decimal places cannot be represented") {
					t.Errorf("Decimals(%q, %q) reported %d places but Parse rejects it for precision: %v",
						tok, string(u), d, err)
				}
			}
		}
	}
}
