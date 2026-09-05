package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

func TestScanFindsOnlyPastTenseSelfReference(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		// The shapes measured in the memories this check was written for.
		{"used to", "The rule is X. This memory used to say Y.", true},
		{"previously, without the demonstrative", "the memory previously said Y, which is wrong", true},
		{"previously, qualified", "This memory previously said Y.", true},
		{"said, past tense", "This memory said THREE until the fourth scope landed.", true},
		{"noun dropped entirely", "A rule. This used to say something else.", true},
		{"AGENTS.md's own errata idiom", "(Correction, recorded here rather than by amending: it is 34.)", true},
		{"once said", "This memory once said check.All() returns 34.", true},
		{"earlier versions", "TWO EARLIER VERSIONS OF THIS MEMORY WERE WRONG.", true},
		{"earlier version singular", "An earlier version of this memory said Y.", true},
		{"corrected with a date", "CORRECTED 2026-08-29. The override lives once.", true},
		{"corrected, lowercase and parenthesised", "WHAT IS TRUE NOW (corrected 2026-08-29).", true},
		{"corrected, leading and undated", "CORRECTED, twice over. Clause (b) is the rule.", true},

		// Sentences about the DOCUMENTS, which is what the corpus is full of and
		// what a looser pattern would refuse. Every one of these is true and
		// belongs in a memory.
		{"a document was corrected", "The city corrected 2024's printed total in a later schedule.", false},
		{"a correction mid-sentence about a page", "p253 prints a typo the appendix corrected 2025 pages later.", false},
		{"a quotation the extractor produced", "ACFR p177 row 2017 was quoted as producing '-512,946'.", false},
		{"an earlier draft of another file", "An earlier draft of the contract shipped paths it does not write.", false},
		{"a bead's text", "Text on the lane's beads describing work as pending is historical.", false},

		// A present-tense scope limit is a true and useful sentence, and it is
		// the case that decides how far the demonstrative patterns may reach.
		{"present-tense scope limit", "WHAT THIS MEMORY CANNOT TELL YOU IS THE CURRENT PUSH STATE.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := []byte(`{"k": ` + quote(tt.body) + `}`)
			hits, _, err := scan(in)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if got := len(hits) > 0; got != tt.want {
				t.Errorf("scan(%q) flagged = %v, want %v (hits: %v)", tt.body, got, tt.want, hits)
			}
		})
	}
}

func TestScanSkipsNonStringValuesAndSortsByKey(t *testing.T) {
	// schema_version is an int and sits alongside the bodies in bd's output; a
	// scanner that assumed every value was a string would fail on it.
	in := []byte(`{"schema_version": 1,
		"zeta": "This memory used to say Z.",
		"alpha": "This memory used to say A."}`)
	hits, _, err := scan(in)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var keys []string
	for _, h := range hits {
		keys = append(keys, strings.SplitN(h, ":", 2)[0])
	}
	if diff := cmp.Diff([]string{"alpha", "zeta"}, keys); diff != "" {
		t.Errorf("keys (-want +got):\n%s", diff)
	}
}

func TestScanReportsEveryHitInOneBody(t *testing.T) {
	// A memory that carries three errata is three findings, not one: fixing the
	// first and re-running must still go red.
	in := []byte(`{"k": "This memory used to say A. CORRECTED 2026-08-29. This memory once said B."}`)
	hits, _, err := scan(in)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("got %d hits, want 3: %v", len(hits), hits)
	}
}

func TestScanRejectsMalformedInput(t *testing.T) {
	// bd absent, or a truncated pipe, must not read as a clean memory set.
	if _, _, err := scan([]byte("not json")); err == nil {
		t.Error("scan(non-JSON) = nil error, want a parse error")
	}
}

func TestScanRefusesJSONNull(t *testing.T) {
	// null unmarshals into a nil map without an error, so it looks to the type
	// system exactly like an empty memory set and is not one.
	if _, _, err := scan([]byte(`null`)); err == nil {
		t.Error("scan(null) = nil error, want a refusal")
	}
}

func TestScanCountsWhatItExaminedRatherThanRefusingAnEmptySet(t *testing.T) {
	// A beads DB that exists and holds no memories is what a fresh `bd init`
	// returns. Refusing it here blocks a contributor in that state from
	// committing at all, so the count goes back to the caller, which warns.
	for _, in := range []string{`{}`, `{"schema_version":1}`} {
		t.Run(in, func(t *testing.T) {
			_, examined, err := scan([]byte(in))
			if err != nil {
				t.Fatalf("scan(%s) = %v, want no error", in, err)
			}
			if examined != 0 {
				t.Errorf("scan(%s) examined = %d, want 0", in, examined)
			}
		})
	}
}

func TestScanReportsHowManyBodiesItRead(t *testing.T) {
	// The count is what tells "clean" apart from "read nothing", and main is the
	// only place that distinction is acted on.
	_, examined, err := scan([]byte(`{"schema_version":1,"a":"x","b":"y"}`))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if examined != 2 {
		t.Errorf("examined = %d, want 2", examined)
	}
}

func TestScanRefusesANestedShape(t *testing.T) {
	// The count rule alone does not catch an envelope: one stray top-level
	// string beside it satisfies "examined at least one body" while the real
	// bodies sit a level down, unread.
	for _, in := range []string{
		`{"note":"x","memories":{"k":"This memory used to say X."}}`,
		`{"memories":{"k":"This memory used to say X."}}`,
		`{"note":"x","memories":["This memory used to say X."]}`,
		`{"schema_version":1,"memories":{"k":"This memory used to say X."}}`,
	} {
		t.Run(in, func(t *testing.T) {
			if _, _, err := scan([]byte(in)); err == nil {
				t.Errorf("scan(%s) = nil error, want a refusal: the bodies are nested", in)
			}
		})
	}
}

func TestExcerptDoesNotCutAMultiByteRuneInHalf(t *testing.T) {
	// The memory bodies carry em-dashes and pad is a byte count, so an excerpt
	// clamped at a raw byte offset prints replacement bytes at the reader.
	body := strings.Repeat("—", 40) + "This memory used to say Y." + strings.Repeat("—", 40)
	start := strings.Index(body, "This memory used to")
	got := excerpt(body, start, start+len("This memory used to"))
	if !utf8.ValidString(got) {
		t.Errorf("excerpt = %q, want valid UTF-8", got)
	}
}

func TestExcerptCollapsesNewlinesAndClampsAtBothEnds(t *testing.T) {
	body := "This memory used to say\nsomething across two lines."
	got := excerpt(body, 0, len("This memory used to"))
	if strings.Contains(got, "\n") {
		t.Errorf("excerpt(%q) = %q, want no newline", body, got)
	}
	if strings.HasPrefix(got, "...") {
		t.Errorf("excerpt at offset 0 = %q, want no leading ellipsis", got)
	}
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}
