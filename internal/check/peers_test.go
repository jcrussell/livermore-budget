package check

import (
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// TestTheCommittedPeersOverlapOnlyByDeclaredIdentity pins, by name, what the
// peer check covers over the committed corpus: which pairs at one level were
// compared, the one overlap and its figure, and which pairs were refused with
// which reason.
func TestTheCommittedPeersOverlapOnlyByDeclaredIdentity(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	res := resultFor(t, runOne(t, s, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusPass {
		t.Fatalf("status = %s, findings:\n  %v", res.Status, res.Findings)
	}
	if res.Subjects != 22 {
		t.Errorf("subjects = %d, want the 22 cells pp.127-140 and p76 share", res.Subjects)
	}
	for _, want := range []string{
		"22 shared cell(s) over 3 pair(s)",
		`revenue-detail + transfers-detail at fund-by-category: 22 shared cell(s), 22 under identity "a-transfer-in-is-printed-at-both-ends" carrying $42183495.00 on each side`,
		"acfr-general-fund-summary + acfr-fund-balances/general at fund-group-by-category: 0 shared cell(s)",
		"acfr-changes-in-fund-balances + acfr-fund-balances/other-governmental at category: 0 shared cell(s)",
		`peers "spine" and "acfr-general-fund-summary": "spine" prints [adopted] columns and "acfr-general-fund-summary" prints [audited]; there is no column both print`,
		`peers "spine" and "acfr-fund-balances/general"`,
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary does not say %q", want)
		}
	}
}

// TestThePeerCheckGoesRed runs the amount mutation through the registered
// check, so what is proven is the line fisc verify prints. The identity
// deletion and the invented identity are run through the binary in the commit
// message, since the check reads the declarations from the package.
func TestThePeerCheckGoesRed(t *testing.T) {
	s, err := Load(LoadOptions{Root: repoRoot(t), Version: testVersion})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	planted := make([]fact.Fact, len(s.Facts))
	copy(planted, s.Facts)
	moved := 0
	for i := range planted {
		f := &planted[i]
		if f.Scope == "transfers-by-fund" && f.Kind == mapping.KindTransferIn && f.FiscalYear == 2027 &&
			f.Fund != nil && *f.Fund == 610 {
			f.AmountCents += 100
			moved++
			break
		}
	}
	if moved == 0 {
		t.Fatal("no p76 transfer into Stormwater to move")
	}
	mutated := *s
	mutated.Facts = planted
	res := resultFor(t, runOne(t, &mutated, &peersOverlapOnlyByDeclaredIdentity{}), "peers-overlap-only-by-declared-identity")
	if res.Status != StatusFail || len(res.Findings) != 1 {
		t.Fatalf("status %s with %d findings, want one failure:\n  %v", res.Status, len(res.Findings), res.Findings)
	}
	for _, want := range []string{"fund=610", "the pages disagree", "$1.00"} {
		if !strings.Contains(res.Findings[0].Detail, want) {
			t.Errorf("the finding does not say %q: %s", want, res.Findings[0].Detail)
		}
	}
}
