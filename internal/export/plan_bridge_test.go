package export

// PlannedPaths is the order p writes its files in, for the external tests:
// Write returns what it wrote sorted, so the order is visible nowhere else.
func PlannedPaths(p *plan) []string {
	paths := make([]string, len(p.files))
	for i, f := range p.files {
		paths[i] = f.Path
	}
	return paths
}

// RemoveListed is removeListed, for the external tests: what it is handed
// was decided earlier, so a test can hand it a directory that has since
// stopped being empty without racing a writer.
var RemoveListed = removeListed
