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
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"

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
	resolved, err := doc.Resolve(&jsonschema.ResolveOptions{Loader: load})
	if err != nil {
		return nil, fmt.Errorf("resolving schema %s: %w", name, err)
	}
	return resolved, nil
}

// load resolves a $ref to another schema in this package.
//
// THE REFS ARE ABSOLUTE URIs AND THE FILES ARE LOCAL, which is the whole of
// what this bridges. Each schema's $id is the URL it will be published at, so a
// $ref reads as a public identifier and resolves offline against the embedded
// copy -- a build must not depend on the network, and a reader who fetches the
// $id must get the same document.
//
// It refuses a URI outside this package rather than reaching for it: an
// unresolvable ref is a mistake in this repository, and returning an error
// names which one.
func load(uri *url.URL) (*jsonschema.Schema, error) {
	name := path.Base(uri.Path)
	raw, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, fmt.Errorf("no schema in this package answers %s: %w", uri, err)
	}
	var doc jsonschema.Schema
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing referenced schema %s: %w", name, err)
	}
	return &doc, nil
}

// Validate holds one decoded JSON value to the named schema.
//
// IT COMPILES THE SCHEMA ON EVERY CALL, which is fine for a caller validating
// once and wrong for one validating a stream. A caller in a loop should Load
// once and reuse the Resolved; this exists so a one-shot caller does not have to
// spell the two steps.
func Validate(name string, v any) error {
	resolved, err := Load(name)
	if err != nil {
		return err
	}
	return resolved.Validate(v)
}

// ValidateJSONL holds every line of a JSONL stream to the named schema, and
// reports the FIRST line that does not match rather than all of them.
//
// THE FIRST ONE IS THE USEFUL ONE. A shape that is wrong is usually wrong the
// same way on every record, so a caller printing all 2,382 buries the answer;
// the line number is what a reader needs to go and look.
//
// IT REFUSES AN EMPTY STREAM. A store this read as clean because it held
// nothing would be green with nothing compared, which is the shape AGENTS.md's
// "Prove it can fail" names.
func ValidateJSONL(r io.Reader, name string) error {
	resolved, err := Load(name)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	seen := 0
	for line := 1; sc.Scan(); line++ {
		text := bytes.TrimSpace(sc.Bytes())
		if len(text) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(text, &v); err != nil {
			return fmt.Errorf("line %d is not JSON: %w", line, err)
		}
		seen++
		if err := resolved.Validate(v); err != nil {
			return fmt.Errorf("line %d does not match %s: %w", line, name, err)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if seen == 0 {
		return fmt.Errorf("nothing to check against %s: the stream carries no records", name)
	}
	return nil
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

	// Column is everything the chart needs for one published column of the
	// budget: one node table, the tier order, and one entry per printed
	// schedule.
	Column = "column.schema.json"
)
