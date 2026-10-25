package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
)

// memoryErrata are matched against a memory body, read from `bd memories
// --json`: a flat object of key to body, with non-string values
// (schema_version) skipped.
//
// THE PHRASE LIST IS NARROW: "used to say" has honest uses about a DOCUMENT
// the city reprinted. Three patterns require past-tense self-reference; the
// ISO-dated "corrected" and line-leading CORRECTED are house-style shapes that
// may refuse a true sentence about the corpus (fisc-kaq1). A bare-year or
// "(correction: ...)" form must not be added.
// TestScanFindsOnlyPastTenseSelfReference holds each edge.
var memoryErrata = []*regexp.Regexp{
	regexp.MustCompile(`(?i)th(is|e)\s+memory\s+(used\s+to|previously|once|said|no\s+longer)`),
	regexp.MustCompile(`(?i)earlier\s+versions?\s+of\s+this\s+memory`),
	regexp.MustCompile(`(?i)\bthis\s+used\s+to\s+(say|read|end|claim)`),
	regexp.MustCompile(`(?i)\bcorrected\s+20\d\d-\d\d-\d\d`),
	regexp.MustCompile(`(?m)^CORRECTED\b`),
}

func memoriesMain(stdin io.Reader, stderr io.Writer) int {
	in, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "memcheck: reading stdin: %v\n", err)
		return 2
	}
	hits, examined, err := scanMemories(in)
	if err != nil {
		fmt.Fprintf(stderr, "memcheck: %v\n", err)
		return 2
	}
	if examined == 0 {
		// A fresh `bd init` holds no memories; warn rather than refuse, as the
		// arm is advisory when bd is absent too.
		fmt.Fprintln(stderr, "warning: the memory set is empty, so nothing was checked")
		return 0
	}
	if len(hits) == 0 {
		return 0
	}
	return refuse(stderr, hits,
		"memcheck: the memories above say what they used to say.",
		"  bd prime injects every memory into every session, so an erratum",
		"  arrives in context whether or not anyone opens the file it is",
		"  about. Keep the corrected statement and delete the sentence",
		"  about what it replaced. Edit with 'bd remember --key <key>'.",
		"  See AGENTS.md, \"History's home is git\".")
}

// scanMemories reports one line per erratum found, sorted by key. It refuses
// any shape it cannot read as bodies -- a top-level null, or a nested value
// (the bodies moved down a level, which a body count alone misses) -- rather
// than reporting it clean. An empty object is returned with a zero count; see
// memoriesMain.
func scanMemories(in []byte) ([]string, int, error) {
	var raw map[string]any
	if err := json.Unmarshal(in, &raw); err != nil {
		return nil, 0, fmt.Errorf("parsing memories: %w", err)
	}
	if raw == nil {
		return nil, 0, fmt.Errorf("the input is JSON null rather than an object of memories")
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var hits []string
	examined := 0
	for _, k := range keys {
		switch v := raw[k].(type) {
		case string:
			examined++
			for _, text := range matches(memoryErrata, v) {
				hits = append(hits, k+": "+text)
			}
		case map[string]any, []any:
			return nil, 0, fmt.Errorf("%q holds a nested value: the memories are not a flat object of key to body, so this check would read past them", k)
		default:
			// schema_version and any other scalar that is not a body.
		}
	}
	return hits, examined, nil
}
