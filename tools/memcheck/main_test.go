package main

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestScanFindsOnlyPastTenseSelfReference(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		// The three shapes measured in the memories this check was written for.
		{"used to", "The rule is X. This memory used to say Y.", true},
		{"previously", "the memory previously said Y, which is wrong", false},
		{"previously, qualified", "This memory previously said Y.", true},
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

		// The live scope limit that made the first draft of this list too wide.
		{"present-tense scope limit", "WHAT THIS MEMORY CANNOT TELL YOU IS THE CURRENT PUSH STATE.", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := []byte(`{"k": ` + quote(tt.body) + `}`)
			hits, err := scan(in)
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
	hits, err := scan(in)
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
	// A memory that carries six errata is six findings, not one: fixing the
	// first and re-running must still go red.
	in := []byte(`{"k": "This memory used to say A. CORRECTED 2026-08-29. This memory once said B."}`)
	hits, err := scan(in)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hits) != 3 {
		t.Errorf("got %d hits, want 3: %v", len(hits), hits)
	}
}

func TestScanRejectsMalformedInput(t *testing.T) {
	// bd absent, or a truncated pipe, must not read as a clean memory set.
	if _, err := scan([]byte("not json")); err == nil {
		t.Error("scan(non-JSON) = nil error, want a parse error")
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
