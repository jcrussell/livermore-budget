package project

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// This file holds the parts of a published document that are NOT the Sankey's.
//
// Source, Counts and the encoder below were declared in sankey.go, beside the
// only projection that existed. They are here because the next projection needs
// them and inheriting them from a graph would be the wrong relationship: a
// document that cites pages and accounts for facts is every projection, and a
// document with nodes and links is one of them.
//
// WHAT DELIBERATELY DID NOT MOVE, because it is not shared and pretending
// otherwise would cost more than it saves:
//
//   - Metadata carries FiscalYear, FiscalYearLabel and Basis, all singular, and
//     a Headline. A trends document spans four fiscal years and four bases, so
//     it can fill none of them; hoisting Metadata would hand every future
//     document a fiscal_year it has to leave wrong or blank. Absent is not zero
//     here either.
//   - Headline's keys are all_funds_gross_revenue_cents and
//     naive_expenditure_cents. They are the spine's, and they mean nothing
//     anywhere else.
//
// This move changes no bytes. encoding/json emits a struct's fields in
// declaration order, and which FILE a type is declared in is not part of that,
// so testdata/sankey.golden.json is the proof the extraction was faithful --
// internal/project/sankey_test.go compares it with bytes.Equal.

// Counts is how much of the corpus this document accounts for.
type Counts struct {
	// Facts is how many facts matched the options, which is NOT how many links
	// were drawn: stocks get no link, and neither do zero-valued cells. The
	// gap between Facts and Links is the part of the schedule the chart cannot
	// show, and stating both is what makes it visible.
	Facts int `json:"facts"`
	// FactsCited is how many of those facts a link actually carries. Facts
	// minus FactsCited is exactly the zero-valued cells plus the stock rows,
	// which makes the gap a quantity a check can assert rather than a
	// discrepancy a reader has to explain to themselves.
	FactsCited int `json:"facts_cited"`
	Nodes      int `json:"nodes"`
	Links      int `json:"links"`
}

// Source is one document the facts came from, with the pages actually read.
type Source struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// encode renders a document as the canonical JSON every projection publishes.
//
// It is one function rather than a habit each projection repeats because the
// configuration is not defaulted and the difference is visible in the file.
// HTML escaping is off, so "Fines & Forfeitures" stays readable instead of
// becoming "Fines & Forfeitures"; the indent is two spaces, so a reviewer
// diffs the file line by line; and json.Encoder appends exactly one newline,
// giving the file LF endings and a trailing newline like every other text file
// in the repo.
//
// A second projection that built its own encoder would ship with escaping ON,
// because that is json.Marshal's default and nothing would say so until a
// reader saw & on the page.
//
// name is the projection's, so a failure names the document that could not be
// written rather than the type that could not be marshalled.
func encode(v any, name string) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode %s projection: %w", name, err)
	}
	return b.Bytes(), nil
}
