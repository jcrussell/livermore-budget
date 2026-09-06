package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The fixtures below hold the refused phrases only inside string literals,
// which this command never reads as comments; see the package comment.

func TestScanSource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []finding
	}{
		{
			name: "erratum wrapped across two line comments is found",
			src: "package p\n\n" +
				"// The zero value stays valid. Note that\n" +
				"// this comment used to\n" +
				"// say the opposite.\n" +
				"var x int\n",
			want: []finding{{file: "fix.go", line: 4, phrase: "this comment used to say"}},
		},
		{
			name: "review credit after code on the same line is found",
			src: "package p\n\n" +
				"var x = 1 // Found by /code-review over some range.\n",
			want: []finding{{file: "fix.go", line: 3, phrase: "Found by /code-review"}},
		},
		{
			name: "bare interior line of a block comment is found",
			src: "package p\n\n" +
				"/*\nThe grid is fixed at extraction.\nFound by /code-review.\n*/\n" +
				"var x int\n",
			want: []finding{{file: "fix.go", line: 5, phrase: "Found by /code-review"}},
		},
		{
			name: "erratum wrapped inside a block comment interior is found",
			src: "package p\n\n" +
				"/*\nThe grid is fixed. this comment\nused to say otherwise.\n*/\n" +
				"var x int\n",
			want: []finding{{file: "fix.go", line: 4, phrase: "this comment used to say"}},
		},
		{
			name: "trailing comment continuing onto the next line is found",
			src: "package p\n\n" +
				"var x = 1 // Note that this comment\n" +
				"// used to say something else.\n",
			want: []finding{{file: "fix.go", line: 3, phrase: "this comment used to say"}},
		},
		{
			name: "sentence-initial capitalisation is found",
			src: "package p\n\n" +
				"// This comment used to say the total was gross.\n" +
				"var x int\n",
			want: []finding{{file: "fix.go", line: 3, phrase: "This comment used to say"}},
		},
		{
			name: "two comments adjacent across a code line must not join",
			src: "package p\n\n" +
				"// Wrapping is chosen by gofmt. Note this comment\n" +
				"var x = 1\n" +
				"// used to say very little about wrapping.\n",
			want: nil,
		},
		{
			name: "two comments adjacent across a blank line must not join",
			src: "package p\n\n" +
				"// Every credit names its range. Found by\n" +
				"\n" +
				"// /code-review is not how a commit body opens.\n" +
				"var x int\n",
			want: nil,
		},
		{
			name: "refused phrases inside string literals are not comments",
			src: "package p\n\n" +
				"var s = \"// Found by /code-review\"\n" +
				"var r = `// this comment used to say`\n",
			want: nil,
		},
		{
			name: "a clean doc comment stays clean",
			src: "package p\n\n" +
				"// ParseOrZero opts out only where a rule has declared\n" +
				"// blanks mean zero for that table.\n" +
				"var x int\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scanSource("fix.go", []byte(tt.src))
			if err != nil {
				t.Fatalf("scanSource: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, cmp.AllowUnexported(finding{})); diff != "" {
				t.Errorf("findings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestScanPathsWalksAndCounts(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.go":     "package p\n\n// Found by\n// /code-review over a range.\nvar x int\n",
		"b.go":     "package p\n\nvar y int\n",
		"notes.md": "Found by /code-review is fine here, this walk reads only .go\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, scanned, err := scanPaths([]string{dir})
	if err != nil {
		t.Fatalf("scanPaths: %v", err)
	}
	if scanned != 2 {
		t.Errorf("scanned = %d, want 2: the walk must read every .go file and nothing else", scanned)
	}
	want := []finding{{file: filepath.Join(dir, "a.go"), line: 3, phrase: "Found by /code-review"}}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(finding{})); diff != "" {
		t.Errorf("findings mismatch (-want +got):\n%s", diff)
	}
}

func TestScanPathsFailsOnAPathItCannotRead(t *testing.T) {
	_, _, err := scanPaths([]string{filepath.Join(t.TempDir(), "gone")})
	if err == nil {
		t.Fatal("want an error for a path that is not there: a path that cannot be read is a path that cannot be checked")
	}
}
