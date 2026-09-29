package export

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// columnSchemaVersion is the version a ColumnDoc declares.
const columnSchemaVersion = 1

// ColumnDoc is everything the chart needs for one published column: one node
// table, and per printed schedule its links and its parent edges, keyed by
// (fiscal year, basis).
//
// The schedules are not merged; why, measured: docs/schema-contracts.md.
type ColumnDoc struct {
	SchemaVersion int                    `json:"schema_version"`
	GeneratedBy   string                 `json:"generated_by,omitempty"`
	Column        ColumnKey              `json:"column"`
	Nodes         []ColumnNode           `json:"nodes"`
	Tiers         []ColumnTier           `json:"tiers"`
	FundGroups    []ColumnFundGroup      `json:"fund_groups"`
	Schedules     map[string]ColumnSched `json:"schedules"`
}

// ColumnKey names the published column: a fiscal year and a basis.
type ColumnKey struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	Label      string `json:"label"`
}

// ColumnNode is a node's identity and its own annotations, stated once for
// the column: every schedule drawing it must agree on every field, and
// ColumnsOf refuses one that does not rather than merging. Where a node hangs
// is ColumnSchedNode's, because two schedules legitimately disagree about it.
type ColumnNode struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Tier    int    `json:"tier"`
	Role    string `json:"role,omitempty"`
	Derived bool   `json:"derived,omitempty"`
	// ConstraintTier, Rationale and SourceNote are the node's: a fund's tier
	// read from data/funds.yaml, or a derived node's words.
	ConstraintTier string `json:"constraint_tier,omitempty"`
	Rationale      string `json:"rationale,omitempty"`
	SourceNote     string `json:"source_note,omitempty"`
}

// ColumnSchedNode is one schedule's view of a node the column carries: its
// place in that schedule's hierarchy.
type ColumnSchedNode struct {
	Node   int    `json:"node"`
	Parent string `json:"parent,omitempty"`
}

// ColumnFundGroup is one fund group this column draws, in the order the page
// lays the fund column out. Slug is the key site/style.css binds a hue to, so
// the client never parses an id.
type ColumnFundGroup struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

// ColumnTier is the reader's left-to-right. A tier with no node is absent.
type ColumnTier struct {
	Tier  int   `json:"tier"`
	Nodes []int `json:"nodes"`
}

// ColumnSched is one printed schedule of a column.
type ColumnSched struct {
	Scopes   []string          `json:"scopes"`
	Headline json.RawMessage   `json:"headline,omitempty"`
	Counts   json.RawMessage   `json:"counts,omitempty"`
	Caveats  json.RawMessage   `json:"caveats,omitempty"`
	Sources  json.RawMessage   `json:"sources,omitempty"`
	Nodes    []ColumnSchedNode `json:"nodes"`
	Links    []ColumnLink      `json:"links"`
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
	// Contra is carried from the projection so the page never composes it
	// from a parent the fold has already blanked.
	Contra string `json:"contra,omitempty"`
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
		Contra     string          `json:"contra"`
	} `json:"links"`
	Metadata struct {
		FiscalYear      int             `json:"fiscal_year"`
		FiscalYearLabel string          `json:"fiscal_year_label"`
		Basis           string          `json:"basis"`
		Scopes          []string        `json:"scopes"`
		GeneratedBy     string          `json:"generated_by"`
		Headline        json.RawMessage `json:"headline"`
		Counts          json.RawMessage `json:"counts"`
		Caveats         json.RawMessage `json:"caveats"`
		Sources         json.RawMessage `json:"sources"`
	} `json:"metadata"`
}

// ColumnIndex answers which built document is one schedule of one column:
// [ColumnPath] -> [scheduleKey] -> filename stem. It is the same join
// site/app.js makes, and is built in [ColumnsOf]'s own loop so the fold and
// the index cannot disagree.
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

// PublishedPath is where a reader fetches a built document: the column it
// folded into, or its own file under data/ where it folded into none. The
// write plan, the footer and the caveats page all read it, so none can link a
// path the site does not write.
func (ix ColumnIndex) PublishedPath(stem string) string {
	if column, folded := ix.Column(stem); folded {
		return column
	}
	return path.Join(dataDir, stem+".json")
}

// Column answers which column a built document folded into, and whether it
// folded into one at all -- revenue-trends and the two balance documents state
// no column and are in none.
func (ix ColumnIndex) Column(stem string) (string, bool) {
	col, ok := ix.columns[stem]
	return col, ok
}

// ColumnKeyOf is the column a built document states itself of, refusing one
// that states no fiscal year, basis or label.
func ColumnKeyOf(raw []byte) (ColumnKey, error) {
	var d decoded
	if err := json.Unmarshal(raw, &d); err != nil {
		return ColumnKey{}, fmt.Errorf("decode column: %w", err)
	}
	m := d.Metadata
	if m.FiscalYear == 0 || m.Basis == "" || m.FiscalYearLabel == "" {
		return ColumnKey{}, fmt.Errorf("the document states no whole column: fiscal year %d, basis %q, label %q",
			m.FiscalYear, m.Basis, m.FiscalYearLabel)
	}
	return ColumnKey{FiscalYear: m.FiscalYear, Basis: m.Basis, Label: m.FiscalYearLabel}, nil
}

// ColumnsOf folds every published document into one document per column.
//
// A document stating no fiscal year or basis is skipped, not refused:
// revenue-trends and the two balance documents carry a series and no column.
func ColumnsOf(projections map[string][]byte, generatedBy string) (map[string]ColumnDoc, ColumnIndex, error) {
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
				// The export's stamp, not the projection's: loadColumn in
				// site/app.js compares it against CONFIG.exported_by to catch
				// a cached column beside a fresh app.js.
				GeneratedBy: generatedBy,
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
				ConstraintTier: n.ConstraintTier, Rationale: n.Rationale, SourceNote: n.SourceNote,
			}
			if !had {
				i = len(col.Nodes)
				at[n.ID] = i
				col.Nodes = append(col.Nodes, node)
			} else if col.Nodes[i] != node {
				return nil, ColumnIndex{}, fmt.Errorf(
					"column %s: %q disagrees between schedules about the same node: %+v and %+v",
					key, n.ID, col.Nodes[i], node)
			}
			drawn = append(drawn, ColumnSchedNode{Node: i, Parent: n.Parent})
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
				Derived: l.Derived, Partition: l.Partition, Contra: l.Contra,
			})
		}
		col.Schedules[schedule] = ColumnSched{
			Nodes:    drawn,
			Scopes:   d.Metadata.Scopes,
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
		groups, err := fundGroupsOf(col.Nodes)
		if err != nil {
			return nil, ColumnIndex{}, fmt.Errorf("column %s: %w", key, err)
		}
		col.FundGroups = groups
		out[key] = *col
	}
	return out, ix, nil
}

// yearSuffix is what project.PublishedStem appends to a projection name.
var yearSuffix = regexp.MustCompile(`-(\d{4})(-actual|-revised)?$`)

// scheduleKey is the schedule a stem's document becomes in its column. It
// inverts project.PublishedStem and nothing holds the two together (fisc-9akl).
func scheduleKey(stem string) string {
	return yearSuffix.ReplaceAllString(stem, "")
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

// fundGroupDisplayOrder is the fund column top to bottom, by fund-type slug,
// and with it the categorical slot each group wears.
//
// The order is measured: over the colour pairs that touch in this graph its
// worst is CVD dE 9.1 light / 8.4 dark (target 8); sorting by size drops the
// worst dark pair to 6.9. Re-run the dataviz validator over the touching pairs
// before changing it (fisc-y0k). A slug it does not name sorts after, by id.
var fundGroupDisplayOrder = []string{
	"internal-service",
	"capital",
	"general",
	"special-revenue",
	"enterprise",
	"debt-service",
}

// fundGroupsOf is the fund groups this column draws, ordered for the page.
func fundGroupsOf(nodes []ColumnNode) ([]ColumnFundGroup, error) {
	out := []ColumnFundGroup{}
	for _, n := range nodes {
		if n.Role != project.RoleFundGroup {
			continue
		}
		// An id with no form is refused: the whole id as a slug would draw
		// the group muted with no other symptom.
		cut := strings.Index(n.ID, "/")
		if cut < 0 || cut == len(n.ID)-1 {
			return nil, fmt.Errorf("node %q is role %s and its id names no fund type", n.ID, project.RoleFundGroup)
		}
		out = append(out, ColumnFundGroup{ID: n.ID, Slug: n.ID[cut+1:]})
	}
	place := func(g ColumnFundGroup) int {
		if i := slices.Index(fundGroupDisplayOrder, g.Slug); i >= 0 {
			return i
		}
		return len(fundGroupDisplayOrder)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if p, q := place(out[i]), place(out[j]); p != q {
			return p < q
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// encodeColumn renders one column and refuses bytes that do not match the
// published schema, which states that a derived node carries its words.
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
// declaration order: the step's own schedule where it names one, else the
// document of the chart its After opens from (`year` itself for ""), as
// site/app.js's stepDocument resolves it. A schedule the year's column does
// not carry is refused by name; a year in no column fails only a step that
// selects a schedule.
func StepStems(steps []DrillStep, year string, ix ColumnIndex) ([]string, error) {
	out := make([]string, len(steps))
	index := make(map[string]int, len(steps))
	for i, s := range steps {
		index[s.Key] = i
	}
	for i, s := range steps {
		if s.Projection == "" {
			doc := ""
			for _, a := range s.After {
				parent := year
				if a != "" {
					j, ok := index[a]
					if !ok || j >= i {
						return nil, fmt.Errorf("step %d opens from %q, which names no earlier step", i, a)
					}
					parent = out[j]
				}
				if doc != "" && parent != doc {
					return nil, fmt.Errorf("step %d names no schedule and opens from charts drawing %q and %q", i, doc, parent)
				}
				doc = parent
			}
			if doc == "" {
				return nil, fmt.Errorf("step %d names no schedule and opens from no chart", i)
			}
			out[i] = doc
			continue
		}
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
		out[i] = stem
	}
	return out, nil
}

// DrawnStems is every document this view's steps draw, across every year it
// lists: the schedule each step names, resolved in each year's own column.
// A year in no column contributes nothing; [StepStems] refuses that.
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
