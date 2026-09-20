package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"
)

// issues writes a JSONL export of the given records and returns its path.
func issues(t *testing.T, recs ...map[string]any) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	var b strings.Builder
	for _, r := range recs {
		line, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

// THE FALSE ROWS ARE THE LOAD-BEARING HALF. Every "this bead said" sentence
// below was read out of the committed export, where a bead names its own
// REQUIREMENT rather than its own former text. Six of them exist and a pattern
// taking bare "said" would refuse all six, which is why the list discriminates
// on tense about the TEXT and not on the verb.
func TestScanRefusesOnlyPastTenseClaimsAboutTheBeadsOwnText(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		// The errata measured in the export this check was written for.
		{"used to say", "CORRECTED (fisc-c00): this bead used to say p67 OMITS the row.", true},
		{"previously said", "DO NOT RE-LITIGATE IT. This bead previously said to decide first.", true},
		{"earlier version of", "func at :434 -- an earlier version of this bead opened with :417.", true},
		{"earlier version, parenthesised", "(An earlier version of this bead cited that guard at :479.)", true},
		{"description is wrong", "2026-09-05 THE COLUMN COUNT IN THIS BEAD'S DESCRIPTION IS WRONG.", true},
		{"once", "This bead once claimed the parser had made the case unreachable.", true},
		{"no longer", "This bead no longer describes the schedule it was filed against.", true},
		{"the bead, not this bead", "The bead used to say the fold was Go's.", true},
		{"notes are wrong", "The bead's notes above are wrong about the column count.", true},

		// A bead naming its OWN REQUIREMENT, which is the construct that makes a
		// close reason readable. All six of these forms are in the export.
		{"said were needed", "TWO THINGS THIS BEAD SAID WERE NEEDED HAD NOT BEEN FILED, and both are now.", false},
		{"said was the point", "THE GUARD THIS BEAD SAID WAS THE POINT IS NOW PAID FOR.", false},
		{"said was the deliverable", "THE CHECK, which this bead said was as much the deliverable as the wording.", false},
		{"said to", "the beadrefs pattern, lifted as this bead said to.", false},
		{"the condition it said", "fisc-phtp.2 LANDED, which is the condition this bead said to re-read it on.", false},
		{"said, quoting the ask", "This bead said the residual has a printed home and should be cited.", false},
		{"a count the bead asked for", "THE ERRATA GREP FOUND 14 WHERE THIS BEAD SAID 15.", false},

		// Prose ABOUT the rule, which the beads that document this gate carry.
		// A pattern naming the comment rather than the bead leaves them alone.
		{"quoting the source-comment form", `No errata: "an earlier version of this comment said X" adds a claim.`, false},
		{"describing the Go arm", `the phrase it refuses -- "this comment used to say" -- is invisible when it wraps.`, false},
		{"counting comment errata", "247 comments correcting an earlier version of themselves, 24 across 22 files.", false},

		// Sentences about the DOCUMENTS, which the corpus is full of.
		{"the city reprinted a figure", "An earlier version of the budget book printed 58,179,468 here.", false},
		{"a superseded extractor", "An earlier version of the extractor dropped 13 detail rows from p130.", false},

		// Bead bodies are hard-wrapped, so every form has to be seen across a
		// line break as readily as on one line.
		{"wrapped after the noun", "CORRECTED: this bead\nused to say p67 omits the row.", true},
		{"wrapped mid-phrase", "an earlier\nversion of this bead opened with :417.", true},
		{"wrapped, still a requirement", "TWO THINGS THIS BEAD\nSAID WERE NEEDED HAD NOT BEEN FILED.", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := issues(t, map[string]any{"id": "fisc-test", "description": tc.body})
			hits, examined, err := scan(path)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if examined != 1 {
				t.Fatalf("examined %d beads, want 1; the body was not read at all", examined)
			}
			if got := len(hits) > 0; got != tc.want {
				t.Errorf("refused = %v, want %v, for %q", got, tc.want, tc.body)
			}
		})
	}
}

// Every field a reader is shown is a field an erratum can hide in, so the scan
// covers the whole of prose rather than the description alone -- oakx.2's is in
// its notes and would be missed by a description-only read.
func TestScanReadsEveryProseField(t *testing.T) {
	for _, field := range prose {
		t.Run(field, func(t *testing.T) {
			rec := map[string]any{"id": "fisc-test", field: "This bead used to say otherwise."}
			hits, _, err := scan(issues(t, rec))
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if len(hits) != 1 {
				t.Fatalf("found %d hits in %s, want 1", len(hits), field)
			}
			if diff := cmp.Diff(field, hits[0].field); diff != "" {
				t.Errorf("reported field (-want +got):\n%s", diff)
			}
		})
	}
}

// A non-prose field is not scanned, so a bead id or a timestamp that happens to
// contain a refused phrase cannot report one.
func TestScanIgnoresFieldsAReaderIsNotShown(t *testing.T) {
	rec := map[string]any{"id": "fisc-test", "created_by": "this bead used to say", "labels": []any{"x"}}
	hits, _, err := scan(issues(t, rec))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("found %d hits outside the prose fields, want 0: %v", len(hits), hits)
	}
}

// The output is a report a person reads top to bottom, so it cannot reorder
// between runs of a map.
func TestHitsAreSortedByBeadThenField(t *testing.T) {
	erratum := "This bead used to say otherwise."
	hits, _, err := scan(issues(t,
		map[string]any{"id": "fisc-zzz", "description": erratum},
		map[string]any{"id": "fisc-aaa", "notes": erratum, "description": erratum},
	))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	var got []string
	for _, h := range hits {
		got = append(got, h.id+"/"+h.field)
	}
	want := []string{"fisc-aaa/description", "fisc-aaa/notes", "fisc-zzz/description"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("order (-want +got):\n%s", diff)
	}
}

// AN EXPORT THIS CANNOT READ IS REFUSED, NOT REPORTED CLEAN. Each of these
// returns exit code 2 from main rather than the 0 that would say the beads were
// read and found good -- the shape AGENTS.md calls green because the gate fired.
func TestScanRefusesWhatItCannotRead(t *testing.T) {
	t.Run("no such file", func(t *testing.T) {
		if _, _, err := scan(filepath.Join(t.TempDir(), "absent.jsonl")); err == nil {
			t.Error("a missing export was read as clean")
		}
	})
	t.Run("not json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "issues.jsonl")
		if err := os.WriteFile(path, []byte("{not json\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := scan(path); err == nil {
			t.Error("an unparseable export was read as clean")
		}
	})
	t.Run("empty export", func(t *testing.T) {
		if _, _, err := scan(issues(t)); err == nil {
			t.Error("an export carrying no beads was read as clean")
		}
	})
	t.Run("record with no id", func(t *testing.T) {
		rec := map[string]any{"description": "This bead used to say otherwise."}
		if _, _, err := scan(issues(t, rec)); err == nil {
			t.Error("a record with no id was read as clean, so its hit could not be named")
		}
	})
}

// The excerpt is cut by a byte count and bead bodies carry em-dashes, so both
// ends are walked out to a rune boundary; without that the report prints
// replacement bytes at the cut.
func TestExcerptCutsOnRuneBoundaries(t *testing.T) {
	body := strings.Repeat("— ", 40) + "This bead used to say otherwise." + strings.Repeat(" —", 40)
	hits, _, err := scan(issues(t, map[string]any{"id": "fisc-test", "description": body}))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("found %d hits, want 1", len(hits))
	}
	if !utf8.ValidString(hits[0].text) {
		t.Errorf("excerpt is not valid UTF-8: %q", hits[0].text)
	}
}
