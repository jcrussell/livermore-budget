package export

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"

	"github.com/jcrussell/livermore-budget/schema"
)

// columnSchemaVersion is the version a ColumnDoc declares, spelled once.
const columnSchemaVersion = 1

// ColumnDoc is everything the chart needs for one published column: one node
// table and one link set per printed schedule, keyed by (fiscal year, basis).
//
// The schedules are not merged; why, measured: docs/schema-contracts.md.
type ColumnDoc struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedBy   string                 `json:"generated_by,omitempty"`
	Column        ColumnKey              `json:"column"`
	Nodes         []ColumnNode           `json:"nodes"`
	Tiers         []ColumnTier           `json:"tiers"`
	Schedules     map[string]ColumnSched `json:"schedules"`
}

// ColumnKey names the published column: a fiscal year and a basis.
type ColumnKey struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	Label      string `json:"label"`
}

// ColumnNode carries only the fields every schedule agrees on. Where a node
// hangs is ColumnSchedNode's, because two schedules disagree about it.
type ColumnNode struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Tier    int    `json:"tier"`
	Role    string `json:"role,omitempty"`
	Derived bool   `json:"derived,omitempty"`
}

// ColumnSchedNode is one schedule's view of a node the column carries.
type ColumnSchedNode struct {
	Node           int    `json:"node"`
	Parent         string `json:"parent,omitempty"`
	ConstraintTier string `json:"constraint_tier,omitempty"`
	Rationale      string `json:"rationale,omitempty"`
	SourceNote     string `json:"source_note,omitempty"`
}

// ColumnTier is the reader's left-to-right. A tier with no node is absent.
type ColumnTier struct {
	Tier  int   `json:"tier"`
	Nodes []int `json:"nodes"`
}

// ColumnSched is one printed schedule of a column.
type ColumnSched struct {
	// An array because fund-flows spans two: revenue-by-fund and
	// expenditure-by-department. The per-projection files spell this two ways.
	Scopes   []string        `json:"scopes"`
	Headline json.RawMessage `json:"headline,omitempty"`
	Counts   json.RawMessage `json:"counts,omitempty"`
	Caveats  json.RawMessage `json:"caveats,omitempty"`
	Sources  json.RawMessage `json:"sources,omitempty"`
	// This schedule's own view of the marks it draws.
	Nodes []ColumnSchedNode `json:"nodes"`
	Links []ColumnLink      `json:"links"`
}

// ColumnLink references its ends by index into the node table, which is why
// that table is emitted in a stable order rather than a map.
type ColumnLink struct {
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

// ColumnIndex answers which built document is one schedule of one column:
// [ColumnPath] -> [scheduleKey] -> filename stem.
//
// IT IS THE JOIN THE CLIENT ALREADY MAKES. site/app.js fetches the column the
// year landed and selects a schedule out of it by [DrillStep.Projection], so a
// step's document is fully determined by (column, schedule key). Go resolving
// it any other way is a second answer to a settled question, and the map that
// used to be declared per step could point a year at another year's figures
// while satisfying every arm that guarded it.
//
// Built in [ColumnsOf]'s own loop so the fold and the index cannot disagree
// about which documents are column-shaped.
type ColumnIndex struct {
	schedules map[string]map[string]string
	columns   map[string]string
}

// Stem answers the document at one schedule of one column, and whether the
// column carries that schedule at all.
func (ix ColumnIndex) Stem(column, schedule string) (string, bool) {
	stem, ok := ix.schedules[column][schedule]
	return stem, ok
}

// Column answers which column a built document folded into, and whether it
// folded into one at all -- revenue-trends and the two balance documents state
// no column and are in none.
func (ix ColumnIndex) Column(stem string) (string, bool) {
	col, ok := ix.columns[stem]
	return col, ok
}

// ColumnsOf folds every published document into one document per column.
//
// A document stating no fiscal year or basis is skipped, not refused:
// revenue-trends and the two balance documents carry a series and no column.
func ColumnsOf(projections map[string][]byte) (map[string]ColumnDoc, ColumnIndex, error) {
	byColumn := map[string]*ColumnDoc{}
	index := map[string]map[string]int{}
	ix := ColumnIndex{schedules: map[string]map[string]string{}, columns: map[string]string{}}

	stems := make([]string, 0, len(projections))
	for stem := range projections {
		stems = append(stems, stem)
	}
	sort.Strings(stems)

	for _, stem := range stems {
		raw := projections[stem]
		schedule := scheduleKey(stem)
		var d decoded
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, ColumnIndex{}, fmt.Errorf("decode %s: %w", stem, err)
		}
		if d.Metadata.FiscalYear == 0 || d.Metadata.Basis == "" || len(d.Nodes) == 0 {
			continue
		}
		key := ColumnPath(d.Metadata.FiscalYear, d.Metadata.Basis)
		col, seen := byColumn[key]
		if !seen {
			col = &ColumnDoc{
				SchemaVersion: columnSchemaVersion,
				GeneratedBy:   d.Metadata.GeneratedBy,
				Column: ColumnKey{
					FiscalYear: d.Metadata.FiscalYear,
					Basis:      d.Metadata.Basis,
					Label:      d.Metadata.FiscalYearLabel,
				},
				Schedules: map[string]ColumnSched{},
			}
			byColumn[key] = col
			index[key] = map[string]int{}
			ix.schedules[key] = map[string]string{}
		}
		if was, dup := ix.schedules[key][schedule]; dup {
			// Two stems folding into one slot is a silent overwrite of a
			// whole schedule, and which one won would depend on sort order.
			return nil, ColumnIndex{}, fmt.Errorf(
				"column %s: %q and %q are both schedule %q", key, was, stem, schedule)
		}
		ix.schedules[key][schedule] = stem
		ix.columns[stem] = key
		at := index[key]

		drawn := make([]ColumnSchedNode, 0, len(d.Nodes))
		for _, n := range d.Nodes {
			i, had := at[n.ID]
			node := ColumnNode{
				ID: n.ID, Label: n.Label, Tier: n.Tier, Role: n.Role, Derived: n.Derived,
			}
			if !had {
				i = len(col.Nodes)
				at[n.ID] = i
				col.Nodes = append(col.Nodes, node)
			} else if col.Nodes[i] != node {
				// Two schedules meaning different things by one id is not
				// something a rule here could pick between.
				return nil, ColumnIndex{}, fmt.Errorf(
					"column %s: %q disagrees between schedules about the same node: %+v and %+v",
					key, n.ID, col.Nodes[i], node)
			}
			drawn = append(drawn, ColumnSchedNode{
				Node: i, Parent: n.Parent, ConstraintTier: n.ConstraintTier,
				Rationale: n.Rationale, SourceNote: n.SourceNote,
			})
		}

		links := make([]ColumnLink, 0, len(d.Links))
		for _, l := range d.Links {
			from, okFrom := at[l.Source]
			to, okTo := at[l.Target]
			if !okFrom || !okTo {
				return nil, ColumnIndex{}, fmt.Errorf("column %s, schedule %s: link %s -> %s names a node the document does not carry",
					key, schedule, l.Source, l.Target)
			}
			links = append(links, ColumnLink{
				From: from, To: to, ValueCents: l.ValueCents, Kind: l.Kind,
				TransferID: l.TransferID, FactIDs: l.FactIDs, Locators: l.Locators,
				Derived: l.Derived, Partition: l.Partition,
			})
		}
		col.Schedules[schedule] = ColumnSched{
			Nodes:    drawn,
			Scopes:   scopesOf(d),
			Headline: d.Metadata.Headline,
			Counts:   d.Metadata.Counts,
			Caveats:  d.Metadata.Caveats,
			Sources:  d.Metadata.Sources,
			Links:    links,
		}
	}

	out := make(map[string]ColumnDoc, len(byColumn))
	for key, col := range byColumn {
		col.Tiers = tiersOf(col.Nodes)
		out[key] = *col
	}
	return out, ix, nil
}

// yearSuffix is what project.PublishedStem appends to a projection name.
var yearSuffix = regexp.MustCompile(`-(\d{4})(-actual|-revised)?$`)

// scheduleKey is the schedule a stem's document becomes in its column.
//
// It inverts project.PublishedStem, which appends the year to a projection name
// when a projection publishes more than one column. TestScheduleKeyInvertsThe
// StemRule holds the two together.
func scheduleKey(stem string) string {
	return yearSuffix.ReplaceAllString(stem, "")
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
func tiersOf(nodes []ColumnNode) []ColumnTier {
	at := map[int][]int{}
	for i, n := range nodes {
		at[n.Tier] = append(at[n.Tier], i)
	}
	tiers := make([]int, 0, len(at))
	for t := range at {
		tiers = append(tiers, t)
	}
	sort.Ints(tiers)
	out := make([]ColumnTier, 0, len(tiers))
	for _, t := range tiers {
		out = append(out, ColumnTier{Tier: t, Nodes: at[t]})
	}
	return out
}

// encodeColumn renders one column and refuses bytes that do not match the
// published schema.
func encodeColumn(doc ColumnDoc) ([]byte, error) {
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

// StepStems is the document each declared step draws for one year, in
// declaration order: the step's own schedule where it names one, the step
// before it's where it names none. The first step draws `year` itself if it
// names no schedule of its own.
//
// ONE SPELLING, TWO CALLERS -- stepDocuments here and the rung walk in
// pkg/cmd/export. They were two, and they disagreed: where a step's year
// carried no entry, one resolved "" and the other fell back to the declared
// projection, so the shipped rung answer and the page could name different
// documents for one rung.
//
// A SCHEDULE THE COLUMN DOES NOT CARRY IS REFUSED BY NAME, which is what the
// four arms guarding the old declared map add up to and the one thing they
// could not say: those checked that the packager had written an entry, and
// this checks that the document it names folded into the column a reader will
// actually fetch.
//
// THE COLUMN IS LOOKED UP HERE AND NOT BY THE CALLER, so a year that folded
// into none is a failure only for a step that needs one. A view whose steps
// all draw the document before them selects nothing out of a column, and a
// document stating no fiscal year is still walkable.
func StepStems(steps []DrillStep, year string, ix ColumnIndex) ([]string, error) {
	out := make([]string, len(steps))
	prev := year
	for i, s := range steps {
		if s.Projection != "" {
			column, folded := ix.Column(year)
			if !folded {
				return nil, fmt.Errorf(
					"step %d opens into schedule %q and year %q folded into no column, so "+
						"there is nothing to select that schedule out of",
					i, s.Projection, year)
			}
			stem, ok := ix.Stem(column, s.Projection)
			if !ok {
				return nil, fmt.Errorf(
					"step %d opens into schedule %q and column %s (year %q) carries no such "+
						"schedule; a reader on that year would open a node into nothing",
					i, s.Projection, column, year)
			}
			prev = stem
		}
		out[i] = prev
	}
	return out, nil
}

// DrawnStems is every document this view's steps draw, across every year it
// lists: the schedule each step names, resolved in each year's own column.
//
// ONE SPELLING FOR TWO QUESTIONS -- which documents the caveats page may
// promise a chart flag for, and which documents the published set may call
// reachable. Both used to read [DrillStep]'s declared per-year map, and both
// got the same wrong answer when it was built from projections and year stems
// alone: every reader of the caveats page was told the charts do not flag
// fund-flows' marks, while they do.
//
// A step drawing the document before it adds nothing, because that document
// is already in the list or is the year's own. A year that folded into no
// column contributes nothing rather than failing: [StepStems] is where that
// is refused, by name, for the step that needed it.
func (v View) DrawnStems(ix ColumnIndex) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range v.Steps {
		if s.Projection == "" {
			continue
		}
		for _, year := range slices.Concat([]string{v.Projection}, v.YearStems) {
			column, folded := ix.Column(year)
			if !folded {
				continue
			}
			stem, ok := ix.Stem(column, s.Projection)
			if !ok || seen[stem] {
				continue
			}
			seen[stem] = true
			out = append(out, stem)
		}
	}
	return out
}
