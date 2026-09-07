package check

import (
	"strings"
	"testing"
)

// TestNoScopeReasonCallsTheTenYearColumnsAuditedResults guards the prose
// `fisc verify` prints to an operator, which is a claim about the documents
// exactly as a published page is.
//
// THE SUBJECT IS THE PHRASE, NOT THE WORD. "audited" is a Basis value and
// names p54's statement truthfully, so a reason may carry it; what no reason
// may say is that the ten-year schedules are audited YEARS, because p161 heads
// that section "Statistical Section (Unaudited)" and p29 disclaims all
// assurance over it. The second half below is what keeps this from becoming a
// ban on the word: it fails if the true sentence naming p54 disappears, so a
// reason set stripped of both halves is a finding rather than a pass.
func TestNoScopeReasonCallsTheTenYearColumnsAuditedResults(t *testing.T) {
	reasons := map[string]string{}
	for scope, reason := range unprojectedScopes {
		reasons["unprojectedScopes["+scope+"]"] = reason
	}
	for pair, reason := range disjointScopes {
		reasons["disjointScopes["+pair.a+", "+pair.b+"]"] = reason
	}
	if len(reasons) == 0 {
		t.Fatal("neither reason map yielded an entry, so this test read nothing")
	}

	for where, reason := range reasons {
		if i := strings.Index(strings.ToLower(reason), "audited year"); i >= 0 {
			t.Errorf("%s says %q; the ten-year schedules sit in the section p161 heads "+
				"\"Statistical Section (Unaudited)\", and fisc verify prints this to an operator",
				where, excerpt(reason, i))
		}
	}

	var namesP54 bool
	for _, reason := range reasons {
		if strings.Contains(reason, "audited statement") {
			namesP54 = true
			break
		}
	}
	if !namesP54 {
		t.Error("no reason names the audited statement any more, so the check above " +
			"has lost its subject: decide whether the corroboration was meant to go " +
			"before deleting this half")
	}
}

// excerpt is the phrase in the words around it, so a failure names the sentence
// rather than the map key alone.
func excerpt(reason string, at int) string {
	lo := max(at-60, 0)
	hi := min(at+60, len(reason))
	return reason[lo:hi]
}
