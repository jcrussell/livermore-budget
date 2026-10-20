package export

import (
	"encoding/json"
	"fmt"
	"html/template"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/schema"
)

// Department is one department as [Options.Departments] states it: its id in
// department-funding, its label, and every division pp.85-125 print under it.
type Department struct {
	ID        string
	Label     string
	Divisions []Division
}

// Division is one division of a [Department]: its id in department-spending
// and its label.
type Division struct {
	ID    string
	Label string
}

// HeldApart is one cell of [Options.DepartmentsHeldApart]: a department in a
// column where pp.85-125's two blocks are declared not to tie, and the
// declaration's reason.
type HeldApart struct {
	Department string
	FiscalYear int
	Basis      string
	Reason     string
}

// departmentConfigVersion is the version a department config declares.
const departmentConfigVersion = 1

// departmentConfig is what site/department.js reads from the page, shaped by
// schema/department.schema.json. It carries no figure: every figure is a link
// of the column a year names.
type departmentConfig struct {
	SchemaVersion int                  `json:"schema_version"`
	ExportedBy    string               `json:"exported_by"`
	Spending      string               `json:"spending"`
	Funding       string               `json:"funding"`
	Years         []departmentYear     `json:"years"`
	Departments   []departmentEntry    `json:"departments"`
	Docs          map[string]clientDoc `json:"docs"`
	KindLabels    map[string]string    `json:"kind_labels"`
}

// departmentYear is one column the page opens, and per department whether its
// two sides tie there.
type departmentYear struct {
	Path  string           `json:"path"`
	Label string           `json:"label"`
	Basis string           `json:"basis"`
	Lede  string           `json:"lede"`
	Sides []departmentSide `json:"sides"`
}

// departmentSide says of one department in one column whether its two blocks
// tie to the cent, as cuts-tie-along-the-lattice holds them, or are held apart
// by a declared exception. A declaration and not a comparison: the client
// prints it and computes no difference.
type departmentSide struct {
	Department string `json:"department"`
	Ties       bool   `json:"ties"`
	Reason     string `json:"reason,omitempty"`
}

// departmentEntry is one department the bars and the dropdown offer.
type departmentEntry struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	Divisions []departmentMember `json:"divisions"`
}

// departmentMember is one division the spending side sums over.
type departmentMember struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// departmentPageData is the department template's input.
type departmentPageData struct {
	chrome
	Years       []departmentYear
	Departments []departmentEntry
	// ConfigJSON is the page's config. json.Marshal escapes <, > and &, so the
	// blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// departmentDocument is as much of department-spending or department-funding
// as the page is vetted over.
type departmentDocument struct {
	Projection string `json:"projection"`
	Nodes      []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Role   string `json:"role"`
		Parent string `json:"parent"`
	} `json:"nodes"`
	Links []struct {
		Source     string           `json:"source"`
		Target     string           `json:"target"`
		ValueCents int64            `json:"value_cents"`
		Kind       project.LinkKind `json:"kind"`
	} `json:"links"`
	Metadata struct {
		GeneratedBy     string       `json:"generated_by"`
		FiscalYear      int          `json:"fiscal_year"`
		FiscalYearLabel string       `json:"fiscal_year_label"`
		Basis           string       `json:"basis"`
		Scopes          []string     `json:"scopes"`
		Sources         []sourceMeta `json:"sources"`
	} `json:"metadata"`
}

// decodeDepartmentDocument reads one of the page's two schedules, refusing one
// another projection built.
func decodeDepartmentDocument(stem, want string, raw []byte) (departmentDocument, error) {
	var d departmentDocument
	if _, err := decodeDocument(stem, raw); err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("decode %s: %w", stem, err)
	}
	if d.Projection != want {
		return d, fmt.Errorf("the department page reads %s as %s and it is %q", stem, want, d.Projection)
	}
	return d, nil
}

// buildDepartmentPage renders the department view: every department as a bar
// of the fund groups paying it, and a dropdown of departments, each read out
// of every year's column on two sides -- department-funding's links into the
// department, and department-spending's into its divisions.
//
// It refuses what would make a side less than the department, in any year:
// a drawn department or division the membership does not file exactly once,
// a department with nothing on either side, a division of the membership the
// spending side draws nothing for, a paying fund under no fund group, and a
// label the two disagree on.
func buildDepartmentPage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(string) string, cx *caveatIndex,
) (departmentPageData, error) {
	if !slices.Equal(v.Reads, []string{project.DepartmentSpendingProjection}) {
		return departmentPageData{}, fmt.Errorf("view %q renders the department page and reads %v "+
			"beside its own document; the page reads %s and nothing else",
			v.Path, v.Reads, project.DepartmentSpendingProjection)
	}
	entries, ofDivision, err := departmentEntries(o.Departments)
	if err != nil {
		return departmentPageData{}, fmt.Errorf("view %q: %w", v.Path, err)
	}
	held := map[string]HeldApart{}
	for _, h := range o.DepartmentsHeldApart {
		key := heldKey(h.Department, h.FiscalYear, h.Basis)
		if _, dup := held[key]; dup {
			return departmentPageData{}, fmt.Errorf("%s is held apart in %s twice", h.Department,
				ColumnPath(h.FiscalYear, h.Basis))
		}
		if h.Reason == "" {
			return departmentPageData{}, fmt.Errorf("%s is held apart in %s with no reason", h.Department,
				ColumnPath(h.FiscalYear, h.Basis))
		}
		held[key] = h
	}

	var (
		years    []departmentYear
		sel      yearSelect
		cited    []sourceMeta
		builtBy  string
		scope    string
		consumed = map[string]bool{}
	)
	for _, stem := range v.yearStems() {
		reads, readErr := v.readStems(stem, ix)
		if readErr != nil {
			return departmentPageData{}, readErr
		}
		funding, fundErr := decodeDepartmentDocument(stem, project.DepartmentFundingProjection, o.Projections[stem])
		if fundErr != nil {
			return departmentPageData{}, fundErr
		}
		spendStem := reads[0]
		spending, spendErr := decodeDepartmentDocument(spendStem, project.DepartmentSpendingProjection,
			o.Projections[spendStem])
		if spendErr != nil {
			return departmentPageData{}, spendErr
		}
		fm, sm := funding.Metadata, spending.Metadata
		if builtBy == "" {
			builtBy = fm.GeneratedBy
		}
		for _, by := range []string{fm.GeneratedBy, sm.GeneratedBy} {
			if by != builtBy {
				return departmentPageData{}, fmt.Errorf("view %q reads documents built by %q and %q; "+
					"one view is one build", v.Path, builtBy, by)
			}
		}
		if vetErr := vetDepartmentColumn(entries, ofDivision, stem, funding, spendStem, spending); vetErr != nil {
			return departmentPageData{}, vetErr
		}

		sides := make([]departmentSide, 0, len(entries))
		for _, e := range entries {
			side := departmentSide{Department: e.ID, Ties: true}
			key := heldKey(e.ID, fm.FiscalYear, fm.Basis)
			if h, ok := held[key]; ok {
				side = departmentSide{Department: e.ID, Reason: h.Reason}
				consumed[key] = true
			}
			sides = append(sides, side)
		}
		sel.newest(fm.FiscalYear, fm.Basis, ledeOf(fm.FiscalYearLabel, fm.Basis))
		years = append(years, departmentYear{
			Path:  ColumnPath(fm.FiscalYear, fm.Basis),
			Label: fm.FiscalYearLabel,
			Basis: fm.Basis,
			Lede:  ledeOf(fm.FiscalYearLabel, fm.Basis),
			Sides: sides,
		})
		cited = append(cited, fm.Sources...)
		cited = append(cited, sm.Sources...)
		scope = strings.Join(append(slices.Clone(fm.Scopes), sm.Scopes...), ", ")
	}
	if len(years) == 0 {
		return departmentPageData{}, fmt.Errorf("view %q opens no year", v.Path)
	}
	// A declared cell no year consumed names a department or a column the
	// page does not show, and is stale.
	for key, h := range held {
		if !consumed[key] {
			return departmentPageData{}, fmt.Errorf("%s is held apart in %s, which the department "+
				"page does not open", h.Department, ColumnPath(h.FiscalYear, h.Basis))
		}
	}

	sources, sourcesPath, docs, err := citedSources(o, v.Path, cited, byID, pageTextBase)
	if err != nil {
		return departmentPageData{}, err
	}
	blob, err := encodeDepartmentConfig(departmentConfig{
		SchemaVersion: departmentConfigVersion,
		ExportedBy:    o.GeneratedBy,
		Spending:      project.DepartmentSpendingProjection,
		Funding:       project.DepartmentFundingProjection,
		Years:         years,
		Departments:   entries,
		Docs:          docs,
		KindLabels:    kindLabels(),
	})
	if err != nil {
		return departmentPageData{}, err
	}
	open := years[len(years)-1]
	title := v.Title
	if title == "" {
		title = "Where Livermore's money goes, department by department"
	}
	return departmentPageData{
		chrome: chrome{
			Title:       title,
			Lede:        v.Lede,
			Nav:         nav,
			Sources:     sources,
			SourcesPath: sourcesPath,
			ExportedBy:  o.GeneratedBy,
			Projections: projectionRefs(o, ix),
			DataPath:    open.Path,
			Scope:       scope,
			CaveatsHere: cx.here(v.Path),
			YearSelect:  sel,
		},
		Years:       years,
		Departments: entries,
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and &, so it cannot terminate the script element it sits in.
		ConfigJSON: template.JS(blob),
	}, nil
}

// heldKey addresses one department in one column.
func heldKey(department string, year int, basis string) string {
	return department + "@" + ColumnPath(year, basis)
}

// departmentEntries is the membership as the config states it, and each
// division's department, refusing an empty membership, a department with no
// division, and an id stated twice.
func departmentEntries(ds []Department) ([]departmentEntry, map[string]string, error) {
	if len(ds) == 0 {
		return nil, nil, fmt.Errorf("the department page is given no departments")
	}
	out := make([]departmentEntry, 0, len(ds))
	ofDivision := map[string]string{}
	seen := map[string]bool{}
	for _, d := range ds {
		switch {
		case !strings.HasPrefix(d.ID, project.PrefixDepartment) || d.Label == "":
			return nil, nil, fmt.Errorf("department %q (%q) is not a labelled %s id", d.ID, d.Label,
				project.PrefixDepartment)
		case seen[d.ID]:
			return nil, nil, fmt.Errorf("department %q is stated twice", d.ID)
		case len(d.Divisions) == 0:
			return nil, nil, fmt.Errorf("department %q is stated with no division", d.ID)
		}
		seen[d.ID] = true
		e := departmentEntry{ID: d.ID, Label: d.Label, Divisions: make([]departmentMember, 0, len(d.Divisions))}
		for _, dv := range d.Divisions {
			if !strings.HasPrefix(dv.ID, project.PrefixDept) || dv.Label == "" {
				return nil, nil, fmt.Errorf("division %q (%q) of %s is not a labelled %s id", dv.ID,
					dv.Label, d.ID, project.PrefixDept)
			}
			if was, dup := ofDivision[dv.ID]; dup {
				return nil, nil, fmt.Errorf("division %q is filed under %s and %s", dv.ID, was, d.ID)
			}
			ofDivision[dv.ID] = d.ID
			e.Divisions = append(e.Divisions, departmentMember(dv))
		}
		out = append(out, e)
	}
	return out, ofDivision, nil
}

// maxSafeCents is JavaScript's Number.MAX_SAFE_INTEGER, the widest cell
// site/department.js's Number.isSafeInteger admits.
const maxSafeCents = 1<<53 - 1

// spendingKindOf is the kind a spending link must declare, by its object
// end's role. site/department.js labels a spending row by its links' kind, so
// a Transfers Out row declared external would be drawn as an expenditure.
var spendingKindOf = map[string]project.LinkKind{
	project.RoleObjectCategory: project.KindExternal,
	project.RoleTransferOut:    project.KindInternalTransfer,
}

// vetDepartmentColumn holds one year's two documents to the membership: every
// department funding pays is stated, every fund paying one hangs from a fund
// group the funding document draws, every division spending draws is filed
// under a stated department, every stated department is paid, every stated
// division draws, every label is the one the membership states, every
// funding cell is a positive integer of cents no wider than a JavaScript safe
// integer, and every spending link declares the kind its object end's role
// names.
//
// A fund's group is the one membership site/department.js splits a
// department's bar by; a fund with none would be drawn in no segment.
// site/department.js refuses a funding cell that is not a positive safe
// integer, so one would pass here and the reader be refused the page.
func vetDepartmentColumn(entries []departmentEntry, ofDivision map[string]string,
	fundingStem string, funding departmentDocument, spendingStem string, spending departmentDocument,
) error {
	labels := func(d departmentDocument) map[string]string {
		m := map[string]string{}
		for _, n := range d.Nodes {
			m[n.ID] = n.Label
		}
		return m
	}
	fundingLabels, spendingLabels := labels(funding), labels(spending)
	stated := map[string]bool{}
	for _, e := range entries {
		stated[e.ID] = true
	}

	fundingParent, fundingRole := map[string]string{}, map[string]string{}
	for _, n := range funding.Nodes {
		fundingParent[n.ID], fundingRole[n.ID] = n.Parent, n.Role
	}
	paid := map[string]bool{}
	for _, l := range funding.Links {
		if !stated[l.Target] {
			return fmt.Errorf("%s pays %s, which the department page states no membership for; "+
				"its rows would be offered under no department", fundingStem, l.Target)
		}
		if g := fundingParent[l.Source]; fundingRole[g] != project.RoleFundGroup {
			return fmt.Errorf("%s draws %s paying %s, parented to %q, which is not a fund group the "+
				"document draws; the department's bar would split it into no fund group",
				fundingStem, l.Source, l.Target, g)
		}
		if l.ValueCents <= 0 || l.ValueCents > maxSafeCents {
			return fmt.Errorf("%s draws %s paying %s as %d cents, which is not a positive safe integer; "+
				"the department's bar would be refused in the browser", fundingStem, l.Source, l.Target, l.ValueCents)
		}
		paid[l.Target] = true
	}
	roleOf := map[string]string{}
	for _, n := range spending.Nodes {
		roleOf[n.ID] = n.Role
	}
	spent := map[string]bool{}
	for _, l := range spending.Links {
		if want, ok := spendingKindOf[roleOf[l.Source]]; !ok || l.Kind != want {
			return fmt.Errorf("%s draws %s -> %s as %q, and its object end's role %q declares %q; "+
				"the page would say whether the row is an expenditure from a kind that is not its own",
				spendingStem, l.Source, l.Target, l.Kind, roleOf[l.Source], want)
		}
		if _, ok := ofDivision[l.Target]; !ok {
			return fmt.Errorf("%s draws %s, which no department's membership files; its rows "+
				"would be left out of every department's cost", spendingStem, l.Target)
		}
		spent[l.Target] = true
	}
	for _, e := range entries {
		if !paid[e.ID] {
			return fmt.Errorf("%s draws no fund paying %s; the page would offer a department "+
				"with nothing on one side", fundingStem, e.ID)
		}
		if got := fundingLabels[e.ID]; got != e.Label {
			return fmt.Errorf("%s labels %s %q and the membership %q", fundingStem, e.ID, got, e.Label)
		}
		for _, dv := range e.Divisions {
			if !spent[dv.ID] {
				return fmt.Errorf("%s draws nothing for %s, a division of %s; the department's "+
					"spending would be summed over some of its divisions and not all",
					spendingStem, dv.ID, e.ID)
			}
			if got := spendingLabels[dv.ID]; got != dv.Label {
				return fmt.Errorf("%s labels %s %q and the membership %q", spendingStem, dv.ID, got, dv.Label)
			}
		}
	}
	return nil
}

// encodeDepartmentConfig renders the config and refuses bytes that do not
// match schema/department.schema.json.
func encodeDepartmentConfig(cfg departmentConfig) ([]byte, error) {
	blob, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode the department config: %w", err)
	}
	var v any
	if err := json.Unmarshal(blob, &v); err != nil {
		return nil, fmt.Errorf("re-read the department config: %w", err)
	}
	if err := schema.Validate(schema.Department, v); err != nil {
		return nil, fmt.Errorf("the department config does not match %s: %w", schema.Department, err)
	}
	return blob, nil
}
