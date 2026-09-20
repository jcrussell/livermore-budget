// Package schema holds the machine-checkable contracts for the artifacts this
// project publishes or reads across a language boundary.
//
// A SCHEMA IS HERE BECAUSE PROSE COULD NOT BE CHECKED. The shape of these
// artifacts was stated in docs/, again in Go struct tags, and a third time in
// site/app.js's hand-written key refusals -- three spellings, none of which
// could be held against the bytes. A committed schema is one spelling that can.
// The arguments stay in docs/: what a schema cannot say is WHY, and those files
// are long and good because of it.
//
// THE EMBED LIVES HERE FOR site/embed.go'S REASON, which is not a style choice:
// //go:embed patterns cannot escape the directory of the file that declares
// them, so the directive has to be a file under schema/.
//
// EACH SCHEMA CARRIES AN $id SO ONE CAN $ref ANOTHER. A shape that appears in
// more than one artifact -- a locator, a caveat -- belongs in its own file
// under its own $id rather than inline in both.
//
// WHAT VALIDATES AGAINST THESE, and the asymmetry is deliberate. Go validates
// fully, through this package. tools/jscheck compares the required arrays here
// against site/app.js's own refusals, because the client ships without npm and
// a second validator in JavaScript would be the thing tools/jscheck refuses to
// write. tools/extract.py reads the required arrays with the standard library
// for the same reason: PyPI is unreachable from the extraction environment. One
// schema, three readers, one implementation.
package schema

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed *.schema.json
var files embed.FS

// FS returns the embedded schemas. Callers take an fs.FS rather than the
// embed.FS so a test can substitute a fstest.MapFS.
func FS() fs.FS { return files }

// Load compiles the named schema, resolving it ready for validation.
//
// IT RESOLVES EAGERLY AND RETURNS THE ERROR. An unresolvable schema is a
// programming error in this repository rather than bad input, but returning it
// keeps the caller free to report which artifact it was about -- "the fact
// schema will not compile" is a different sentence from "this fact is
// malformed", and a reader needs to know which they are looking at.
func Load(name string) (*jsonschema.Resolved, error) {
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("reading schema %s: %w", name, err)
	}
	var doc jsonschema.Schema
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing schema %s: %w", name, err)
	}
	resolved, err := doc.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("resolving schema %s: %w", name, err)
	}
	return resolved, nil
}

// Names are the schemas this package carries, so a caller names one rather than
// spelling a filename that a rename would silently break.
const (
	// Fact is one figure the city printed: a line of facts/facts.jsonl and of
	// every published page shard.
	Fact = "fact.schema.json"

	// Manifest is data/extracted/<doc_id>/manifest.json, which tools/extract.py
	// writes and internal/corpus reads.
	Manifest = "manifest.schema.json"
)
