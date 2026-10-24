package export

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/amount"
	"github.com/jcrussell/livermore-budget/internal/corpus"
	"github.com/jcrussell/livermore-budget/internal/project"
)

// sourceMeta is one cited document in any projection's metadata.
type sourceMeta struct {
	DocID string `json:"doc_id"`
	Pages []int  `json:"pages"`
}

// documentSources is as much of ANY document as the citation union needs.
//
// It is deliberately shape-blind: metadata.sources is the one block every
// projection carries whatever its body is.
type documentSources struct {
	Metadata struct {
		Sources []sourceMeta `json:"sources"`
	} `json:"metadata"`
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

// projectionRef names one data file the site publishes.
type projectionRef struct {
	Path string
}

// clientDoc is a source document as the client sees it: its title, and for
// every page the site cites, the three links a citation opens, built here so
// the client composes none.
type clientDoc struct {
	Title     string                `json:"title"`
	Publisher string                `json:"publisher"`
	Pages     map[string]clientPage `json:"pages"`
}

// clientPage is one cited page's links. An empty one is a link this export
// has nothing to point at: no PDF URL for the document, or no records.
type clientPage struct {
	PDF     string `json:"pdf"`
	Text    string `json:"text"`
	Records string `json:"records"`
}

// citationsOf is every (document, page) one projection's metadata cites.
//
// It decodes through documentSources rather than through the projection
// metadata, so a view of a document this packager has never heard of still
// contributes its pages to the set the site ships.
func citationsOf(stem string, raw []byte) ([]Citation, error) {
	if _, err := decodeDocument(stem, raw); err != nil {
		return nil, err
	}
	var src documentSources
	if err := json.Unmarshal(raw, &src); err != nil {
		return nil, fmt.Errorf("decode %s sources: %w", stem, err)
	}
	out := make([]Citation, 0, len(src.Metadata.Sources))
	for _, s := range src.Metadata.Sources {
		for _, p := range s.Pages {
			out = append(out, Citation{DocID: s.DocID, Page: p})
		}
	}
	return out, nil
}

// sourcesFor builds a view's own footer citations, and the client's copy of the
// same documents.
func sourcesFor(srcs []sourceMeta, byID map[string]Doc, pageTextBase func(string) string,
	recordsBase map[string]string,
) ([]sourceRef, map[string]clientDoc) {
	sources := make([]sourceRef, 0, len(srcs))
	clientDocs := make(map[string]clientDoc, len(srcs))
	for _, s := range srcs {
		d := byID[s.DocID]
		ref := sourceRef{DocID: s.DocID, Title: d.Title, Publisher: d.Publisher, PDFURL: d.PDFURL}
		if ref.Title == "" {
			// A document the registry does not describe still gets cited, by
			// id. Dropping the citation because a title is missing would hide
			// the provenance the page exists to show.
			ref.Title = s.DocID
		}
		base := pageTextBase(s.DocID)
		pages := make(map[string]clientPage, len(s.Pages))
		for _, p := range s.Pages {
			ref.Pages = append(ref.Pages, pageRef{
				Number:  p,
				PDFURL:  pdfPageURL(d.PDFURL, p),
				TextURL: base + pageTextFile(p),
			})
			// An empty records base is a document published with no records:
			// no link, not one pointing nowhere.
			records := ""
			if rb := recordsBase[s.DocID]; rb != "" {
				records = rb + RecordsFile(p)
			}
			pages[strconv.Itoa(p)] = clientPage{PDF: pdfPageURL(d.PDFURL, p), Text: base + pageTextFile(p), Records: records}
		}
		sources = append(sources, ref)
		clientDocs[s.DocID] = clientDoc{Title: ref.Title, Publisher: ref.Publisher, Pages: pages}
	}
	return sources, clientDocs
}

// citedSources is what a view's footer and client take of the pages it cites:
// the footer's list and the client's documents, and the sources page the
// footer links instead of a list when the site has one. cells is every page
// this export composed a figure's own citation for, and nil for a view whose
// figures the client cites from their links' locators -- the sankey,
// department and sides views -- whose pages are held here only as far as
// cited names them (fisc-51fr.40).
//
// WITH A SOURCES PAGE THE LIST IS NIL, and only because listedBy has held
// every cited page -- its metadata's and each figure's own -- to that page's
// index; a page it does not list refuses the export rather than dropping out
// of every list on the site.
func citedSources(o *Options, view string, cited []sourceMeta, cells *cellCites, byID map[string]Doc,
	pageTextBase func(string) string,
) (sources []sourceRef, sourcesPath string, docs map[string]clientDoc, err error) {
	if cells != nil {
		cited = append(slices.Clone(cited), cells.pages...)
	}
	union := unionSources(cited)
	sourcesPath = templatePathOf(o, ProvenanceTemplate)
	if sourcesPath != "" {
		if err := listedBy(o.PageIndex, union, view, sourcesPath); err != nil {
			return nil, "", nil, err
		}
	}
	sources, docs = sourcesFor(union, byID, pageTextBase, o.RecordsBase)
	if sourcesPath != "" {
		sources = nil
	}
	return sources, sourcesPath, docs, nil
}

// unionSources merges the sources of every year a view publishes into one list,
// deduplicated and ordered.
//
// A union and not the opening year's, because site/app.js's citations() skips
// a fact whose doc_id is not in CONFIG.docs, in silence. A union within a view
// and never across views.
func unionSources(srcs []sourceMeta) []sourceMeta {
	pages := map[string]map[int]bool{}
	for _, s := range srcs {
		if pages[s.DocID] == nil {
			pages[s.DocID] = map[int]bool{}
		}
		for _, p := range s.Pages {
			pages[s.DocID][p] = true
		}
	}
	out := make([]sourceMeta, 0, len(pages))
	for _, id := range slices.Sorted(maps.Keys(pages)) {
		ps := make([]int, 0, len(pages[id]))
		for p := range pages[id] {
			ps = append(ps, p)
		}
		slices.Sort(ps)
		out = append(out, sourceMeta{DocID: id, Pages: ps})
	}
	return out
}

// projectionRefs is every data file the site publishes, which is the whole set
// on every page: they are downloadable provenance, not this view's figures.
// One entry per file and not per document.
func projectionRefs(o *Options, ix ColumnIndex) []projectionRef {
	seen := map[string]bool{}
	var paths []string
	for _, name := range slices.Sorted(maps.Keys(o.Projections)) {
		p := ix.PublishedPath(name)
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	slices.Sort(paths)
	refs := make([]projectionRef, 0, len(paths))
	for _, p := range paths {
		refs = append(refs, projectionRef{Path: p})
	}
	return refs
}

// citedPage is one page's citation, composed by sourcesFor, for a caller
// citing a figure by its own document and page. It refuses a document the
// registry does not describe, which sourcesFor cites by id with no PDF link.
func citedPage(byID map[string]Doc, pageTextBase func(string) string, docID string, page int) (pageRef, error) {
	if _, ok := byID[docID]; !ok {
		return pageRef{}, fmt.Errorf("%s p%d cites a document the registry does not describe", docID, page)
	}
	refs, _ := sourcesFor([]sourceMeta{{DocID: docID, Pages: []int{page}}}, byID, pageTextBase, nil)
	return refs[0].Pages[0], nil
}

// cellCites composes each figure's own page through citedPage and keeps
// every page it composed, for citedSources to hold to the sources page.
type cellCites struct {
	byID         map[string]Doc
	pageTextBase func(string) string
	pages        []sourceMeta
}

func newCellCites(byID map[string]Doc, pageTextBase func(string) string) *cellCites {
	return &cellCites{byID: byID, pageTextBase: pageTextBase}
}

// page is one figure's citation of one page.
func (c *cellCites) page(docID string, page int) (pageRef, error) {
	ref, err := citedPage(c.byID, c.pageTextBase, docID, page)
	if err != nil {
		return pageRef{}, err
	}
	c.pages = append(c.pages, sourceMeta{DocID: docID, Pages: []int{page}})
	return ref, nil
}

// locators is one figure's citation of every page its locators name.
func (c *cellCites) locators(locs []sourceMeta) ([]pageRef, error) {
	var out []pageRef
	for _, s := range unionSources(locs) {
		for _, p := range s.Pages {
			ref, err := c.page(s.DocID, p)
			if err != nil {
				return nil, err
			}
			out = append(out, ref)
		}
	}
	return out, nil
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

// pageTextFile is page n's text file name, corpus.PagePath's last element.
func pageTextFile(page int) string { return path.Base(corpus.PagePath(page)) }

// provenancePageData is the provenance index's own shape.
type provenancePageData struct {
	chrome
	Rows      []provenanceRow
	Downloads []downloadRef
	// DataFiles is every published data file with what it is, so a reader
	// opens one to read about it before downloading it.
	DataFiles []dataFileRef
	Documents int
	Pages     int
	Records   int
	// Inferred is every column's derived nodes and flows, the site's one
	// statement of what is ours rather than the city's.
	Inferred []inferredColumn
	Wording  wording
}

// inferredColumn is one published column's derived nodes, each with its
// rationale, its source note and the derived flows into or out of it, and its
// derived flows between printed nodes.
type inferredColumn struct {
	Label string
	Path  string
	Nodes []inferredNode
	Flows []inferredFlow
}

type inferredNode struct {
	Label, Rationale, SourceNote string
	Flows                        []inferredFlow
}

// inferredFlow is one derived flow with the cents its projection link
// carries, the facts it states, and the pages its locators cite, each
// composed by sourcesFor.
type inferredFlow struct {
	From, To string
	Dollars  string
	Facts    string
	Pages    []pageRef
}

// inferredColumns gathers every derived node and every derived flow out of
// the documents folded into a column, one entry per column in path order.
//
// A flow is the cells it cites: each set of fact ids is listed once however
// many ribbons draw it -- the leaf, a hub leg repeating it, another schedule
// stating it -- with every page any of those ribbons cites, and two ribbons
// stating one set at two values refuse the export. A ribbon is named at the
// ends its [project.Link.Repeats] spells, else at its own, in its own
// schedule's labels. Of the ribbons drawing a set, the listed one is the
// first whose target is derived, else the first whose source is, else the
// first, a ribbon naming its Repeats first among equals; it is listed under
// that derived end, and under the column when none has a derived end.
//
// A ROW IS DROPPED ONLY WHERE A DECLARATION SAYS ANOTHER STATES IT, and the
// one that can is [project.Link.Completes]: a derived ribbon whose own cell
// the column's derived ribbons complete is not listed where each of its facts
// is cited by a listed completing row citing every page it cites. Restates
// says a split's links state leaves again only together, so it drops none.
// Cents summing to a row's are no declaration, and leave it listed.
//
// The derived nodes are the column's across all its schedules, gathered
// before any flow is filed, so the result does not depend on the order the
// stems sort in. A column inferring nothing is listed with nothing. refs is
// the sources page's composed citations, and a flow citing a page it does
// not list is refused.
func inferredColumns(o *Options, ix ColumnIndex, refs map[Citation]pageRef) ([]inferredColumn, error) {
	type ribbon struct {
		stem            string
		source, target  string
		from, to        string
		named           bool
		cell, completes string
		cents           int64
		facts           []string
	}
	type cell struct {
		ribbons []ribbon
		locs    []sourceMeta
	}
	type column struct {
		inferredColumn
		nodeAt map[string]int
		order  []string
		cells  map[string]*cell
	}
	byPath := map[string]*column{}
	for _, stem := range slices.Sorted(maps.Keys(o.Projections)) {
		path, folded := ix.Column(stem)
		if !folded {
			continue
		}
		var d project.Document
		if err := json.Unmarshal(o.Projections[stem], &d); err != nil {
			return nil, fmt.Errorf("decode %s nodes: %w", stem, err)
		}
		col := byPath[path]
		if col == nil {
			col = &column{inferredColumn: inferredColumn{Label: ledeOf(d.Metadata.FiscalYearLabel, d.Metadata.Basis), Path: path},
				nodeAt: map[string]int{}, cells: map[string]*cell{}}
			byPath[path] = col
		}
		labels := make(map[string]string, len(d.Nodes))
		for _, n := range d.Nodes {
			labels[n.ID] = n.Label
			if !n.Derived {
				continue
			}
			if _, ok := col.nodeAt[n.ID]; !ok {
				col.nodeAt[n.ID] = len(col.Nodes)
				col.Nodes = append(col.Nodes, inferredNode{Label: n.Label, Rationale: n.Rationale, SourceNote: n.SourceNote})
			}
		}
		for _, l := range d.Links {
			if !l.Derived {
				continue
			}
			source, target := l.Source, l.Target
			if l.Repeats != "" {
				if source, target = project.LeafEnds(l.Repeats); source == "" || target == "" {
					return nil, fmt.Errorf("%s draws %s -> %s repeating %q, which spells no leaf's ends",
						stem, l.Source, l.Target, l.Repeats)
				}
			}
			from, okFrom := labels[source]
			to, okTo := labels[target]
			if !okFrom || !okTo {
				return nil, fmt.Errorf("%s draws the derived flow %s -> %s at an end it has no node for", stem, source, target)
			}
			ids := slices.Clone(l.FactIDs)
			slices.Sort(ids)
			key := strings.Join(ids, " ")
			if len(ids) == 0 {
				key = "\x00" + source + "\x00" + target
			}
			c := col.cells[key]
			if c == nil {
				c = &cell{}
				col.cells[key] = c
				col.order = append(col.order, key)
			} else if c.ribbons[0].cents != l.ValueCents {
				return nil, fmt.Errorf("%s states the derived flow %s -> %s at %s, and another ribbon of %s citing its cells at %s",
					stem, l.Source, l.Target, amount.Cents(l.ValueCents).Dollars(), path, amount.Cents(c.ribbons[0].cents).Dollars())
			}
			c.ribbons = append(c.ribbons, ribbon{stem: stem, source: source, target: target, from: from, to: to,
				named: l.Repeats != "", cell: project.CellKey(l.Source, l.Target, l.Kind), completes: l.Completes,
				cents: l.ValueCents, facts: ids})
			for _, s := range l.Locators {
				c.locs = append(c.locs, sourceMeta(s))
			}
		}
	}
	out := make([]inferredColumn, 0, len(byPath))
	for _, path := range slices.Sorted(maps.Keys(byPath)) {
		col := byPath[path]
		rank := func(r ribbon) int {
			if _, ok := col.nodeAt[r.target]; ok {
				return 0
			}
			if _, ok := col.nodeAt[r.source]; ok {
				return 1
			}
			return 2
		}
		named := func(r ribbon) int {
			if r.named {
				return 0
			}
			return 1
		}
		type row struct {
			ribbon
			pages map[Citation]bool
		}
		rows := make(map[string]*row, len(col.order))
		completing := map[string][]string{}
		for _, key := range col.order {
			c := col.cells[key]
			r := c.ribbons[0]
			for _, x := range c.ribbons[1:] {
				if cmp.Or(cmp.Compare(rank(x), rank(r)), cmp.Compare(named(x), named(r))) < 0 {
					r = x
				}
			}
			pages := map[Citation]bool{}
			for _, s := range c.locs {
				for _, p := range s.Pages {
					pages[Citation{DocID: s.DocID, Page: p}] = true
				}
			}
			rows[key] = &row{ribbon: r, pages: pages}
			for _, x := range c.ribbons {
				if x.completes != "" && !slices.Contains(completing[x.completes], key) {
					completing[x.completes] = append(completing[x.completes], key)
				}
			}
		}
		// completed is the keys of the rows completing one of key's ribbons'
		// own cells, other than key's own.
		completed := func(key string) []string {
			var by []string
			for _, x := range col.cells[key].ribbons {
				for _, k := range completing[x.cell] {
					if k != key && !slices.Contains(by, k) {
						by = append(by, k)
					}
				}
			}
			return by
		}
		dropped := map[string]bool{}
		for _, key := range col.order {
			by := completed(key)
			if len(by) == 0 {
				continue
			}
			r := rows[key]
			stated := len(r.facts) > 0
			for _, id := range r.facts {
				stated = stated && slices.ContainsFunc(by, func(k string) bool {
					f := rows[k]
					if len(completed(k)) > 0 || !slices.Contains(f.facts, id) {
						return false
					}
					for p := range r.pages {
						if !f.pages[p] {
							return false
						}
					}
					return true
				})
			}
			dropped[key] = stated
		}
		for _, key := range col.order {
			if dropped[key] {
				continue
			}
			r := rows[key]
			cites := slices.SortedFunc(maps.Keys(r.pages), func(a, b Citation) int {
				return cmp.Or(cmp.Compare(a.DocID, b.DocID), cmp.Compare(a.Page, b.Page))
			})
			if len(cites) == 0 {
				return nil, fmt.Errorf("%s's derived flow %s -> %s cites no page", r.stem, r.source, r.target)
			}
			pages := make([]pageRef, 0, len(cites))
			for _, c := range cites {
				ref, ok := refs[c]
				if !ok {
					return nil, fmt.Errorf("%s's derived flow %s -> %s cites %s p%d, which the sources page does not list",
						r.stem, r.source, r.target, c.DocID, c.Page)
				}
				pages = append(pages, ref)
			}
			flow := inferredFlow{From: r.from, To: r.to, Dollars: amount.Cents(r.cents).Dollars(),
				Facts: strings.Join(r.facts, " "), Pages: pages}
			switch rank(r.ribbon) {
			case 0:
				i := col.nodeAt[r.target]
				col.Nodes[i].Flows = append(col.Nodes[i].Flows, flow)
			case 1:
				i := col.nodeAt[r.source]
				col.Nodes[i].Flows = append(col.Nodes[i].Flows, flow)
			default:
				col.Flows = append(col.Flows, flow)
			}
		}
		out = append(out, col.inferredColumn)
	}
	return out, nil
}

// provenanceRow is one published locator, with both halves of its citation
// already composed.
type provenanceRow struct {
	DocID    string
	DocTitle string
	Page     int
	Records  int
	Bytes    int
	Note     string
	DataURL  string
	PDFURL   string
	TextURL  string
}

// dataFileRef is one published data file as the provenance page describes
// it: every document folded into Path, together.
type dataFileRef struct {
	Path string
	// Label is the fiscal year and basis its documents state, "" where they
	// state none.
	Label string
	// Pages are the views that draw it, in nav order.
	Pages []navItem
	// Sources are the city's documents it cites, each with the pages.
	Sources []sourceRef
	// Caveats is how many distinct caveats travel inside it, and
	// CaveatsPath the caveats page, "" when the site has none.
	Caveats     int
	CaveatsPath string
}

// dataFilesOf describes every published data file. A file's documents are
// the stems ix.PublishedPath folds into it; its pages are the views whose
// [View.ReachedStems] reach one of them; its sources go through sourcesFor,
// so no URL is composed here.
func dataFilesOf(o *Options, nav []navItem, byID map[string]Doc, ix ColumnIndex,
	pageTextBase func(string) string) ([]dataFileRef, error) {
	stemsOf := map[string][]string{}
	for _, stem := range slices.Sorted(maps.Keys(o.Projections)) {
		p := ix.PublishedPath(stem)
		stemsOf[p] = append(stemsOf[p], stem)
	}
	reachedBy := map[string][]navItem{}
	for i, v := range o.Views {
		if !templateRendersADocument(v.Template) {
			continue
		}
		reached, err := v.ReachedStems(ix)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, stem := range reached {
			p := ix.PublishedPath(stem)
			if _, built := o.Projections[stem]; !built || seen[p] {
				continue
			}
			seen[p] = true
			reachedBy[p] = append(reachedBy[p], navItem{Label: nav[i].Label, Path: v.Path})
		}
	}
	out := make([]dataFileRef, 0, len(stemsOf))
	for _, p := range slices.Sorted(maps.Keys(stemsOf)) {
		f := dataFileRef{Path: p, Pages: reachedBy[p], CaveatsPath: caveatsPathOf(o)}
		var cited []sourceMeta
		caveats := map[string]bool{}
		for _, stem := range stemsOf[p] {
			var d struct {
				Metadata struct {
					FiscalYearLabel string       `json:"fiscal_year_label"`
					Basis           string       `json:"basis"`
					Caveats         []caveatMeta `json:"caveats"`
					Sources         []sourceMeta `json:"sources"`
				} `json:"metadata"`
			}
			if err := json.Unmarshal(o.Projections[stem], &d); err != nil {
				return nil, fmt.Errorf("decode %s metadata: %w", stem, err)
			}
			if f.Label == "" {
				f.Label = yearLabel(d.Metadata.FiscalYearLabel, d.Metadata.Basis)
			}
			for _, c := range d.Metadata.Caveats {
				caveats[c.ID] = true
			}
			cited = append(cited, d.Metadata.Sources...)
		}
		f.Caveats = len(caveats)
		f.Sources, _ = sourcesFor(unionSources(cited), byID, pageTextBase, o.RecordsBase)
		out = append(out, f)
	}
	return out, nil
}

// downloadRef is one whole-store artifact offered for download.
type downloadRef struct {
	Path  string
	Label string
	Note  string
	Bytes int
}

// buildProvenancePage renders the index of the published fact store.
//
// IT COMPOSES NO URL ITSELF, and that is the one thing worth guarding here.
// Every PDF and page-text link comes back out of sourcesFor, the same function
// every other page's citations go through, so a citation's pdfPageURL and
// pageTextFile are reached by exactly one path in this package. It matters
// beyond tidiness: the PDF link must target the city's canonical URL with
// #page=N and never a forge's raw host, which serves LFS pointer text rather
// than the document. A second
// composition here would be a second place for that to go wrong, on the one
// page whose entire purpose is that its links resolve.
//
// THE DATA LINK IS THE EXCEPTION AND IS NOT COMPOSED EITHER: it is
// PageIndexEntry.Data verbatim, the path the caller says it wrote the records
// to, which Options.validate has already checked against the files being
// written. This package does not know how that path is built and must not
// learn -- the locator-to-URL rule belongs to whoever produced the records.
func buildProvenancePage(o *Options, v View, nav []navItem, byID map[string]Doc,
	ix ColumnIndex, pageTextBase func(docID string) string) (provenancePageData, error) {
	if len(o.PageIndex) == 0 {
		// A page listing nothing is not an empty state, it is a page that
		// should not have been asked for: views() adds this one only when
		// there is an index, so reaching here means two callers disagree.
		return provenancePageData{}, fmt.Errorf(
			"view %q renders the provenance index and Options carries no page index", v.Path)
	}

	// The sources this page cites ARE its rows, so they are built from the
	// index rather than read out of a document -- and then put through the
	// same union and the same composition every other page uses.
	byDoc := map[string][]int{}
	order := []string{}
	for _, e := range o.PageIndex {
		if _, ok := byDoc[e.DocID]; !ok {
			order = append(order, e.DocID)
		}
		byDoc[e.DocID] = append(byDoc[e.DocID], e.Page)
	}
	metas := make([]sourceMeta, 0, len(order))
	for _, id := range order {
		metas = append(metas, sourceMeta{DocID: id, Pages: byDoc[id]})
	}
	sources, _ := sourcesFor(unionSources(metas), byID, pageTextBase, o.RecordsBase)

	// Index the composed refs so each row can take its own, rather than
	// recomposing them.
	refs := map[Citation]pageRef{}
	titles := map[string]string{}
	for _, s := range sources {
		titles[s.DocID] = s.Title
		for _, p := range s.Pages {
			refs[Citation{DocID: s.DocID, Page: p.Number}] = p
		}
	}

	rows := make([]provenanceRow, 0, len(o.PageIndex))
	records := 0
	for _, e := range o.PageIndex {
		ref, ok := refs[Citation{DocID: e.DocID, Page: e.Page}]
		if !ok {
			return provenancePageData{}, fmt.Errorf(
				"page index entry %s p%d composed no citation", e.DocID, e.Page)
		}
		records += e.Records
		rows = append(rows, provenanceRow{
			DocID: e.DocID, DocTitle: titles[e.DocID], Page: e.Page,
			Records: e.Records, Bytes: e.Bytes, Note: e.Note,
			DataURL: e.Data, PDFURL: ref.PDFURL, TextURL: ref.TextURL,
		})
	}

	downloads := make([]downloadRef, 0, len(o.Downloads))
	for _, d := range o.Downloads {
		// A conversion rather than a field-by-field copy: the two types have
		// the same shape ON PURPOSE -- Download is the caller's vocabulary and
		// downloadRef is the template's -- and a conversion cannot silently
		// drop a field the day one of them gains one.
		downloads = append(downloads, downloadRef(d))
	}

	inferred, err := inferredColumns(o, ix, refs)
	if err != nil {
		return provenancePageData{}, err
	}
	dataFiles, err := dataFilesOf(o, nav, byID, ix, pageTextBase)
	if err != nil {
		return provenancePageData{}, err
	}

	title := v.Title
	if title == "" {
		title = "City of Livermore budget: every published figure"
	}
	return provenancePageData{
		chrome: chrome{
			Title: title,
			Lede:  v.Lede,
			Nav:   nav,
			// The index is its own sources page: its rows list every page.
			SourcesPath: v.Path,
			SourcesPage: true,
			ExportedBy:  o.GeneratedBy,
			// NO Projections: DataFiles lists every data file, described.
			// NO DataPath. Every other page names the projection it draws;
			// this one draws none, and pointing it at an unrelated document
			// would be a false statement about where its figures came from.
		},
		Rows:      rows,
		Downloads: downloads,
		DataFiles: dataFiles,
		Documents: len(order),
		Pages:     len(rows),
		Records:   records,
		Inferred:  inferred,
		Wording:   defaultWording(),
	}, nil
}
