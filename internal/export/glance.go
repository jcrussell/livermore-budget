package export

import (
	"cmp"
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
	// Residual is the magnitude; Direction is "less" or "more", by the sign
	// of ApartFromSpine.ResidualCents.
	Residual  string
	Direction string
	Reason    string
}

// glanceBar is one bar of shares: which cells it is drawn from and the
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
	Ties  bool
	Apart []glanceApart
}

// glanceYear is one published year's bars.
type glanceYear struct {
	// Key is the year's [YearKey], which the year select names it by.
	Key     string
	Label   string
	Revenue figure
	Spent   figure
	Bars    []glanceBar
}

// glancePageData is the glance template's input.
type glancePageData struct {
	chrome
	Years []glanceYear
}

// glanceDocument is as much of the spine or of department-funding as the
// glance page reads.
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
	} `json:"metadata"`
}

// glanceRows is every link of doc whose ends pick picks, as rows grouped by
// the end pick names. It refuses an end the table lacks or that has no label,
// and an inferred link: the page lists every row as a printed cell.
func glanceRows(doc glanceDocument, pick func(src, dst string) (group, other string, ok bool),
	cite *cellCites,
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
		pages, err := cite.locators(l.Locators)
		if err != nil {
			return nil, fmt.Errorf("%s draws %s -> %s: %w", doc.Projection, l.Source, l.Target, err)
		}
		rows = append(rows, glanceRow{
			Group: group, GroupLabel: g.label,
			Other: other, OtherLabel: o.label,
			Cents:   l.ValueCents,
			Dollars: amount.Cents(l.ValueCents).Dollars(),
			Facts:   strings.Join(l.FactIDs, " "),
			Pages:   pages,
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
func buildGlancePage(p pageCtx) (glancePageData, error) {
	o, v, ix := p.o, p.v, p.ix
	if !slices.Equal(v.Reads, []string{project.DepartmentFundingProjection}) {
		return glancePageData{}, fmt.Errorf("view %q renders the glance page and reads %v "+
			"beside its own document; the page reads %s and nothing else",
			v.Path, v.Reads, project.DepartmentFundingProjection)
	}
	opens, meta, err := decodeSankey(v.Projection, o.Projections[v.Projection])
	if err != nil {
		return glancePageData{}, err
	}
	cite := newCellCites(p.byID, p.pageTextBase)
	var (
		years []glanceYear
		cited []sourceMeta
		sel   yearSelect
	)
	for _, stem := range v.yearStems() {
		spine, spineErr := decodeProjection[glanceDocument]("glance", stem, opens.Projection, o.Projections[stem])
		if spineErr != nil {
			return glancePageData{}, spineErr
		}
		m := spine.Metadata
		if strings.Join(m.Scopes, ", ") != meta.Scope() || m.GeneratedBy != meta.GeneratedBy {
			return glancePageData{}, fmt.Errorf(
				"view %q opens on %q (scope %q, built by %q) and its year stem %q is scope %q, built by %q; "+
					"one view is one scope and one build", v.Path, v.Projection, meta.Scope(), meta.GeneratedBy,
				stem, strings.Join(m.Scopes, ", "), m.GeneratedBy)
		}
		reads, readErr := v.readStems(stem, ix)
		if readErr != nil {
			return glancePageData{}, readErr
		}
		funding, fundingErr := decodeProjection[glanceDocument]("glance", reads[0],
			project.DepartmentFundingProjection, o.Projections[reads[0]])
		if fundingErr != nil {
			return glancePageData{}, fundingErr
		}
		if funding.Metadata.FiscalYear != m.FiscalYear || funding.Metadata.Basis != m.Basis {
			return glancePageData{}, fmt.Errorf("view %q reads %s beside %s and they state different columns",
				v.Path, reads[0], stem)
		}
		if funding.Metadata.GeneratedBy != meta.GeneratedBy {
			return glancePageData{}, fmt.Errorf("view %q reads documents built by %q and %q; one view is one build",
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
				Lede: "What the city budgets to take in, by the kind of revenue, each kind's share of the whole. Money moved " +
					"between the city's own funds, and money drawn from what its funds already hold, is not counted.",
				GroupHead: "Revenue category", OtherHead: "Fund group", Whole: revenue},
			doc: spine,
			pick: func(src, dst string) (string, string, bool) {
				return src, dst, spineRole[src] == project.RoleRevenueSource
			},
			whole: h.AllFundsGrossRevenueCents,
		}, {
			bar: glanceBar{ID: "spending", Heading: "What the money buys",
				Lede: "What the city budgets to spend, by what it pays for, each kind's share of the whole. Transfers between the " +
					"city's own funds and additions to its reserves are not spending and are left out.",
				GroupHead: "Spending category", OtherHead: "Fund group", Whole: spent},
			doc: spine,
			pick: func(src, dst string) (string, string, bool) {
				return dst, src, spineRole[dst] == project.RoleObjectCategory
			},
			whole: h.AllFundsGrossExpenditureCents,
		}, {
			bar: glanceBar{ID: "departments", Heading: "Who spends it",
				Lede:      "What the city budgets to spend, by the department that spends it, each department's share of the whole, every fund together.",
				GroupHead: "Department", OtherHead: "Paid from", Whole: spent},
			doc: funding,
			pick: func(src, dst string) (string, string, bool) {
				return dst, src, fundingRole[dst] == project.RoleWholeDepartment
			},
			whole:     h.AllFundsGrossExpenditureCents,
			residuals: apart,
		}}
		sel.newest(m.FiscalYear, m.FiscalYearLabel, m.Basis)
		year := glanceYear{
			Key:     YearKey(m.FiscalYear, m.Basis),
			Label:   ledeOf(m.FiscalYearLabel, m.Basis),
			Revenue: revenue,
			Spent:   spent,
		}
		for _, b := range bars {
			bar := b.bar
			bar.Rows, err = glanceRows(b.doc, b.pick, cite)
			if err != nil {
				return glancePageData{}, fmt.Errorf("view %q, %s bar: %w", v.Path, bar.ID, err)
			}
			if len(bar.Rows) == 0 {
				return glancePageData{}, fmt.Errorf("view %q draws no cell into its %s bar in %s", v.Path, bar.ID, stem)
			}
			if err = glanceTies(bar, b.whole, b.residuals); err != nil {
				return glancePageData{}, fmt.Errorf("view %q, %s: %w", v.Path, stem, err)
			}
			bar.Ties = len(b.residuals) == 0
			for _, a := range b.residuals {
				direction, magnitude := "less", a.ResidualCents
				if magnitude < 0 {
					direction, magnitude = "more", -magnitude
				}
				bar.Apart = append(bar.Apart, glanceApart{
					Residual: amount.Cents(magnitude).Dollars(), Direction: direction, Reason: a.Reason})
			}
			year.Bars = append(year.Bars, bar)
		}
		// Newest first: the stems are oldest first.
		years = append([]glanceYear{year}, years...)
	}
	ch, _, err := p.chrome(chrome{
		Title:      cmp.Or(v.Title, "Livermore's budget at a glance"),
		Scope:      meta.Scope(),
		YearSelect: sel,
	}, cited, cite)
	if err != nil {
		return glancePageData{}, err
	}
	return glancePageData{chrome: ch, Years: years}, nil
}
