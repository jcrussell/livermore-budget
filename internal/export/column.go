package export

import (
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/registry"
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
	// Fund is the number the city prints for the fund the node's id names
	// ([project.FundNumber]), absent on a node naming none, so a client never
	// parses an id for it.
	Fund int `json:"fund,omitempty"`
}

// ColumnSchedNode is one schedule's view of a node the column carries: its
// place in that schedule's hierarchy.
type ColumnSchedNode struct {
	Node   int    `json:"node"`
	Parent string `json:"parent,omitempty"`
	// Balances is a fund's printed balances where the schedule cites them.
	Balances *ColumnBalances `json:"balances,omitempty"`
	// Alternative is the expansion schedule's: the key of the alternative
	// split this node is a member of.
	Alternative string `json:"alternative,omitempty"`
}

// ColumnBalances is a fund's printed beginning and ending balance, carried
// from the projection; a blank balance is absent.
type ColumnBalances struct {
	Beginning *ColumnBalance `json:"beginning,omitempty"`
	Ending    *ColumnBalance `json:"ending,omitempty"`
	Change    *ColumnChange  `json:"change,omitempty"`
}

// ColumnChange is a fund's derived change in balance, carried from the
// projection with its words and the two facts it cites.
type ColumnChange struct {
	ValueCents int64           `json:"value_cents"`
	Derived    bool            `json:"derived"`
	Rationale  string          `json:"rationale"`
	SourceNote string          `json:"source_note"`
	FactIDs    []string        `json:"fact_ids"`
	Locators   json.RawMessage `json:"locators"`
}

// ColumnBalance is one printed balance and the fact it cites, its locators
// copied as a link's are.
type ColumnBalance struct {
	ValueCents int64           `json:"value_cents"`
	FactID     string          `json:"fact_id"`
	Locators   json.RawMessage `json:"locators"`
}

// ColumnFundGroup is one fund group this column draws, in the order the page
// lays the fund column out. Slot is the 1-based categorical slot it wears,
// its place in fundGroupDisplayOrder, which site/style.css binds a hue to as
// --fund-slot-<n>; 0 is a group with no slot, drawn muted.
type ColumnFundGroup struct {
	ID   string `json:"id"`
	Slot int    `json:"slot"`
}

// ColumnSched is one printed schedule of a column.
type ColumnSched struct {
	Scopes  []string          `json:"scopes"`
	Counts  json.RawMessage   `json:"counts,omitempty"`
	Caveats json.RawMessage   `json:"caveats,omitempty"`
	Sources json.RawMessage   `json:"sources,omitempty"`
	Nodes   []ColumnSchedNode `json:"nodes"`
	Links   []ColumnLink      `json:"links"`
	// Splits is the expansion schedule's: each way a node opens.
	Splits []ColumnSplit `json:"splits,omitempty"`
}

// ColumnSplit is one way a node of the expansion schedule opens, its node by
// index into the node table; project.Split says what each field means.
type ColumnSplit struct {
	Node        int    `json:"node"`
	Key         string `json:"key"`
	Form        string `json:"form"`
	Alternative bool   `json:"alternative"`
	Side        string `json:"side"`
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
	// Alternative is the expansion schedule's: the key of the alternative
	// split this is a leaf link of.
	Alternative string `json:"alternative,omitempty"`
	// Completes is the expansion schedule's: project.Link's.
	Completes string `json:"completes,omitempty"`
	// Repeats is the expansion schedule's: project.Link's.
	Repeats string `json:"repeats,omitempty"`
	// Restates is the expansion schedule's: project.Link's.
	Restates []string `json:"restates,omitempty"`
}

// decoded is a published document, read far enough to fold into a column.
type decoded struct {
	Projection string `json:"projection"`
	Nodes      []struct {
		ID             string `json:"id"`
		Label          string `json:"label"`
		Tier           int    `json:"tier"`
		Parent         string `json:"parent"`
		Role           string `json:"role"`
		ConstraintTier string `json:"constraint_tier"`
		Derived        bool   `json:"derived"`
		Rationale      string `json:"rationale"`
		SourceNote     string `json:"source_note"`
		// Balances is a fund's printed balances, carried with the schedule.
		Balances    *ColumnBalances `json:"balances"`
		Alternative string          `json:"alternative"`
	} `json:"nodes"`
	Links []struct {
		Source      string          `json:"source"`
		Target      string          `json:"target"`
		ValueCents  int64           `json:"value_cents"`
		Kind        string          `json:"kind"`
		TransferID  string          `json:"transfer_id"`
		FactIDs     []string        `json:"fact_ids"`
		Locators    json.RawMessage `json:"locators"`
		Derived     bool            `json:"derived"`
		Partition   bool            `json:"partition"`
		Contra      string          `json:"contra"`
		Alternative string          `json:"alternative"`
		Completes   string          `json:"completes"`
		Repeats     string          `json:"repeats"`
		Restates    []string        `json:"restates"`
	} `json:"links"`
	Splits []struct {
		Node        string `json:"node"`
		Key         string `json:"key"`
		Form        string `json:"form"`
		Alternative bool   `json:"alternative"`
		Side        string `json:"side"`
	} `json:"splits"`
	Metadata struct {
		FiscalYear      int             `json:"fiscal_year"`
		FiscalYearLabel string          `json:"fiscal_year_label"`
		Basis           string          `json:"basis"`
		Scopes          []string        `json:"scopes"`
		GeneratedBy     string          `json:"generated_by"`
		Counts          json.RawMessage `json:"counts"`
		Caveats         json.RawMessage `json:"caveats"`
		Sources         json.RawMessage `json:"sources"`
	} `json:"metadata"`
}

// ColumnIndex answers which built document is one schedule of one column:
// [ColumnPath] -> the document's own projection name -> filename stem. It is the same join
// site/app.js makes, and is built in [ColumnsOf]'s own loop so the fold and
// the index cannot disagree. It also answers the tiers each built document
// carries a node at, read off the same decode, so a declaration is held
// against the document and not against a second reading of it.
type ColumnIndex struct {
	schedules map[string]map[string]string
	columns   map[string]string
	tiers     map[string][]int
}

// Tiers is every tier a built document carries a node at, ascending, or nil
// for a stem that was not built or states no node.
func (ix ColumnIndex) Tiers(stem string) []int {
	return ix.tiers[stem]
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

// ColumnsOf folds every published document into one document per column.
//
// A document stating no fiscal year or basis is skipped, not refused:
// revenue-trends and the two balance documents carry a series and no column.
func ColumnsOf(projections map[string][]byte, generatedBy string) (map[string]ColumnDoc, ColumnIndex, error) {
	byColumn := map[string]*ColumnDoc{}
	index := map[string]map[string]int{}
	ix := ColumnIndex{
		schedules: map[string]map[string]string{}, columns: map[string]string{},
		tiers: map[string][]int{},
	}

	stems := make([]string, 0, len(projections))
	for stem := range projections {
		stems = append(stems, stem)
	}
	sort.Strings(stems)

	for _, stem := range stems {
		raw := projections[stem]
		var d decoded
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, ColumnIndex{}, fmt.Errorf("decode %s: %w", stem, err)
		}
		// The schedule a document becomes is the projection it says it is.
		schedule := d.Projection
		if schedule == "" && len(d.Nodes) > 0 {
			return nil, ColumnIndex{}, fmt.Errorf("document %s states no projection, so it is no schedule of its column", stem)
		}
		// The tiers the document carries, column or none: a view may draw a
		// document that folded into no column.
		for _, n := range d.Nodes {
			if !slices.Contains(ix.tiers[stem], n.Tier) {
				ix.tiers[stem] = append(ix.tiers[stem], n.Tier)
			}
		}
		slices.Sort(ix.tiers[stem])
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
			// A mark is the client's: a document carrying one would ship a
			// figure nothing cited as though a page printed it.
			if slices.Contains(MarkRoles(), n.Role) || markPrefixOf(n.ID) != "" {
				return nil, ColumnIndex{}, fmt.Errorf(
					"column %s: schedule %q carries node %q in role %q, a mark the client "+
						"makes and no document may ship", key, schedule, n.ID, n.Role)
			}
			i, had := at[n.ID]
			node := ColumnNode{
				ID: n.ID, Label: n.Label, Tier: n.Tier, Role: n.Role, Derived: n.Derived,
				ConstraintTier: n.ConstraintTier, Rationale: n.Rationale, SourceNote: n.SourceNote,
			}
			if fund, ok := project.FundNumber(n.ID); ok {
				node.Fund = fund
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
			drawn = append(drawn, ColumnSchedNode{Node: i, Parent: n.Parent, Balances: n.Balances,
				Alternative: n.Alternative})
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
				Alternative: l.Alternative, Completes: l.Completes, Repeats: l.Repeats,
				Restates: l.Restates,
			})
		}
		var splits []ColumnSplit
		for _, sp := range d.Splits {
			i, ok := at[sp.Node]
			if !ok {
				return nil, ColumnIndex{}, fmt.Errorf("column %s, schedule %s: split of %s names a node the document does not carry",
					key, schedule, sp.Node)
			}
			splits = append(splits, ColumnSplit{Node: i, Key: sp.Key, Form: sp.Form, Alternative: sp.Alternative, Side: sp.Side})
		}
		col.Schedules[schedule] = ColumnSched{
			Nodes:   drawn,
			Scopes:  d.Metadata.Scopes,
			Counts:  d.Metadata.Counts,
			Caveats: d.Metadata.Caveats,
			Sources: d.Metadata.Sources,
			Links:   links,
			Splits:  splits,
		}
	}

	out := make(map[string]ColumnDoc, len(byColumn))
	for key, col := range byColumn {
		groups, err := fundGroupsOf(col.Nodes)
		if err != nil {
			return nil, ColumnIndex{}, fmt.Errorf("column %s: %w", key, err)
		}
		col.FundGroups = groups
		out[key] = *col
	}
	return out, ix, nil
}

// fundGroupDisplayOrder is the fund column top to bottom, by fund-type slug,
// and with it the categorical slot each group wears.
//
// The order is measured: over the colour pairs that touch in this graph its
// worst is CVD dE 9.1 light / 8.4 dark (target 8); sorting by size drops the
// worst dark pair to 6.9. Re-run the dataviz validator over the touching pairs
// before changing it (fisc-y0k). A slug it does not name sorts after, by id.
var fundGroupDisplayOrder = []string{
	registry.FundTypeInternalService,
	registry.FundTypeCapital,
	registry.FundTypeGeneral,
	registry.FundTypeSpecialRevenue,
	registry.FundTypeEnterprise,
	registry.FundTypeDebtService,
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
		out = append(out, ColumnFundGroup{ID: n.ID, Slot: slices.Index(fundGroupDisplayOrder, n.ID[cut+1:]) + 1})
	}
	place := func(g ColumnFundGroup) int {
		if g.Slot > 0 {
			return g.Slot - 1
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
		return nil, fmt.Errorf("the column this build produced for %s does not match %s: %w",
			fact.ColumnLabel(doc.Column.FiscalYear, doc.Column.Basis), schema.Column, err)
	}
	return b, nil
}

// ReachedStems is every document a reader of this view can reach, once each:
// its projection, its year stems, and the expansion schedule of each year's
// column with every document of that column the expansion is stitched from
// ([project.ExpansionStitches]), whose links are the expansion's leaves. It
// refuses a projection outside the year stems, as validate does, since it
// runs on views validate has not seen. The caveats page's drawn set and
// `fisc export`'s reachability gate both read it, so the two cannot disagree.
//
// A [ChangeTemplate] view also reaches [project.TrendsProjection], whose
// columns buildChangePage renders beside the view's own document.
func (v View) ReachedStems(ix ColumnIndex) ([]string, error) {
	if err := v.projectionAmongYears(); err != nil {
		return nil, err
	}
	var out []string
	if v.Template == ChangeTemplate {
		out = append(out, project.TrendsProjection)
	}
	for _, year := range v.yearStems() {
		reached := []string{year}
		if column, folded := ix.Column(year); folded && v.Overview.Form == SankeyForm {
			if stem, ok := ix.Stem(column, project.ExpansionProjection); ok {
				reached = append(reached, stem)
				for _, p := range project.ExpansionStitches() {
					if stitched, ok := ix.Stem(column, p); ok {
						reached = append(reached, stitched)
					}
				}
			}
		}
		beside, err := v.readStems(year, ix)
		if err != nil {
			return nil, err
		}
		reached = append(reached, beside...)
		for _, stem := range reached {
			if stem != "" && !slices.Contains(out, stem) {
				out = append(out, stem)
			}
		}
	}
	return out, nil
}

// readStems is the document of each of [View.Reads] in one year's column,
// refusing a year whose column carries none of one: the page would read a
// schedule that is not there.
func (v View) readStems(year string, ix ColumnIndex) ([]string, error) {
	out := make([]string, 0, len(v.Reads))
	for _, schedule := range v.Reads {
		col, folded := ix.Column(year)
		stem, ok := ix.Stem(col, schedule)
		if !folded || !ok {
			return nil, fmt.Errorf("view %q reads schedule %q, and %q, a year it lists, is of a "+
				"column that carries none", v.Path, schedule, year)
		}
		out = append(out, stem)
	}
	return out, nil
}
