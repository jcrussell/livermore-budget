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

// columnDoc is everything the chart needs for one published column: one node
// table and one link set per printed schedule, keyed by (fiscal year, basis).
//
// The schedules are not merged; why, measured: docs/schema-contracts.md.
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

// columnNode carries only the fields every schedule agrees on. Where a node
// hangs is columnSchedNode's, because two schedules disagree about it.
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

// columnTier is the reader's left-to-right. A tier with no node is absent.
type columnTier struct {
	Tier  int   `json:"tier"`
	Nodes []int `json:"nodes"`
}

type columnSched struct {
	// An array because fund-flows spans two: revenue-by-fund and
	// expenditure-by-department. The per-projection files spell this two ways.
	Scopes   []string        `json:"scopes"`
	Headline json.RawMessage `json:"headline,omitempty"`
	Counts   json.RawMessage `json:"counts,omitempty"`
	Caveats  json.RawMessage `json:"caveats,omitempty"`
	Sources  json.RawMessage `json:"sources,omitempty"`
	// This schedule's own view of the marks it draws.
	Nodes []columnSchedNode `json:"nodes"`
	Links []columnLink      `json:"links"`
}

// columnLink references its ends by index into the node table, which is why
// that table is emitted in a stable order rather than a map.
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

// decoded is a published document, read far enough to fold into a column.
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
// A document stating no fiscal year or basis is skipped, not refused:
// revenue-trends and the two balance documents carry a series and no column.
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
				// Two schedules meaning different things by one id is not
				// something a rule here could pick between.
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

// encodeColumn renders one column and refuses bytes that do not match the
// published schema.
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
