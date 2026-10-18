package export

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// ApartFromSpine is one column where the caller's checks declare pp.85-125's
// department rows apart from pp.66-67's citywide figure, with the declared
// residual and reason. The glance page prints it beside the departments bar
// in place of saying the rows tie; every other column's rows are ones
// `fisc verify` holds to sum to the spine's figure to the cent.
type ApartFromSpine struct {
	FiscalYear    int
	Basis         string
	ResidualCents int64
	Reason        string
	Bead          string
}

// glanceRow is one printed cell behind a bar, as the schedule draws it: the
// link's two ends, its cents and where it was read. Go sums nothing here;
// site/glance.js groups the rows and sums each group.
type glanceRow struct {
	Group      string
	GroupLabel string
	Other      string
	OtherLabel string
	Cents      int64
	Dollars    string
	Facts      string
	Pages      []pageRef
}

// glanceApart is one ApartFromSpine as the page prints it.
type glanceApart struct {
	// Residual is the magnitude; Direction is "less" or "more", read off
	// the sign of ApartFromSpine.ResidualCents, which is what the rows
	// miss the whole by, either way.
	Residual  string
	Direction string
	Reason    string
	Bead      string
}

// glanceBar is one "of every $100" bar: which cells it is drawn from and the
// projection's total printed beside the client's sum of them.
type glanceBar struct {
	ID        string
	Heading   string
	Lede      string
	GroupHead string
	OtherHead string
	// Whole is the projection's headline for this bar's total, its sum over
	// every cell of the bar's kind, and not a printed cell. Rows plus Apart's
	// residuals sum to it, or buildGlancePage refuses the page.
	Whole figure
	Rows  []glanceRow
	// Ties is whether Apart is empty: the rows sum to Whole with no declared
	// residual.
	Ties    bool
	Apart   []glanceApart
	Caveats []caveatRef
}

// glanceYear is one published year's bars.
type glanceYear struct {
	Label   string
	Revenue figure
	Spent   figure
	Bars    []glanceBar
	Caveats []caveatRef
}

// glancePageData is the glance template's input.
type glancePageData struct {
	chrome
	Years []glanceYear
}

// glanceDocument is as much of the spine or of department-funding as the
// glance page reads: the node table for roles and labels, every link with
// its figure and provenance, and the metadata the footer and caveats state.
type glanceDocument struct {
	Projection string `json:"projection"`
	Nodes      []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Role  string `json:"role"`
	} `json:"nodes"`
	Links []struct {
		Source     string       `json:"source"`
		Target     string       `json:"target"`
		ValueCents int64        `json:"value_cents"`
		Derived    bool         `json:"derived"`
		FactIDs    []string     `json:"fact_ids"`
		Locators   []sourceMeta `json:"locators"`
	} `json:"links"`
	Metadata struct {
		GeneratedBy     string           `json:"generated_by"`
		FiscalYear      int              `json:"fiscal_year"`
		FiscalYearLabel string           `json:"fiscal_year_label"`
		Basis           string           `json:"basis"`
		Scopes          []string         `json:"scopes"`
		Sources         []sourceMeta     `json:"sources"`
		Headline        project.Headline `json:"headline"`
		Caveats         []caveatMeta     `json:"caveats"`
	} `json:"metadata"`
}

// decodeGlanceDocument reads one schedule the glance page draws, refusing one
// that is another projection's.
func decodeGlanceDocument(stem, want string, raw []byte) (glanceDocument, error) {
	var d glanceDocument
	if _, err := decodeDocument(stem, raw); err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("decode %s: %w", stem, err)
	}
	if d.Projection != want {
		return d, fmt.Errorf("the glance page reads %s as %s and it is %q", stem, want, d.Projection)
	}
	return d, nil
}

// glanceRows is every link of doc whose ends pick picks, as rows grouped by
// the end pick names. It refuses a link naming a node the table lacks and a
// node with no label, because a row with no words is a cell a reader cannot
// name, and an inferred link, because the page lists every row as a printed
// cell and marks none inferred.
func glanceRows(doc glanceDocument, pick func(src, dst string) (group, other string, ok bool),
	pages func([]sourceMeta) []pageRef,
) ([]glanceRow, error) {
	type node struct{ label, role string }
	table := make(map[string]node, len(doc.Nodes))
	for _, n := range doc.Nodes {
		table[n.ID] = node{n.Label, n.Role}
	}
	var rows []glanceRow
	for _, l := range doc.Links {
		group, other, ok := pick(l.Source, l.Target)
		if !ok {
			continue
		}
		g, gok := table[group]
		o, ook := table[other]
		if !gok || !ook {
			return nil, fmt.Errorf("%s draws %s -> %s and its node table lacks one end",
				doc.Projection, l.Source, l.Target)
		}
		if g.label == "" || o.label == "" {
			return nil, fmt.Errorf("%s draws %s -> %s and one end has no label",
				doc.Projection, l.Source, l.Target)
		}
		if l.Derived {
			return nil, fmt.Errorf("%s draws %s -> %s inferred, and the glance page lists every row as a printed cell",
				doc.Projection, l.Source, l.Target)
		}
		if l.ValueCents <= 0 {
			return nil, fmt.Errorf("%s draws %s -> %s at %d cents; a bar segment is positive",
				doc.Projection, l.Source, l.Target, l.ValueCents)
		}
		rows = append(rows, glanceRow{
			Group: group, GroupLabel: g.label,
			Other: other, OtherLabel: o.label,
			Cents:   l.ValueCents,
			Dollars: amount.Cents(l.ValueCents).Dollars(),
			Facts:   strings.Join(l.FactIDs, " "),
			Pages:   pages(l.Locators),
		})
	}
	return rows, nil
}

// glanceTies refuses a bar whose rows, plus every declared residual, do not
// sum to the figure the page prints beside them. The client's sum is the
// figure a reader sees; this is the check that it is the city's.
func glanceTies(bar glanceBar, whole int64, residuals []ApartFromSpine) error {
	var sum int64
	for _, r := range bar.Rows {
		sum += r.Cents
	}
	for _, a := range residuals {
		sum += a.ResidualCents
	}
	if sum != whole {
		return fmt.Errorf("the %s bar's %d rows and %d declared residuals sum to %s and the page prints %s beside them",
			bar.ID, len(bar.Rows), len(residuals), amount.Cents(sum), amount.Cents(whole))
	}
	return nil
}

// buildGlancePage renders the glance view: per published year, newest first,
// three bars of printed cells with the cited figure each sums to, and the
// declaration where one does not.
func buildGlancePage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string,
) (glancePageData, error) {
	if !slices.Equal(v.Reads, []string{project.DepartmentFundingProjection}) {
		return glancePageData{}, fmt.Errorf("view %q renders the glance page and reads %v "+
			"beside its own document; the page reads %s and nothing else",
			v.Path, v.Reads, project.DepartmentFundingProjection)
	}
	opens, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return glancePageData{}, err
	}
	caveatsPath := caveatsPathOf(o)
	pages := func(locs []sourceMeta) []pageRef {
		var out []pageRef
		for _, s := range unionSources(locs) {
			d := byID[s.DocID]
			base := pageTextBase(s.DocID)
			for _, p := range s.Pages {
				out = append(out, pageRef{Number: p, PDFURL: pdfPageURL(d.PDFURL, p), TextURL: base + pageTextFile(p)})
			}
		}
		return out
	}
	var (
		years []glanceYear
		cited []sourceMeta
	)
	for _, stem := range v.yearStems() {
		spine, err := decodeGlanceDocument(stem, opens.Projection, o.Projections[stem])
		if err != nil {
			return glancePageData{}, err
		}
		m := spine.Metadata
		if strings.Join(m.Scopes, ", ") != meta.Scope() || m.GeneratedBy != meta.GeneratedBy {
			return glancePageData{}, fmt.Errorf(
				"view %q opens on %q (scope %q, built by %q) and its year stem %q is scope %q, built by %q; "+
					"the footer states one of each", v.Path, v.Projection, meta.Scope(), meta.GeneratedBy,
				stem, strings.Join(m.Scopes, ", "), m.GeneratedBy)
		}
		reads, err := v.readStems(stem, ix)
		if err != nil {
			return glancePageData{}, err
		}
		funding, err := decodeGlanceDocument(reads[0], project.DepartmentFundingProjection, o.Projections[reads[0]])
		if err != nil {
			return glancePageData{}, err
		}
		if funding.Metadata.FiscalYear != m.FiscalYear || funding.Metadata.Basis != m.Basis {
			return glancePageData{}, fmt.Errorf("view %q reads %s beside %s and they state different columns",
				v.Path, reads[0], stem)
		}
		if funding.Metadata.GeneratedBy != meta.GeneratedBy {
			return glancePageData{}, fmt.Errorf("view %q reads documents built by %q and %q; the footer credits one projection",
				v.Path, meta.GeneratedBy, funding.Metadata.GeneratedBy)
		}
		cited = append(cited, m.Sources...)
		cited = append(cited, funding.Metadata.Sources...)

		role := func(doc glanceDocument) map[string]string {
			out := make(map[string]string, len(doc.Nodes))
			for _, n := range doc.Nodes {
				out[n.ID] = n.Role
			}
			return out
		}
		spineRole, fundingRole := role(spine), role(funding)
		h := m.Headline
		// The projection's headline, a sum over every revenue or expenditure
		// cell the city prints (headline-ties-to-facts), and not a cell itself:
		// the page says so, and cites no page for it.
		revenue := figure{Label: "Total revenues, all funds, summed from every revenue cell the city prints", Value: amount.Cents(h.AllFundsGrossRevenueCents).Dollars()}
		spent := figure{Label: "Total expenditures, all funds, summed from every expenditure cell the city prints", Value: amount.Cents(h.AllFundsGrossExpenditureCents).Dollars()}

		var apart []ApartFromSpine
		for _, a := range o.DepartmentsApartFromSpine {
			if a.FiscalYear == m.FiscalYear && a.Basis == m.Basis {
				if a.Reason == "" {
					return glancePageData{}, fmt.Errorf("the departments are declared apart from the spine in %s with no reason", stem)
				}
				apart = append(apart, a)
			}
		}
		bars := []struct {
			bar       glanceBar
			doc       glanceDocument
			pick      func(src, dst string) (string, string, bool)
			whole     int64
			residuals []ApartFromSpine
		}{{
			bar: glanceBar{ID: "revenue", Heading: "Where the money comes from",
				Lede: "Of every $100 the city budgets to take in, by the kind of revenue. Money moved " +
					"between the city's own funds, and money drawn from what its funds already hold, is not counted.",
				GroupHead: "Revenue category", OtherHead: "Fund group", Whole: revenue},
			doc: spine,
			pick: func(src, dst string) (string, string, bool) {
				return src, dst, spineRole[src] == project.RoleRevenueSource
			},
			whole: h.AllFundsGrossRevenueCents,
		}, {
			bar: glanceBar{ID: "spending", Heading: "What the money buys",
				Lede: "Of every $100 the city budgets to spend, by what it pays for. Transfers between the " +
					"city's own funds and additions to its reserves are not spending and are left out.",
				GroupHead: "Spending category", OtherHead: "Fund group", Whole: spent},
			doc: spine,
			pick: func(src, dst string) (string, string, bool) {
				return dst, src, spineRole[dst] == project.RoleObjectCategory
			},
			whole: h.AllFundsGrossExpenditureCents,
		}, {
			bar: glanceBar{ID: "departments", Heading: "Who spends it",
				Lede:      "Of every $100 the city budgets to spend, by the department that spends it, every fund together.",
				GroupHead: "Department", OtherHead: "Paid from", Whole: spent},
			doc: funding,
			pick: func(src, dst string) (string, string, bool) {
				return dst, src, fundingRole[dst] == project.RoleWholeDepartment
			},
			whole:     h.AllFundsGrossExpenditureCents,
			residuals: apart,
		}}
		year := glanceYear{
			Label:   ledeOf(m.FiscalYearLabel, m.Basis),
			Revenue: revenue,
			Spent:   spent,
			Caveats: caveatRefs(m.Caveats, stem, caveatsPath),
		}
		for _, b := range bars {
			bar := b.bar
			bar.Rows, err = glanceRows(b.doc, b.pick, pages)
			if err != nil {
				return glancePageData{}, fmt.Errorf("view %q, %s bar: %w", v.Path, bar.ID, err)
			}
			if len(bar.Rows) == 0 {
				return glancePageData{}, fmt.Errorf("view %q draws no cell into its %s bar in %s", v.Path, bar.ID, stem)
			}
			if err := glanceTies(bar, b.whole, b.residuals); err != nil {
				return glancePageData{}, fmt.Errorf("view %q, %s: %w", v.Path, stem, err)
			}
			bar.Ties = len(b.residuals) == 0
			for _, a := range b.residuals {
				direction, magnitude := "less", a.ResidualCents
				if magnitude < 0 {
					direction, magnitude = "more", -magnitude
				}
				bar.Apart = append(bar.Apart, glanceApart{
					Residual: amount.Cents(magnitude).Dollars(), Direction: direction, Reason: a.Reason, Bead: a.Bead})
			}
			if bar.ID == "departments" {
				bar.Caveats = caveatRefs(funding.Metadata.Caveats, reads[0], caveatsPath)
			}
			year.Bars = append(year.Bars, bar)
		}
		// Newest first: the stems are oldest first.
		years = append([]glanceYear{year}, years...)
	}
	sources, _ := sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)
	title := v.Title
	if title == "" {
		title = "Livermore's budget at a glance"
	}
	return glancePageData{
		chrome: chrome{
			Title:        title,
			Lede:         v.Lede,
			Nav:          nav,
			Sources:      sources,
			ProjectionBy: meta.GeneratedBy,
			ExportedBy:   o.GeneratedBy,
			Projections:  projectionRefs(o, ix),
			Scope:        meta.Scope(),
			CaveatsPath:  caveatsPath,
		},
		Years: years,
	}, nil
}
