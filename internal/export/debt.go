package export

import (
	"cmp"
	"fmt"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// debtPageData is the debt template's input, rendered server-side; a fund
// type's total is the document's, not computed here.
type debtPageData struct {
	chrome
	Columns []string
	// HeadSpan is every column an activity heading spans: the issue and a
	// principal and interest per budget column.
	HeadSpan   int
	Activities []debtActivity
	FundTypes  []string
	Periods    []debtPeriodRow
	Totals     []debtPairRef
	Facts      int
}

// debtActivity is one printed block of the by-issue table.
type debtActivity struct {
	Heading string
	Rows    []debtIssueRow
}

// debtIssueRow is one issue, one pair of cells per budget column.
type debtIssueRow struct {
	Label, Payer, RowLabel string
	Cells                  []debtPairRef
}

// debtPeriodRow is one printed row of the requirements: a year or a band.
type debtPeriodRow struct {
	Label string
	Band  bool
	Years int
	Cells []debtPairRef
}

// debtPairRef is a principal and an interest cell as the page shows them.
type debtPairRef struct {
	Principal, Interest debtFigRef
}

// debtFigRef is one cell. Missing is a cell the page prints nothing in, which
// is absent, not zero; Dash is a cell the page prints "-" in, which is a
// figure of zero and links to its page like any other.
type debtFigRef struct {
	Value   string
	Href    string
	Page    int
	Missing bool
	Dash    bool
}

// debtDoc is as much of the debt-service document as the page reads, field
// for field with schema/debt-service.schema.json.
type debtDoc struct {
	Metadata struct {
		GeneratedBy string       `json:"generated_by"`
		Sources     []sourceMeta `json:"sources"`
		Counts      struct {
			Facts   int `json:"facts"`
			Figures int `json:"figures"`
		} `json:"counts"`
	} `json:"metadata"`
	IssueColumns []debtColumn `json:"issue_columns"`
	Issues       []struct {
		Label    string     `json:"label"`
		RowLabel string     `json:"row_label"`
		Activity string     `json:"activity"`
		Payer    string     `json:"payer"`
		Cells    []debtPair `json:"cells"`
	} `json:"issues"`
	Requirements struct {
		FundTypes []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		} `json:"fund_types"`
		Periods []struct {
			FiscalYearThrough *int       `json:"fiscal_year_through"`
			Label             string     `json:"label"`
			Years             int        `json:"years"`
			Cells             []debtPair `json:"cells"`
		} `json:"periods"`
		Totals []struct {
			FundType       string `json:"fund_type"`
			PrincipalCents int64  `json:"principal_cents"`
			InterestCents  int64  `json:"interest_cents"`
		} `json:"totals"`
	} `json:"requirements"`
}

type debtColumn struct {
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
}

type debtPair struct {
	Principal *debtFigure `json:"principal"`
	Interest  *debtFigure `json:"interest"`
}

type debtFigure struct {
	AmountCents int64  `json:"amount_cents"`
	DocID       string `json:"doc_id"`
	Page        int    `json:"page"`
	Token       string `json:"token"`
}

// buildDebtPage renders the debt view, refusing figures that do not reconcile
// with the document's count and a row not its table's width.
func buildDebtPage(p pageCtx) (debtPageData, error) {
	o, v, ix := p.o, p.v, p.ix
	d, err := decodeWhole[debtDoc](v.Projection, o.Projections[v.Projection])
	if err != nil {
		return debtPageData{}, err
	}

	shown := 0
	cite := newCellCites(p.byID, p.pageTextBase)
	var citeErr error
	fig := func(f *debtFigure) debtFigRef {
		if f == nil {
			return debtFigRef{Missing: true}
		}
		shown++
		cited, pageErr := cite.page(f.DocID, f.Page)
		if pageErr != nil {
			citeErr = cmp.Or(citeErr, pageErr)
			return debtFigRef{}
		}
		ref := debtFigRef{Value: amount.Cents(f.AmountCents).Dollars(), Page: f.Page, Href: cited.TextURL}
		if f.Token == "-" {
			ref.Value, ref.Dash = "–", true
		}
		return ref
	}
	pair := func(p debtPair) debtPairRef {
		return debtPairRef{Principal: fig(p.Principal), Interest: fig(p.Interest)}
	}

	columns := make([]string, len(d.IssueColumns))
	for i, c := range d.IssueColumns {
		columns[i] = c.FiscalYearLabel + " " + c.Basis
	}
	var activities []debtActivity
	for _, is := range d.Issues {
		if len(is.Cells) != len(columns) {
			return debtPageData{}, fmt.Errorf("%s issue %q carries %d cells for %d columns",
				v.Projection, is.RowLabel, len(is.Cells), len(columns))
		}
		if len(activities) == 0 || activities[len(activities)-1].Heading != is.Activity {
			activities = append(activities, debtActivity{Heading: is.Activity})
		}
		row := debtIssueRow{Label: is.Label, Payer: is.Payer, RowLabel: is.RowLabel}
		for _, c := range is.Cells {
			row.Cells = append(row.Cells, pair(c))
		}
		a := &activities[len(activities)-1]
		a.Rows = append(a.Rows, row)
	}

	types := make([]string, len(d.Requirements.FundTypes))
	at := map[string]int{}
	for i, t := range d.Requirements.FundTypes {
		types[i] = t.Label
		at[t.Key] = i
	}
	var periods []debtPeriodRow
	for _, p := range d.Requirements.Periods {
		if len(p.Cells) != len(types) {
			return debtPageData{}, fmt.Errorf("%s period %q carries %d cells for %d fund types",
				v.Projection, p.Label, len(p.Cells), len(types))
		}
		row := debtPeriodRow{Label: p.Label, Band: p.FiscalYearThrough != nil, Years: p.Years}
		for _, c := range p.Cells {
			row.Cells = append(row.Cells, pair(c))
		}
		periods = append(periods, row)
	}
	if citeErr != nil {
		return debtPageData{}, fmt.Errorf("%s: %w", v.Projection, citeErr)
	}
	if shown != d.Metadata.Counts.Figures {
		return debtPageData{}, fmt.Errorf("%s publishes %d figures and the page shows %d",
			v.Projection, d.Metadata.Counts.Figures, shown)
	}

	totals := make([]debtPairRef, len(types))
	for _, t := range d.Requirements.Totals {
		i, ok := at[t.FundType]
		if !ok {
			return debtPageData{}, fmt.Errorf("%s totals fund type %q, which it does not head",
				v.Projection, t.FundType)
		}
		totals[i] = debtPairRef{
			Principal: debtFigRef{Value: amount.Cents(t.PrincipalCents).Dollars()},
			Interest:  debtFigRef{Value: amount.Cents(t.InterestCents).Dollars()},
		}
	}

	ch, _, err := p.chrome(chrome{
		Title:    cmp.Or(v.Title, "What Livermore owes"),
		DataPath: ix.PublishedPath(v.Projection),
	}, d.Metadata.Sources, cite)
	if err != nil {
		return debtPageData{}, err
	}
	return debtPageData{
		chrome:     ch,
		Columns:    columns,
		HeadSpan:   1 + 2*len(columns),
		Activities: activities,
		FundTypes:  types,
		Periods:    periods,
		Totals:     totals,
		Facts:      d.Metadata.Counts.Facts,
	}, nil
}
