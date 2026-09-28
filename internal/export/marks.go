package export

// Gap licenses one gap mark: the column it may stand in, the signed cents it
// comes to there -- what the chart above sends into the opened node less what
// the drawn document draws out of it -- and why the two documents differ, as
// terminated sentences. The cents is the one figure of a gap the client cannot
// sum for itself: it is structure's, and the client holds the drawn difference
// to it.
type Gap struct {
	FiscalYear int    `json:"fiscal_year"`
	Basis      string `json:"basis"`
	Cents      int64  `json:"cents"`
	Reason     string `json:"reason"`
}

// Gaps is one node's licences, one per column it differs in.
type Gaps []Gap
