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
