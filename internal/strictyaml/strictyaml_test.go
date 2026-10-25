package strictyaml_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/jcrussell/livermore-budget/internal/strictyaml"
)

type doc struct {
	Name string `yaml:"name"`
}

// TestDecodeRefuses holds the three refusals. Mutations: drop KnownFields and
// the unknown key decodes; skip the second Decode and the second document is
// discarded; return the EOF as is and the empty file reads as "EOF".
func TestDecodeRefuses(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{"an unknown key", "name: a\nnmae: b\n", "field nmae not found in type strictyaml_test.doc"},
		{"an empty file", "", strictyaml.ErrEmpty.Error()},
		{"a second document", "name: a\n---\nname: b\n", strictyaml.ErrMultipleDocuments.Error()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var d doc
			err := strictyaml.Decode(strings.NewReader(tc.src), &d)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Decode = %v, want an error containing %q", err, tc.want)
			}
		})
	}
	if err := strictyaml.Decode(strings.NewReader("name: a\n---\nname: b\n"), &doc{}); !errors.Is(err, strictyaml.ErrMultipleDocuments) {
		t.Errorf("a second document = %v, want ErrMultipleDocuments for a caller to hint on", err)
	}

	var d doc
	if err := strictyaml.Decode(strings.NewReader("name: a\n"), &d); err != nil {
		t.Fatalf("one document: %v", err)
	}
	if diff := cmp.Diff(doc{Name: "a"}, d); diff != "" {
		t.Errorf("decoded (-want +got):\n%s", diff)
	}
}
