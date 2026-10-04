package check

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/english"
	"github.com/jcrussell/livermore-budget/internal/project"
	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// projectionsBuild asserts every slice of the fact store that a projection was
// asked for actually produced a graph.
//
// THIS CHECK EXISTS BECAUSE ITS ABSENCE WAS SILENT IN BOTH DIRECTIONS. A
// projection that refuses to build used to abort check.Load, so `fisc verify`
// exited 1 having printed no report — every other finding in the run lost, and
// the reason wrapped inside a load error. Recording the refusal instead fixes
// that, and opens the opposite hole: a slice missing from Subject.Projections is
// not a failure anywhere, it is silence, and ten checks below read nothing but
// the graph. So this one must FAIL on a recorded refusal and must never be
// vacuous while one is recorded.
//
// The refusal it exists for today is internal/project's: netCells rejects a fact
// carrying a department, because the citywide spine crosses category against
// fund group and has no department tier (fisc-gxa.2). A correctly scoped
// pp.167-170 fact never reaches it — selectFacts filters on scope first — so on
// a healthy corpus this is vacuous, and what makes it reachable is a rule whose
// `scope:` says all-funds-gross when it means expenditure-by-department.
type projectionsBuild struct{}

var _ Check = (*projectionsBuild)(nil)

func (*projectionsBuild) ID() string { return "projections-build" }
func (*projectionsBuild) Tier() int  { return 1 }
func (*projectionsBuild) Full() bool { return false }
func (*projectionsBuild) Description() string {
	return "every projection the fact store's slices asked for produced a graph, rather than " +
		"refusing and taking the whole report down with it"
}

// Run counts one subject per SLICE a projection was asked for — the built ones
// and the failed ones together — because the claim is about all of them and a
// count of only the failures would read as "1 of 1" on a corpus where nine
// slices built and one did not.
//
// THE UNIT IS SLICES AND SO IS THE COUNT, which took a fix (fisc-rwo). It
// counted PROJECTIONS under that unit, and one projection stopped being one
// slice when the first multi-column document landed: keysOf was added to this
// file for exactly that reason -- its own doc comment says "anything keying a
// map on a projection has to iterate this rather than call it once" -- and this
// Run did not. Measured on the committed corpus, it reported "3 projection
// slices built" over six: the two sankey columns plus revenue-trends' four.
func (*projectionsBuild) Run(_ context.Context, s *Subject) (Result, error) {
	findings := make([]Finding, 0, len(s.ProjectionFailures))
	for _, f := range s.ProjectionFailures {
		findings = append(findings, finding(f.String(),
			"this projection refused to build: %v. Every check that reads a graph is "+
				"silent about this slice — they are not failing it, they cannot see it",
			f.Err))
	}
	slicesBuilt := 0
	for _, p := range s.Projections {
		slicesBuilt += len(keysOf(p.Options))
	}
	slicesRefused := 0
	for _, f := range s.ProjectionFailures {
		slicesRefused += len(keysOf(f.Options))
	}
	return conclusion{
		subjects: slicesBuilt + slicesRefused,
		unit:     "projection slices",
		held: fmt.Sprintf("%d %s built, none refused",
			slicesBuilt, english.Plural(slicesBuilt, "projection slice", "projection slices")),
		nothing:  "no projection was asked for at all",
		findings: findings,
	}.result(), nil
}

// sliceKey identifies one projection slice by year, basis and scope. selectFacts
// (internal/project) also filters on Options.Kinds, so a key covered here does
// not mean every fact at that address is drawn: transfers-out selects p222's
// transfers and not the grants beside them (fisc-jyjn).
type sliceKey struct {
	year  int
	basis vocab.Basis
	scope string
}

// keysOf is every slice one Options covers: one per column PER SCOPE.
//
// IT RETURNS A LIST BECAUSE ONE DOCUMENT NEED NOT BE ONE SLICE. While every
// projection was of a single (year, basis) this was a one-to-one map and was
// spelled as one; a trends document is of four columns of one schedule, so an
// Options that maps to a single key can no longer be assumed. Anything keying a
// map on a projection has to iterate this rather than call it once.
//
// THE PRODUCT OVER SCOPES IS LOAD-BEARING AND NOT TIDINESS. A sliceKey is the
// address a FACT has -- (year, basis, scope) -- so a document of two schedules
// occupies two addresses per column and must say so. projectionsBuild counts
// what was asked for from these keys, and emitting one key per column with a
// single scope would count a two-schedule document as half of what it draws.
func keysOf(o project.Options) []sliceKey {
	out := make([]sliceKey, 0, len(o.Columns)*len(o.Scopes))
	for _, c := range o.Columns {
		for _, scope := range o.Scopes {
			out = append(out, sliceKey{c.FiscalYear, c.Basis, scope})
		}
	}
	return out
}

// publishedProjectionBuilt asserts EVERY document the site publishes was built,
// over every column it publishes it over.
//
// A projection that was not built fails nowhere else: Trends.Slices returns
// nil when the store carries no revenue-by-fund fact, projections-build counts
// only what was asked for, and the trend checks go vacuous. Columns are checked
// one at a time for the same reason: a trends document that lost FY2023-24
// builds, and every series is complete over the columns that remain (fisc-7dt).
type publishedProjectionBuilt struct{}

var _ Check = (*publishedProjectionBuilt)(nil)

func (*publishedProjectionBuilt) ID() string { return "published-projection-built" }
func (*publishedProjectionBuilt) Tier() int  { return 1 }
func (*publishedProjectionBuilt) Full() bool { return false }
func (*publishedProjectionBuilt) Description() string {
	return "every document `fisc export` publishes was built, over every column it " +
		"publishes it over, and is among the projections these checks were run over"
}

// Run has one subject per published document; a repository publishing nothing
// has nothing here to look at.
func (*publishedProjectionBuilt) Run(_ context.Context, s *Subject) (Result, error) {
	// Every SLICE built, grouped by the projection and scope that built it. A
	// published document must be covered by ONE of them, not their union: a
	// four-column document is otherwise satisfied by four single-column
	// slices while the file at its stem covers a quarter of it (fisc-b8o).
	//
	// It is keyed on the projection NAME, and that is not belt-and-braces.
	// Since each projection is built over the slices it declares
	// (project.Sliced), "some projection was built at the published triple"
	// does not imply the published DOCUMENT was: a second projection whose
	// slices happen to include that triple would satisfy this check while the
	// site's chart was of nothing.
	// Keyed on (name, scope) and not on the name alone, so that a projection
	// which one day builds two schedules cannot have the columns of one satisfy
	// a published document of the other. Nothing does that today; the key costs
	// nothing and the alternative is a silent wrong answer rather than a
	// refusal.
	type source struct{ name, scopes string }
	built := map[source][]project.Options{}
	names := make([]string, 0, len(s.Projections))
	for _, p := range s.Projections {
		k := source{p.Name, scopeKey(p.Options.Scopes)}
		built[k] = append(built[k], p.Options)
		names = append(names, p.String())
	}

	var findings []Finding
	// The slices some published document claims, so the sweep below can report
	// the ones none does.
	claimed := map[string]bool{}
	for _, d := range s.Published {
		// project.MissingColumns rather than a comparison written here: `fisc
		// export` asks the same question of the documents it wrote, and two
		// spellings of "does this cover what we said" is the divergence the
		// shared declaration exists to remove. It compares the scope too, so a
		// projection of the right name built over a different SCHEDULE does not
		// satisfy a published document by accident.
		// The BEST-covering slice, not the first, and it is marked claimed even
		// when it covers the document only partly. A slice short a column is
		// already reported here, by name and by which column; letting the sweep
		// below report it a second time as "no document covers this" would give
		// one defect two findings that read as two, and send a reader looking
		// for a stray document that is really the declared one gone short.
		absent := d.Columns
		for _, o := range built[source{d.Projection, scopeKey(d.Scopes)}] {
			missing := project.MissingColumns(d, o)
			if len(missing) < len(absent) {
				absent = missing
				claimed[sliceID(d.Projection, o)] = true
			}
			if len(absent) == 0 {
				break
			}
		}
		if len(absent) == 0 {
			continue
		}
		detail := "no projection was built at all"
		if len(names) > 0 {
			detail = fmt.Sprintf("the projections built were: %s", strings.Join(names, ", "))
		}
		// The finding names the DOCUMENT, then which of its columns is missing,
		// because those are two different repairs. A whole document absent is a
		// projection that built nothing -- a scope typo, a registry entry
		// dropped. A column absent from a document that built is the corpus
		// having lost a printed column, and the fix is in the mapping.
		what := "and nothing checked it"
		if len(absent) < len(d.Columns) {
			what = fmt.Sprintf("and it was built without %s", project.Describe(absent))
		}
		findings = append(findings, finding(d.String(),
			"`fisc export` publishes this document %s — %s. Every check below this one "+
				"reads the projections, so a document that was not built is not failed, "+
				"it is unexamined", what, detail))
	}

	// THE OTHER DIRECTION, and it is the half fisc-b8o's acceptance criterion
	// asks for. A slice that built and that no published document claims is a
	// file `fisc export` writes and the site never declares -- and the way it
	// bites is not the stray file: it is that the site would serve a document it
	// never declared, under a name nothing else expects.
	//
	// It asks project.Stem rather than pkg/cmd/export's stemFor, and the
	// distinction is the decision fisc-b8o left open. Stem is the shared
	// declaration -- one place, read by the packager that writes the files, by
	// project.PublishedDocuments that declares them, and by this check -- so
	// reading it is this package doing its job, where reaching into a command's
	// naming helper would have been the coupling internal/check avoids
	// everywhere else.
	at := map[string]string{}
	for _, d := range s.Published {
		at[d.Stem] = d.String()
	}
	// THE DECLARED SLICES OF EACH PROJECTION, BY NAME, built and refused
	// together -- which is the list `fisc export` names its files from, and the
	// reason it is assembled here rather than reusing `built` above. `built` is
	// keyed on (name, scope) and holds only what BUILT, so a projection that
	// declared three slices and refused one would be named against a list of
	// two, and this check would report a stem export never writes. The one
	// spelling of the rule is only worth having if its argument is one thing
	// too.
	declared := map[string][]project.Options{}
	for _, p := range s.Projections {
		declared[p.Name] = append(declared[p.Name], p.Options)
	}
	for _, f := range s.ProjectionFailures {
		declared[f.Name] = append(declared[f.Name], f.Options)
	}
	for _, p := range s.Projections {
		if claimed[sliceID(p.Name, p.Options)] {
			continue
		}
		stem, err := project.Stem(p.Name, p.Options, declared[p.Name])
		if err != nil {
			// An Options with no columns. It built, so it is not
			// projectionsBuild's subject; it cannot be named, so it is not this
			// sweep's either, and the graph checks read it like any other.
			continue
		}
		detail := "the site declares no document at that stem, so `fisc export` would " +
			"write a file nothing published claims"
		if other, ok := at[stem]; ok {
			detail = fmt.Sprintf("the site already publishes %s at that stem, so `fisc "+
				"export` computes one stem for two documents and its duplicate-stem guard "+
				"refuses the ENTIRE export -- no file written, including the ones that "+
				"were fine", other)
		}
		findings = append(findings, finding(p.String(),
			"this slice was built and no document the site publishes covers it. It would "+
				"be written at stem %q, and %s", stem, detail))
	}

	return conclusion{
		// One subject per published document, so the summary counts what the
		// site serves rather than what happened to build.
		subjects: len(s.Published),
		unit:     "published documents",
		held: fmt.Sprintf("every document the site publishes was built over every column "+
			"it publishes: %s", strings.Join(describeDocuments(s.Published), ", ")),
		nothing:  "the site publishes no document, so there is nothing to have built",
		findings: findings,
	}.result(), nil
}

// describeDocuments names the published documents the way the report should: the
// stem a reader can fetch, then the columns and the scope behind it, so one can
// be matched against the projections listed elsewhere in the run.
// sliceID names one built slice, for telling a claimed slice from an unclaimed
// one. Name, scope and every column, so two slices of one projection that
// differ only in basis are two ids and not one.
func sliceID(name string, o project.Options) string {
	return name + "\x1f" + scopeKey(o.Scopes) + "\x1f" + project.Describe(o.Columns)
}

// scopeKey is a scope SET as one comparable string.
//
// Sorted, unlike [project.Options.ScopeList], and the difference is the point:
// ScopeList is published text and keeps the order a projection declared, while
// this is a map key and two declarations of the same schedules must land on it
// however they were written. A sorted copy rather than a sort in place, because
// the slice it is handed is a projection's own declaration.
func scopeKey(scopes []string) string {
	sorted := slices.Clone(scopes)
	slices.Sort(sorted)
	return strings.Join(sorted, "\x1f")
}

func describeDocuments(docs []project.PublishedDocument) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.String())
	}
	return out
}
