package export

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// columnSchemaVersion is the version a columnDoc declares, spelled once.
const columnSchemaVersion = 1

// columnDoc is everything the chart needs for one published column.
//
// ONE NODE TABLE AND N LINK SETS, WHICH IS THE WHOLE SHAPE. The schedules this
// column publishes name the same marks -- fund-group/capital appears in three
// of them -- and every shared node was measured byte-identical with no tier
// disagreement, so the node repetition was removable. Their LINKS are not: each
// schedule is a different page of the city's budget with its own provenance,
// and folding them into one graph would silently reconcile cells this project
// goes to trouble to keep visibly unreconciled.
//
// KEYED BY COLUMN, NOT BY YEAR. A column is (fiscal year, basis). One basis per
// year is a property of today's corpus rather than a guarantee, and the packager
// already refuses a step document on both -- so naming the file by year would
// collide the day the city prints FY2026 revised beside FY2026 adopted.
type columnDoc struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedBy   string                 `json:"generated_by,omitempty"`
	Column        columnKey              `json:"column"`
	Nodes         []columnNode           `json:"nodes"`
	Tiers         []columnTier           `json:"tiers"`
	Schedules     map[string]columnSched `json:"schedules"`
}

type columnKey struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	Label      string `json:"label"`
}

// columnNode carries only what the schedules AGREE on.
//
// MEASURED RATHER THAN CHOSEN. Over every published column, two schedules naming
// the same mark never disagree about id, label, tier, role or derived -- and do
// disagree about parent (156 times), constraint_tier, rationale and source_note.
// A node's identity is shared; where it hangs and what is said about it belong
// to the schedule that draws it. The first attempt put parent here and the
// build refused itself on fy2024-actual, which is how this was found.
type columnNode struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Tier    int    `json:"tier"`
	Role    string `json:"role,omitempty"`
	Derived bool   `json:"derived,omitempty"`
}

// columnSchedNode is one schedule's view of a node the column carries.
type columnSchedNode struct {
	Node           int    `json:"node"`
	Parent         string `json:"parent,omitempty"`
	ConstraintTier string `json:"constraint_tier,omitempty"`
	Rationale      string `json:"rationale,omitempty"`
	SourceNote     string `json:"source_note,omitempty"`
}

// columnTier is the reader's left-to-right and which nodes stand in each.
// A tier this column draws no node at is absent rather than present and empty.
type columnTier struct {
	Tier  int   `json:"tier"`
	Nodes []int `json:"nodes"`
}

type columnSched struct {
	// Scopes is an array because one document genuinely spans two: fund-flows
	// joins revenue-by-fund and expenditure-by-department. The per-projection
	// files spell this two ways and this normalises it.
	Scopes   []string        `json:"scopes"`
	Headline json.RawMessage `json:"headline,omitempty"`
	Counts   json.RawMessage `json:"counts,omitempty"`
	Caveats  json.RawMessage `json:"caveats,omitempty"`
	Sources  json.RawMessage `json:"sources,omitempty"`
	// Nodes is this schedule's own view of the marks it draws: which nodes of
	// the shared table, and where each hangs in THIS schedule's hierarchy.
	Nodes []columnSchedNode `json:"nodes"`
	Links []columnLink      `json:"links"`
}

// columnLink references its ends by INDEX into the column's node table. That is
// what de-duplicates the node objects, and it is why the table is emitted in a
// stable order rather than a map.
type columnLink struct {
	From       int             `json:"from"`
	To         int             `json:"to"`
	ValueCents int64           `json:"value_cents"`
	Kind       string          `json:"kind"`
	TransferID string          `json:"transfer_id,omitempty"`
	FactIDs    []string        `json:"fact_ids"`
	Locators   json.RawMessage `json:"locators"`
	Derived    bool            `json:"derived,omitempty"`
	Partition  bool            `json:"partition,omitempty"`
}

// decoded is one published document as it sits in Options.Projections, decoded
// far enough to be folded into a column.
type decoded struct {
	Nodes []struct {
		ID             string `json:"id"`
		Label          string `json:"label"`
		Tier           int    `json:"tier"`
		Parent         string `json:"parent"`
		Role           string `json:"role"`
		ConstraintTier string `json:"constraint_tier"`
		Derived        bool   `json:"derived"`
		Rationale      string `json:"rationale"`
		SourceNote     string `json:"source_note"`
	} `json:"nodes"`
	Links []struct {
		Source     string          `json:"source"`
		Target     string          `json:"target"`
		ValueCents int64           `json:"value_cents"`
		Kind       string          `json:"kind"`
		TransferID string          `json:"transfer_id"`
		FactIDs    []string        `json:"fact_ids"`
		Locators   json.RawMessage `json:"locators"`
		Derived    bool            `json:"derived"`
		Partition  bool            `json:"partition"`
	} `json:"links"`
	Metadata struct {
		FiscalYear      int             `json:"fiscal_year"`
		FiscalYearLabel string          `json:"fiscal_year_label"`
		Basis           string          `json:"basis"`
		Scope           string          `json:"scope"`
		Scopes          []string        `json:"scopes"`
		GeneratedBy     string          `json:"generated_by"`
		Headline        json.RawMessage `json:"headline"`
		Counts          json.RawMessage `json:"counts"`
		Caveats         json.RawMessage `json:"caveats"`
		Sources         json.RawMessage `json:"sources"`
	} `json:"metadata"`
}

// columnsOf folds every published document into one document per column.
//
// A DOCUMENT WITH NO COLUMN IS NOT AN ERROR AND IS NOT INCLUDED. revenue-trends,
// fund-balances and changes-in-fund-balances carry a series rather than a graph
// and state no fiscal year or basis at all; a column list cannot express them
// and this does not try.
func columnsOf(projections map[string][]byte) (map[string]columnDoc, error) {
	byColumn := map[string]*columnDoc{}
	index := map[string]map[string]int{}

	for _, doc := range project.PublishedDocuments() {
		raw, ok := projections[doc.Stem]
		if !ok {
			continue
		}
		var d decoded
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("decode %s: %w", doc.Stem, err)
		}
		if d.Metadata.FiscalYear == 0 || d.Metadata.Basis == "" || len(d.Nodes) == 0 {
			continue
		}
		key := columnPath(d.Metadata.FiscalYear, d.Metadata.Basis)
		col, seen := byColumn[key]
		if !seen {
			col = &columnDoc{
				SchemaVersion: columnSchemaVersion,
				GeneratedBy:   d.Metadata.GeneratedBy,
				Column: columnKey{
					FiscalYear: d.Metadata.FiscalYear,
					Basis:      d.Metadata.Basis,
					Label:      d.Metadata.FiscalYearLabel,
				},
				Schedules: map[string]columnSched{},
			}
			byColumn[key] = col
			index[key] = map[string]int{}
		}
		at := index[key]

		// THE NODE TABLE IS SHARED AND THE FIRST WRITER WINS, which is safe
		// only because the shared nodes were measured identical. A second
		// document disagreeing about a node it shares is a real defect, so it
		// is refused here rather than silently taking one of the two.
		drawn := make([]columnSchedNode, 0, len(d.Nodes))
		for _, n := range d.Nodes {
			i, had := at[n.ID]
			node := columnNode{
				ID: n.ID, Label: n.Label, Tier: n.Tier, Role: n.Role, Derived: n.Derived,
			}
			if !had {
				i = len(col.Nodes)
				at[n.ID] = i
				col.Nodes = append(col.Nodes, node)
			} else if col.Nodes[i] != node {
				// THE SHARED HALF IS REFUSED ON DISAGREEMENT rather than
				// resolved. These five fields were measured identical across
				// every schedule of every column; one that stopped being so
				// would mean two schedules mean different things by one id,
				// which no rule here could pick between.
				return nil, fmt.Errorf(
					"column %s: %q disagrees between schedules about the same node: %+v and %+v",
					key, n.ID, col.Nodes[i], node)
			}
			drawn = append(drawn, columnSchedNode{
				Node: i, Parent: n.Parent, ConstraintTier: n.ConstraintTier,
				Rationale: n.Rationale, SourceNote: n.SourceNote,
			})
		}

		links := make([]columnLink, 0, len(d.Links))
		for _, l := range d.Links {
			from, okFrom := at[l.Source]
			to, okTo := at[l.Target]
			if !okFrom || !okTo {
				return nil, fmt.Errorf("column %s, schedule %s: link %s -> %s names a node the document does not carry",
					key, doc.Projection, l.Source, l.Target)
			}
			links = append(links, columnLink{
				From: from, To: to, ValueCents: l.ValueCents, Kind: l.Kind,
				TransferID: l.TransferID, FactIDs: l.FactIDs, Locators: l.Locators,
				Derived: l.Derived, Partition: l.Partition,
			})
		}
		col.Schedules[doc.Projection] = columnSched{
			Nodes:    drawn,
			Scopes:   scopesOf(d),
			Headline: d.Metadata.Headline,
			Counts:   d.Metadata.Counts,
			Caveats:  d.Metadata.Caveats,
			Sources:  d.Metadata.Sources,
			Links:    links,
		}
	}

	out := make(map[string]columnDoc, len(byColumn))
	for key, col := range byColumn {
		col.Tiers = tiersOf(col.Nodes)
		out[key] = *col
	}
	return out, nil
}

// scopesOf normalises the two spellings the published documents use.
func scopesOf(d decoded) []string {
	if len(d.Metadata.Scopes) > 0 {
		return d.Metadata.Scopes
	}
	if d.Metadata.Scope != "" {
		return []string{d.Metadata.Scope}
	}
	return nil
}

// tiersOf is the reader's left-to-right: every tier the column draws a node at,
// ascending, each with its nodes in table order.
func tiersOf(nodes []columnNode) []columnTier {
	at := map[int][]int{}
	for i, n := range nodes {
		at[n.Tier] = append(at[n.Tier], i)
	}
	tiers := make([]int, 0, len(at))
	for t := range at {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	out := make([]columnTier, 0, len(tiers))
	for _, t := range tiers {
		out = append(out, columnTier{Tier: t, Nodes: at[t]})
	}
	return out
}

// columnPath is the file one column ships at.
func columnPath(year int, basis string) string {
	return fmt.Sprintf("fy%d-%s.json", year, basis)
}

// encodeColumn renders one column and REFUSES to return bytes that do not match
// the published schema.
//
// FAIL-CLOSED AT THE BOUNDARY, which is this project's standing rule for
// ambiguity. The alternative is writing a file and discovering at `fisc verify`
// that it was malformed, by which point it has been served.
func encodeColumn(doc columnDoc) ([]byte, error) {
	b, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return nil, fmt.Errorf("encode column: %w", err)
	}
	b = append(b, '\n')

	resolved, err := schema.Load(schema.Column)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("re-read column: %w", err)
	}
	if err := resolved.Validate(v); err != nil {
		return nil, fmt.Errorf("the column this build produced for FY%d %s does not match %s: %w",
			doc.Column.FiscalYear, doc.Column.Basis, schema.Column, err)
	}
	return b, nil
}
