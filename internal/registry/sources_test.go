package registry

import (
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// validSource is one well-formed entry, as the smallest thing that loads. The
// tests below make one field wrong at a time by substituting a whole entry, so
// each case reads as the file it describes.
const validSource = `schema_version: 1
sources:
  - id: livermore-budget-fy2026-2027
    title: "FY 2025-2027 Budget Book"
    publisher: "City of Livermore, California"
    fiscal_years: [2026, 2027]
    basis: adopted
    document_id: 12813
    url: "https://www.livermoreca.gov/home/showpublisheddocument/12813"
    url_versioned: "https://www.livermoreca.gov/home/showpublisheddocument/12813/638871258781300000"
    retrieved: 2026-08-15
    file: data/pdf/livermore-budget-fy2026-2027.pdf
    bytes: 35648752
    sha256: "6838990f0668bae2f170533630ae61185ff3c94a011c7b10ea33c0890089a83d"
    pages: 268
`

func sourcesFS(body string) fstest.MapFS {
	return fstest.MapFS{SourcesFile: &fstest.MapFile{Data: []byte(body)}}
}

func TestLoadSources(t *testing.T) {
	got, err := LoadSources(sourcesFS(validSource))
	if err != nil {
		t.Fatalf("LoadSources: %v", err)
	}
	want := []Source{{
		ID:           "livermore-budget-fy2026-2027",
		Title:        "FY 2025-2027 Budget Book",
		Publisher:    "City of Livermore, California",
		FiscalYears:  []int{2026, 2027},
		Basis:        "adopted",
		DocumentID:   12813,
		URL:          "https://www.livermoreca.gov/home/showpublisheddocument/12813",
		URLVersioned: "https://www.livermoreca.gov/home/showpublisheddocument/12813/638871258781300000",
		Retrieved:    "2026-08-15",
		File:         "data/pdf/livermore-budget-fy2026-2027.pdf",
		Bytes:        35648752,
		SHA256:       "6838990f0668bae2f170533630ae61185ff3c94a011c7b10ea33c0890089a83d",
		Pages:        268,
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("LoadSources (-want +got):\n%s", diff)
	}
}

// TestLoadSourcesRejects covers every way the source registry can be wrong.
// Each case asserts the message names the entry, because a report that says only
// "sha256: is not a lower-case hex sha256" over a three-document file is a
// search.
func TestLoadSourcesRejects(t *testing.T) {
	// entry is the body of one source, indented into the document below.
	entry := func(lines ...string) string {
		return "schema_version: 1\nsources:\n  - " + strings.Join(lines, "\n    ") + "\n"
	}
	const (
		id    = "id: livermore-budget-fy2026-2027"
		file  = "file: data/pdf/livermore-budget-fy2026-2027.pdf"
		sha   = `sha256: "6838990f0668bae2f170533630ae61185ff3c94a011c7b10ea33c0890089a83d"`
		size  = "bytes: 35648752"
		pages = "pages: 268"
	)

	tests := []struct {
		name string
		body string
		want string
	}{{
		name: "a schema version from a newer fisc",
		body: "schema_version: 2\nsources: []\n",
		want: "sources.yaml: schema_version: got 2, want 1",
	}, {
		name: "no schema version",
		body: "sources: []\n",
		want: "sources.yaml: schema_version: got 0, want 1",
	}, {
		name: "no sources",
		body: "schema_version: 1\nsources: []\n",
		want: "sources.yaml: sources: is empty",
	}, {
		name: "an unknown key, which is how a typo'd sha256 would go unread",
		body: entry(id, file, sha, size, pages, `sha256sum: "x"`),
		want: "field sha256sum not found",
	}, {
		name: "a source with no id",
		body: entry(`title: "Budget Book"`, file, sha, size, pages),
		want: `sources.yaml: sources[0]: id: is required (title "Budget Book")`,
	}, {
		name: "two sources claiming one id",
		body: entry(id, file, sha, size, pages) + "  - " + strings.Join([]string{id, file, sha, size, pages}, "\n    ") + "\n",
		want: `sources.yaml: source "livermore-budget-fy2026-2027": id: duplicate document id`,
	}, {
		name: "a source with no file",
		body: entry(id, sha, size, pages),
		want: `sources.yaml: source "livermore-budget-fy2026-2027": file: is required`,
	}, {
		// The path is joined onto the repository root and read through an fs.FS,
		// which would refuse this anyway; refusing it here means the registry
		// cannot even express a document outside the tree.
		name: "a file outside the repository",
		body: entry(id, "file: ../../etc/passwd", sha, size, pages),
		want: "is not a repository-relative slash-separated path",
	}, {
		name: "an absolute file path",
		body: entry(id, "file: /var/tmp/budget.pdf", sha, size, pages),
		want: "is not a repository-relative slash-separated path",
	}, {
		name: "a truncated hash",
		body: entry(id, file, `sha256: "6838990f"`, size, pages),
		want: `sha256: "6838990f" is not a lower-case hex sha256`,
	}, {
		// Not folded to lower case: every party that records this hash compares
		// it for equality, and normalizing on one side only would make agreement
		// depend on who did the comparing.
		name: "an upper-case hash",
		body: entry(id, file, `sha256: "6838990F0668BAE2F170533630AE61185FF3C94A011C7B10EA33C0890089A83D"`, size, pages),
		want: "is not a lower-case hex sha256",
	}, {
		name: "no hash at all",
		body: entry(id, file, size, pages),
		want: `sha256: "" is not a lower-case hex sha256`,
	}, {
		name: "no size",
		body: entry(id, file, sha, pages),
		want: "bytes: is 0; the size of the retrieved file is half of what identifies it",
	}, {
		// A page count of zero would make the completeness check in `fisc verify`
		// vacuous: an extraction with every page deleted from its manifest agrees
		// with a directory with every page deleted from it, and page_count is the
		// only number that catches it.
		name: "no page count",
		body: entry(id, file, sha, size),
		want: "pages: is 0; it is what says how much of the document an extraction should contain",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadSources(sourcesFS(tt.body))
			if err == nil {
				t.Fatalf("LoadSources(%s) = nil error, want %q", tt.body, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", err, tt.want)
			}
		})
	}
}

// TestLoadSourcesHintsAtANewerSchema: a file written for a newer fisc is not a
// malformed file, and the fix is to upgrade the binary rather than to edit the
// registry.
func TestLoadSourcesHintsAtANewerSchema(t *testing.T) {
	_, err := LoadSources(sourcesFS("schema_version: 2\nsources: []\n"))
	var hint *cmdutil.ErrHint
	if !errors.As(err, &hint) {
		t.Fatalf("error %v carries no hint", err)
	}
	if !strings.Contains(hint.Hint, "upgrade the binary") {
		t.Errorf("hint %q does not say what to do", hint.Hint)
	}
}

// TestLoadSourcesWithNoFile: the source registry is not optional, and a missing
// one has to be distinguishable from an empty one.
func TestLoadSourcesWithNoFile(t *testing.T) {
	_, err := LoadSources(fstest.MapFS{})
	if err == nil {
		t.Fatal("LoadSources over an empty filesystem = nil error")
	}
	if !strings.Contains(err.Error(), SourcesFile) {
		t.Errorf("error %q does not name the file it looked for", err)
	}
}

// TestLoadSourcesReadsTheCommittedRegistry is the check that this loader and the
// hand-written file still agree: it is strict about unknown keys, so a key added
// to data/sources.yaml without being declared here fails HERE rather than being
// silently dropped from what `fisc verify` compares.
func TestLoadSourcesReadsTheCommittedRegistry(t *testing.T) {
	got, err := LoadSources(os.DirFS("../../data"))
	if err != nil {
		t.Fatalf("LoadSources over data/: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("the registry lists %d sources, want the 3 documents this project reads", len(got))
	}
	for _, s := range got {
		if s.URL == "" || s.Retrieved == "" {
			t.Errorf("%s records no url or no retrieval date", s.ID)
		}
		if want := "data/pdf/" + s.ID + ".pdf"; s.File != want {
			// tools/extract.py discovers its work by this name, so a source
			// stored elsewhere would never be extracted.
			t.Errorf("%s is stored at %q, but the extractor looks for %q", s.ID, s.File, want)
		}
	}
}
