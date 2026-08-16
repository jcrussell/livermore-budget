package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
)

// templateName is the page template inside the site asset tree.
const templateName = "index.html.tmpl"

// projectionDoc is as much of a projection document as the packager reads.
// Everything else — nodes, links — is bulk the browser fetches, and decoding
// it here would be a second parser for a contract that already has one.
//
// Metadata is kept as raw JSON as well as decoded, so window.FISC_CONFIG can
// carry the projection's own bytes rather than this package's re-rendering of
// them. A field this struct does not know about still reaches the client.
type projectionDoc struct {
	SchemaVersion int             `json:"schema_version"`
	Projection    string          `json:"projection"`
	Metadata      json.RawMessage `json:"metadata"`
}

// projectionMetadata is the decoded metadata block. The page renders these
// figures server-side so the headline survives without JavaScript.
type projectionMetadata struct {
	GeneratedBy     string `json:"generated_by"`
	FiscalYear      int    `json:"fiscal_year"`
	FiscalYearLabel string `json:"fiscal_year_label"`
	Basis           string `json:"basis"`
	Scope           string `json:"scope"`
	Currency        string `json:"currency"`
	Units           string `json:"units"`
	Sources         []struct {
		DocID string `json:"doc_id"`
		Pages []int  `json:"pages"`
	} `json:"sources"`
	Headline headline `json:"headline"`
	Counts   struct {
		Facts int `json:"facts"`
		Nodes int `json:"nodes"`
		Links int `json:"links"`
	} `json:"counts"`
	Caveats []string `json:"caveats"`
}

// headline is the projection's published totals, in cents.
type headline struct {
	AllFundsGrossRevenueCents     int64 `json:"all_funds_gross_revenue_cents"`
	AllFundsGrossExpenditureCents int64 `json:"all_funds_gross_expenditure_cents"`
	ExternalRevenueCents          int64 `json:"external_revenue_cents"`
	ExternalExpenditureCents      int64 `json:"external_expenditure_cents"`
	InternalTransferInCents       int64 `json:"internal_transfer_in_cents"`
	InternalTransferOutCents      int64 `json:"internal_transfer_out_cents"`
	NaiveExpenditureCents         int64 `json:"naive_expenditure_cents"`
	TransferResidualCents         int64 `json:"transfer_residual_cents"`
}

// figure is one stat tile.
type figure struct {
	Label string
	Value string
	Note  string
	// Kind selects the tile's treatment: "hero" for the headline, "error" for
	// the figure the page exists to argue against, "" for the rest. It is a
	// class name, not a colour: the stylesheet owns the palette.
	Kind string
}

// pageRef is one cited page of one source document.
type pageRef struct {
	Number  int
	PDFURL  string
	TextURL string
}

// sourceRef is one cited source document.
type sourceRef struct {
	DocID     string
	Title     string
	Publisher string
	PDFURL    string
	Pages     []pageRef
}

// projectionRef names a projection file the page ships.
type projectionRef struct {
	Name string
	Path string
}

// pageData is the template's input.
type pageData struct {
	Title           string
	FiscalYearLabel string
	Basis           string
	Scope           string
	Hero            figure
	Figures         []figure
	Caveats         []string
	Sources         []sourceRef
	Facts           int
	Nodes           int
	Links           int
	ProjectionBy    string
	ExportedBy      string
	Projections     []projectionRef
	PrimaryPath     string
	// ConfigJSON is window.FISC_CONFIG. json.Marshal escapes <, > and & to
	// their \u form, so the blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// clientDoc is a source document as the client sees it.
type clientDoc struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	PDFURL    string `json:"pdf_url"`
	// PageTextBase is the directory URL holding the committed page text; the
	// client appends pNNNN.md. Splitting it this way keeps the zero-padding
	// rule (corpus.PagePath) in one place per side rather than in a template
	// string the client has to parse.
	PageTextBase string `json:"page_text_base"`
}

// clientConfig is window.FISC_CONFIG: the metadata the page needs before it
// has fetched anything, plus where to fetch the bulk from.
type clientConfig struct {
	SchemaVersion int               `json:"schema_version"`
	ExportedBy    string            `json:"exported_by"`
	Primary       string            `json:"primary"`
	Projections   map[string]string `json:"projections"`
	// Metadata is the primary projection's metadata block, verbatim.
	Metadata json.RawMessage      `json:"metadata"`
	Docs     map[string]clientDoc `json:"docs"`
}

// buildPage decodes the primary projection and assembles everything the
// template and the client need.
func buildPage(projections map[string][]byte, docs []Doc, browseURL, exportedBy string) (pageData, error) {
	var doc projectionDoc
	if err := json.Unmarshal(projections[PrimaryProjection], &doc); err != nil {
		return pageData{}, fmt.Errorf("decode %s projection: %w", PrimaryProjection, err)
	}
	if len(doc.Metadata) == 0 {
		return pageData{}, fmt.Errorf("%s projection has no metadata block", PrimaryProjection)
	}
	var meta projectionMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return pageData{}, fmt.Errorf("decode %s metadata: %w", PrimaryProjection, err)
	}
	if meta.FiscalYearLabel == "" {
		return pageData{}, fmt.Errorf("%s metadata has no fiscal_year_label", PrimaryProjection)
	}
	if meta.Headline.AllFundsGrossExpenditureCents == 0 {
		return pageData{}, fmt.Errorf("%s metadata has no headline expenditure", PrimaryProjection)
	}

	byID := make(map[string]Doc, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	sources := make([]sourceRef, 0, len(meta.Sources))
	clientDocs := make(map[string]clientDoc, len(meta.Sources))
	for _, s := range meta.Sources {
		d := byID[s.DocID]
		ref := sourceRef{
			DocID:     s.DocID,
			Title:     d.Title,
			Publisher: d.Publisher,
			PDFURL:    d.PDFURL,
		}
		if ref.Title == "" {
			// A document the registry does not describe still gets cited, by
			// id. Dropping the citation because a title is missing would hide
			// the provenance the page exists to show.
			ref.Title = s.DocID
		}
		base := pageTextBase(browseURL, s.DocID)
		for _, p := range s.Pages {
			ref.Pages = append(ref.Pages, pageRef{
				Number:  p,
				PDFURL:  pdfPageURL(d.PDFURL, p),
				TextURL: base + pageTextFile(p),
			})
		}
		sources = append(sources, ref)
		clientDocs[s.DocID] = clientDoc{
			Title:        ref.Title,
			Publisher:    ref.Publisher,
			PDFURL:       d.PDFURL,
			PageTextBase: base,
		}
	}

	files := make(map[string]string, len(projections))
	refs := make([]projectionRef, 0, len(projections))
	for _, name := range sortedKeys(projections) {
		p := path.Join(DataDir, name+".json")
		files[name] = p
		refs = append(refs, projectionRef{Name: name, Path: p})
	}

	cfg := clientConfig{
		SchemaVersion: doc.SchemaVersion,
		ExportedBy:    exportedBy,
		Primary:       PrimaryProjection,
		Projections:   files,
		Metadata:      doc.Metadata,
		Docs:          clientDocs,
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return pageData{}, fmt.Errorf("encode page config: %w", err)
	}

	h := meta.Headline
	return pageData{
		Title:           "City of Livermore budget flows — " + meta.FiscalYearLabel,
		FiscalYearLabel: meta.FiscalYearLabel,
		Basis:           meta.Basis,
		Scope:           meta.Scope,
		Hero: figure{
			Label: "What the city actually spends",
			Value: dollars(h.AllFundsGrossExpenditureCents),
			Note:  "All funds, gross, " + meta.FiscalYearLabel + " " + meta.Basis + " budget",
			Kind:  "hero",
		},
		Figures: []figure{{
			Label: "Naive column total",
			Value: dollars(h.NaiveExpenditureCents),
			Note: "The wrong answer: summing the expenditure column counts transfers between funds twice, inflating the total by " +
				dollars(h.NaiveExpenditureCents-h.AllFundsGrossExpenditureCents) + ".",
			Kind: "error",
		}, {
			Label: "All-funds gross revenue",
			Value: dollars(h.AllFundsGrossRevenueCents),
			Note:  "Ties to the printed schedule; includes internal service charges.",
		}, {
			Label: "External revenue",
			Value: dollars(h.ExternalRevenueCents),
			Note:  "Net of internal service charges billed between city departments.",
		}, {
			Label: "External spending",
			Value: dollars(h.ExternalExpenditureCents),
			Note:  "Net of internal service charges.",
		}, {
			Label: "Transfers in / out",
			Value: dollars(h.InternalTransferInCents) + " / " + dollars(h.InternalTransferOutCents),
			Note:  "Money moving between the city's own funds.",
		}, {
			Label: "Unmatched transfers",
			Value: dollars(h.TransferResidualCents),
			Note:  "Transfers out minus transfers in. The schedule that would pair them is not mapped yet, so no link carries a transfer id.",
		}},
		Caveats:      meta.Caveats,
		Sources:      sources,
		Facts:        meta.Counts.Facts,
		Nodes:        meta.Counts.Nodes,
		Links:        meta.Counts.Links,
		ProjectionBy: meta.GeneratedBy,
		ExportedBy:   exportedBy,
		Projections:  refs,
		PrimaryPath:  files[PrimaryProjection],
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}, nil
}

// renderPage executes the page template against the assembled data.
func renderPage(assets fs.FS, data pageData) ([]byte, error) {
	tmpl, err := template.New(templateName).ParseFS(assets, templateName)
	if err != nil {
		return nil, fmt.Errorf("parse page template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render page: %w", err)
	}
	return buf.Bytes(), nil
}

// pdfPageURL points at a page of the published PDF. The city's CMS serves the
// document from a numeric id and viewers honour the #page fragment, so the
// citation lands on the page rather than the cover.
func pdfPageURL(pdfURL string, page int) string {
	if pdfURL == "" {
		return ""
	}
	return fmt.Sprintf("%s#page=%d", pdfURL, page)
}

// pageTextBase is the directory URL of a document's committed page text.
func pageTextBase(browseURL, docID string) string {
	return strings.TrimSuffix(browseURL, "/") + "/data/extracted/" + docID + "/pages/"
}

// pageTextFile mirrors corpus.PagePath's zero padding. It is spelled out
// rather than imported because it is a URL here, not a filesystem path, and
// the two only look alike.
func pageTextFile(page int) string { return fmt.Sprintf("p%04d.md", page) }

// dollars renders integer cents the way the schedule prints them: whole
// dollars with thousands separators, keeping the cents only when a figure
// actually has some.
func dollars(cents int64) string {
	s := amount.Cents(cents).String()
	return strings.TrimSuffix(s, ".00")
}
