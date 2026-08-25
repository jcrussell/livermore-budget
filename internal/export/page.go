package export

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/pkg/cmdutil"
)

// templateName is the page template inside the site asset tree.
const templateName = "index.html.tmpl"

// SchemaVersion is the projection schema this packager understands.
//
// It is deliberately a second copy of project.SchemaVersion and not an import
// of it. This package consumes projections as filename stem -> JSON bytes and
// does not import internal/project (see the package doc); reaching for the
// producer's constant here to save a line would put back the seam the package
// exists to hold open.
//
// What makes the duplication safe is TestSchemaVersionIsPinnedToTheProducer,
// which asserts the two are equal. The test is load-bearing, not decorative:
// without it the constants drift the first time the producer's version moves,
// and this gate then waves through exactly the document it exists to refuse.
// The same test pins the client's copy in site/app.js, which nothing compiles
// against at all.
const SchemaVersion = 1

// ErrSchemaVersion reports a projection whose schema this packager does not
// understand. It is always wrapped with the versions involved, so callers
// match it with errors.Is rather than by string.
var ErrSchemaVersion = errors.New("unsupported schema_version")

// checkSchemaVersion refuses a projection this packager cannot read.
//
// A version this binary does not know is not a malformed document — nothing is
// wrong with the file — so the two directions get the hint that says which way
// the mismatch runs, as registry.schemaVersionErr and mapping.File.validate
// already do for their own files. Rendering it anyway is the failure worth
// preventing: a schema bump changes what the graph MEANS, so an old packager
// handed a new projection would publish a page that is wrong rather than one
// that fails.
func checkSchemaVersion(stem string, got int) error {
	if got == SchemaVersion {
		return nil
	}
	// The refusal names the document it read, not the primary one. It used to
	// say PrimaryProjection unconditionally, which was harmless while there was
	// one document and is a wrong signpost the moment there are two: an
	// operator sent to sankey.json to fix sankey-2027.json finds nothing wrong
	// with it.
	err := fmt.Errorf("%s projection: %w: got %d, want %d",
		stem, ErrSchemaVersion, got, SchemaVersion)
	switch {
	case got > SchemaVersion:
		return cmdutil.WithHint(err,
			"this projection was written by a newer fisc; upgrade the binary")
	case got == 0:
		return cmdutil.WithHint(err,
			"schema_version is absent or zero; this may not be a fisc projection")
	default:
		return err
	}
}

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
// The JSON tags are load-bearing, not decoration: a figure is rendered by the
// template AND shipped in window.FISC_CONFIG for the year toggle to swap in, and
// without them it serialises as Label/Value/Note/Kind while the client reads
// label/value/note/kind. That is a page of empty tiles, and it is silent — the
// template keeps working, because the template never sees the JSON.
type figure struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
	// Kind selects the tile's treatment: "hero" for the headline, "error" for
	// the figure the page exists to argue against, "" for the rest. It is a
	// class name, not a colour: the stylesheet owns the palette.
	Kind string `json:"kind"`
}

// citation is one (document, page) pair the page cites. It is what Write needs
// out of buildPage besides the page itself: the projection's metadata says
// which pages are cited, and re-decoding it to find out would be the second
// implementation of the contract this package refuses to become.
type citation struct {
	DocID string
	Page  int
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

// yearView is one published fiscal year's worth of everything the page states
// in words rather than draws.
//
// IT IS BUILT IN GO FOR EVERY YEAR, not just the one the page opens on, and the
// client swaps between them. The alternative was for app.js to rebuild the
// tiles itself on a year switch, which would put the prose — "The wrong answer:
// summing the expenditure column counts transfers between funds twice" — in two
// languages and let them drift. Here there is one implementation and the client
// only chooses.
//
// It also keeps the no-JavaScript headline: the template renders the opening
// year's tiles into the HTML exactly as before, and these are what the toggle
// reaches for afterwards.
type yearView struct {
	Year    int       `json:"year"`
	Label   string    `json:"label"`
	Stem    string    `json:"stem"`
	Path    string    `json:"path"`
	Basis   string    `json:"basis"`
	Hero    figure    `json:"hero"`
	Figures []figure  `json:"figures"`
	Caveats []string  `json:"caveats"`
	Counts  countsRef `json:"counts"`
}

// countsRef is the "N flows between M nodes, from K facts" line, per year.
type countsRef struct {
	Facts int `json:"facts"`
	Nodes int `json:"nodes"`
	Links int `json:"links"`
}

// pageData is the template's input.
type pageData struct {
	Title           string
	FiscalYearLabel string
	Basis           string
	Scope           string
	Hero            figure
	Figures         []figure
	// Years is every published year, opening year first. The template renders
	// Years[0]'s tiles and caveats into the HTML and lists the rest as a
	// selector; app.js swaps between them without refetching the page.
	Years        []yearView
	Caveats      []string
	Sources      []sourceRef
	Facts        int
	Nodes        int
	Links        int
	ProjectionBy string
	ExportedBy   string
	Projections  []projectionRef
	PrimaryPath  string
	// ConfigJSON is window.FISC_CONFIG. json.Marshal escapes <, > and & to
	// their \u form, so the blob cannot close the script element it sits in.
	ConfigJSON template.JS
}

// clientDoc is a source document as the client sees it.
type clientDoc struct {
	Title     string `json:"title"`
	Publisher string `json:"publisher"`
	PDFURL    string `json:"pdf_url"`
	// PageTextBase is the directory holding the committed page text; the
	// client appends pNNNN.txt. Splitting it this way keeps the zero-padding
	// rule (corpus.PagePath) in one place per side rather than in a template
	// string the client has to parse.
	//
	// It is a relative path into this site — see export.PageTextDir — whenever
	// the export shipped the text, and an absolute URL only when it was told
	// to cite a remote instead. The client appends the same filename either
	// way and must not assume a scheme.
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
	Metadata json.RawMessage `json:"metadata"`
	// Years is every published year with the words that belong to it, built by
	// the packager so the client never composes a figure or a caveat itself.
	Years []yearView           `json:"years"`
	Docs  map[string]clientDoc `json:"docs"`
}

// buildPage decodes the primary projection and assembles everything the
// template and the client need, plus the citations it composed.
//
// pageTextBase resolves a doc id to the directory the page text is cited from,
// with its trailing slash. It is a function and not a URL because the caller,
// not this file, decides between a remote browse view and the copy the site
// ships (see Write).
// tilesFor renders one year's headline figures.
//
// The prose lives here and nowhere else. Each note is a claim about what the
// number means, and a second implementation of it — in the client, for a year
// switch — is the way two figures on one page come to disagree about the same
// schedule.
func tilesFor(meta projectionMetadata) (figure, []figure) {
	h := meta.Headline
	hero := figure{
		Label: "What the city actually spends",
		Value: dollars(h.AllFundsGrossExpenditureCents),
		Note:  "All funds, gross, " + meta.FiscalYearLabel + " " + meta.Basis + " budget",
		Kind:  "hero",
	}
	return hero, []figure{{
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
	}}
}

// decodeProjection reads one projection document and refuses it before anything
// is read out of it, because every field named below belongs to a contract a
// version this packager does not know may have renamed or redefined.
func decodeProjection(stem string, raw []byte) (projectionDoc, projectionMetadata, error) {
	var doc projectionDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, projectionMetadata{}, fmt.Errorf("decode %s projection: %w", stem, err)
	}
	if err := checkSchemaVersion(stem, doc.SchemaVersion); err != nil {
		return doc, projectionMetadata{}, err
	}
	if len(doc.Metadata) == 0 {
		return doc, projectionMetadata{}, fmt.Errorf("%s projection has no metadata block", stem)
	}
	var meta projectionMetadata
	if err := json.Unmarshal(doc.Metadata, &meta); err != nil {
		return doc, projectionMetadata{}, fmt.Errorf("decode %s metadata: %w", stem, err)
	}
	if meta.FiscalYearLabel == "" {
		return doc, projectionMetadata{}, fmt.Errorf("%s metadata has no fiscal_year_label", stem)
	}
	if meta.Headline.AllFundsGrossExpenditureCents == 0 {
		return doc, projectionMetadata{}, fmt.Errorf("%s metadata has no headline expenditure", stem)
	}
	return doc, meta, nil
}

// orderedYears is the year documents to render, opening year first.
//
// It takes the caller's list rather than inferring one from the stems. Which
// documents are fiscal years of the same projection is a statement the
// composition root makes; guessing it from a name prefix would be this package
// deciding what "sankey-2027" means, and would be wrong for the first document
// named after the primary that is not a year of it.
//
// An empty list means one year, which is a page with no year control and not a
// defect.
func orderedYears(stems []string) []string {
	if len(stems) == 0 {
		return []string{PrimaryProjection}
	}
	return stems
}

func buildPage(projections map[string][]byte, yearStems []string, docs []Doc, pageTextBase func(docID string) string, exportedBy string) (pageData, []citation, error) {
	doc, meta, err := decodeProjection(PrimaryProjection, projections[PrimaryProjection])
	if err != nil {
		return pageData{}, nil, err
	}

	// Every published year, in the order the packager was handed them, opening
	// year first. A year's document is decoded and refused on its own terms:
	// one bad document is named, rather than the page silently opening on
	// whichever year happened to parse.
	//
	// The metadata each year contributes is also where its CITATIONS come from,
	// which is why the source loop below reads yearMetas rather than the
	// primary's alone. Both years cite pp.66-67 today so the union is the same
	// set, but a page that shipped page text for only the year it opened on
	// would render dead citation links on the other -- see fisc-fjy, which is
	// the general case of this and is not closed by the loop here.
	stems := orderedYears(yearStems)
	yearMetas := make([]projectionMetadata, 0, len(stems))
	years := make([]yearView, 0, len(stems))
	for _, stem := range stems {
		m := meta
		if stem != PrimaryProjection {
			if _, m, err = decodeProjection(stem, projections[stem]); err != nil {
				return pageData{}, nil, err
			}
		}
		yearMetas = append(yearMetas, m)
		hero, figures := tilesFor(m)
		years = append(years, yearView{
			Year:    m.FiscalYear,
			Label:   m.FiscalYearLabel,
			Stem:    stem,
			Path:    path.Join(DataDir, stem+".json"),
			Basis:   m.Basis,
			Hero:    hero,
			Figures: figures,
			Caveats: m.Caveats,
			Counts: countsRef{
				Facts: m.Counts.Facts, Nodes: m.Counts.Nodes, Links: m.Counts.Links,
			},
		})
	}
	hero, figures := tilesFor(meta)

	byID := make(map[string]Doc, len(docs))
	for _, d := range docs {
		byID[d.ID] = d
	}
	sources := make([]sourceRef, 0, len(meta.Sources))
	clientDocs := make(map[string]clientDoc, len(meta.Sources))
	var cited []citation
	citedSeen := make(map[citation]bool)
	// The FOOTER lists the opening year's sources, because a page claiming
	// provenance for figures it is not showing is its own defect. The SHIPPED
	// page text is the union across every year, because a citation link that
	// resolves for one year and 404s for another is worse than either.
	for _, m := range yearMetas[1:] {
		for _, src := range m.Sources {
			for _, pg := range src.Pages {
				if key := (citation{DocID: src.DocID, Page: pg}); !citedSeen[key] {
					citedSeen[key] = true
					cited = append(cited, key)
				}
			}
		}
	}
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
		base := pageTextBase(s.DocID)
		for _, p := range s.Pages {
			ref.Pages = append(ref.Pages, pageRef{
				Number:  p,
				PDFURL:  pdfPageURL(d.PDFURL, p),
				TextURL: base + pageTextFile(p),
			})
			// Deduplicated: a document that cites a page twice is one file to
			// ship, and shipping it twice is a write collision.
			if key := (citation{DocID: s.DocID, Page: p}); !citedSeen[key] {
				citedSeen[key] = true
				cited = append(cited, key)
			}
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
		Years:         years,
		Docs:          clientDocs,
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return pageData{}, nil, fmt.Errorf("encode page config: %w", err)
	}

	page := pageData{
		Title:           "City of Livermore budget flows — " + meta.FiscalYearLabel,
		FiscalYearLabel: meta.FiscalYearLabel,
		Basis:           meta.Basis,
		Scope:           meta.Scope,
		Hero:            hero,
		Figures:         figures,
		Years:           years,
		Caveats:         meta.Caveats,
		Sources:         sources,
		Facts:           meta.Counts.Facts,
		Nodes:           meta.Counts.Nodes,
		Links:           meta.Counts.Links,
		ProjectionBy:    meta.GeneratedBy,
		ExportedBy:      exportedBy,
		Projections:     refs,
		PrimaryPath:     files[PrimaryProjection],
		// #nosec G203 -- blob is encoding/json's output, which escapes <, >
		// and & to their \u form, so it cannot terminate the script element
		// or inject markup. The alternative, letting html/template escape a
		// string, would corrupt the JSON.
		ConfigJSON: template.JS(blob),
	}
	return page, cited, nil
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

// remotePageTextBase is the directory URL of a document's committed page text
// in a browsable copy of the repository, so the path after the base is the
// repository's own layout rather than the exported site's.
func remotePageTextBase(browseURL, docID string) string {
	return strings.TrimSuffix(browseURL, "/") + "/data/extracted/" + docID + "/pages/"
}

// pageTextFile mirrors corpus.PagePath's zero padding. It is spelled out
// rather than imported because it is a URL here, not a filesystem path, and
// the two only look alike.
func pageTextFile(page int) string { return fmt.Sprintf("p%04d.txt", page) }

// dollars renders integer cents the way the schedule prints them: whole
// dollars with thousands separators, keeping the cents only when a figure
// actually has some.
func dollars(cents int64) string {
	s := amount.Cents(cents).String()
	return strings.TrimSuffix(s, ".00")
}
