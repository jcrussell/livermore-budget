package structure

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jcrussell/livermore-budget/internal/fact"
	"github.com/jcrussell/livermore-budget/internal/mapping"
)

// A Split is a coarse cut whose money, in some kinds, no finer cut decomposes
// at the grain the lattice compares them: pp.66-67's TRANSFER OUT is p76's
// list plus p222's transfers to the CIP, and neither page alone sums to it;
// and it is pp.186-209's Transfers Out plus Transfers Out to CIP, one page's
// two columns under two categories the spine prints as one line.
//
// The pair of the Whole and any one Part is compared without Kinds, and the
// Parts are summed and held to the Whole at At instead.
type Split struct {
	Name  string
	Whole string
	Parts []string
	// At is the level the Parts are summed to. Every Part and the Whole must
	// refine it, and it may drop an axis on which the Parts name the money
	// differently from the Whole.
	At    Level
	Kinds []mapping.Kind
}

// BudgetBookSplits are the Budget Book's splits.
func BudgetBookSplits() []Split {
	return []Split{{
		// Category is not an axis of the level: the spine prints TRANSFER OUT
		// and pp.186-209 print it as two columns, Transfers Out and Transfers
		// Out to CIP, which agree with the spine only by group.
		Name:  "a-fund-transfers-out-or-to-the-cip",
		Whole: CutSpine,
		Parts: []string{CutFundBalanceFlows},
		At:    LevelFundGroup,
		Kinds: []mapping.Kind{mapping.KindTransferOut},
	}, {
		// Category is not an axis of the level: the spine prints TRANSFER OUT
		// and p222's rows are the "Transfers Out to CIP" pp.69-75 print beside
		// it, so the two name one money differently and agree only by group.
		Name:  "a-transfer-out-is-p76-or-to-the-cip",
		Whole: CutSpine,
		Parts: []string{"transfers-detail", "cip-transfers-out"},
		At:    LevelFundGroup,
		Kinds: []mapping.Kind{mapping.KindTransferOut},
	}}
}

// ValidateSplits refuses a split naming an undeclared cut, a part twice or
// the whole as a part, no kind, a kind a side does not print, a level a side
// does not refine, or a pair two splits both claim.
func ValidateSplits(cuts []Cut, splits []Split) error {
	byName := map[string]Cut{}
	for _, c := range cuts {
		byName[c.Name] = c
	}
	claimed := map[[2]string]string{}
	names := map[string]bool{}
	for _, s := range splits {
		if names[s.Name] {
			return fmt.Errorf("split %q is declared twice", s.Name)
		}
		names[s.Name] = true
		whole, ok := byName[s.Whole]
		if !ok {
			return fmt.Errorf("split %q: whole %q is not a declared cut", s.Name, s.Whole)
		}
		if len(s.Parts) == 0 {
			return fmt.Errorf("split %q names no part", s.Name)
		}
		// One part is a split only where At drops an axis the pair meets on;
		// at the meet itself the lattice already compares the two.
		if len(s.Parts) == 1 {
			part, ok := byName[s.Parts[0]]
			if !ok {
				return fmt.Errorf("split %q: part %q is not a declared cut", s.Name, s.Parts[0])
			}
			meet, err := Meet(part.Level, whole.Level)
			if err != nil {
				return fmt.Errorf("split %q names one part, and the lattice places no grain it compares %q with %q at: %w",
					s.Name, part.Name, whole.Name, err)
			}
			if meet == s.At {
				return fmt.Errorf("split %q names one part at %q, where the lattice compares %q with %q; "+
					"one part decomposing the whole at their meet is a containment", s.Name, s.At, part.Name, whole.Name)
			}
		}
		if len(s.Kinds) == 0 {
			return fmt.Errorf("split %q names no kind", s.Name)
		}
		if Axes(s.At) == nil {
			return fmt.Errorf("split %q: %q is not a declared level", s.Name, s.At)
		}
		sides := []Cut{whole}
		seen := map[string]bool{}
		for _, p := range s.Parts {
			c, ok := byName[p]
			if !ok {
				return fmt.Errorf("split %q: part %q is not a declared cut", s.Name, p)
			}
			if p == s.Whole || seen[p] {
				return fmt.Errorf("split %q names %q twice", s.Name, p)
			}
			seen[p] = true
			sides = append(sides, c)
			key := [2]string{p, s.Whole}
			if other, dup := claimed[key]; dup {
				return fmt.Errorf("splits %q and %q both relate %q to %q", other, s.Name, p, s.Whole)
			}
			claimed[key] = s.Name
		}
		for _, c := range sides {
			if c.Outside != "" {
				return fmt.Errorf("split %q: %q is outside the reference and is compared with no cut", s.Name, c.Name)
			}
			if len(c.FundGroups) > 0 {
				return fmt.Errorf("split %q: %q covers fund groups %v only, and a split sums every side over every group",
					s.Name, c.Name, c.FundGroups)
			}
			if c.Level != s.At && !Refines(c.Level, s.At) {
				return fmt.Errorf("split %q: %q is at %q, which does not refine %q", s.Name, c.Name, c.Level, s.At)
			}
			for _, k := range s.Kinds {
				if !containsKind(c.Kinds, k) {
					return fmt.Errorf("split %q: %q does not print %s", s.Name, c.Name, k)
				}
			}
		}
	}
	return nil
}

// SplitPair is the pair a and b as the lattice compares them once a split
// relating them has taken its kinds. held is true when a split relates them
// and leaves no kind both print, so the split is their only comparison.
func SplitPair(a, b Cut, splits []Split) (Cut, Cut, bool) {
	split := false
	for _, s := range splits {
		for _, p := range s.Parts {
			if (a.Name == p && b.Name == s.Whole) || (b.Name == p && a.Name == s.Whole) {
				a.Kinds = withoutKinds(a.Kinds, s.Kinds)
				b.Kinds = withoutKinds(b.Kinds, s.Kinds)
				split = true
			}
		}
	}
	return a, b, split && !kindsMeet(a, b)
}

func withoutKinds(have, drop []mapping.Kind) []mapping.Kind {
	var out []mapping.Kind
	for _, k := range have {
		if !containsKind(drop, k) {
			out = append(out, k)
		}
	}
	return out
}

// HoldSplit sums a split's parts at its level and compares them with the
// whole in the whole's columns. A part's column the whole does not print is
// outside the comparison, as a finer cut's is under Contain.
func HoldSplit(facts []fact.Fact, cuts []Cut, s Split) (Comparison, error) {
	byName := map[string]Cut{}
	for _, c := range cuts {
		byName[c.Name] = c
	}
	whole, ok := byName[s.Whole]
	if !ok {
		return Comparison{}, fmt.Errorf("split %q: whole %q is not a declared cut", s.Name, s.Whole)
	}
	r := restriction{kinds: s.Kinds}
	ref, err := project(facts, whole, s.At, r)
	if err != nil {
		return Comparison{}, err
	}
	sum := map[Key]Sum{}
	for _, name := range s.Parts {
		part, ok := byName[name]
		if !ok {
			return Comparison{}, fmt.Errorf("split %q: part %q is not a declared cut", s.Name, name)
		}
		cells, err := project(facts, part, s.At, r)
		if err != nil {
			return Comparison{}, err
		}
		for k, v := range cells {
			have := sum[k]
			have.Cents += v.Cents
			have.Present = true
			sum[k] = have
		}
	}
	columns := map[string]bool{}
	for k := range ref {
		columns[k.Column()] = true
	}
	// Named by the split, not its parts: a split of one part would otherwise
	// share its name with the lattice's comparison of that part.
	out := Comparison{
		Cut:      Cut{Name: s.Name},
		Against:  whole,
		Relation: Containment,
		At:       s.At,
	}
	out.tally(sum, ref, func(k Key) bool { return columns[k.Column()] })
	return out, nil
}

// ValidateOutside holds each cut declared Outside to the store: every fact it
// admits carries a fund, and no such fund is carried by a fact another cut
// admits. It returns one line per fact or fund at fault.
func ValidateOutside(facts []fact.Fact, cuts []Cut) []string {
	var out []string
	for _, c := range cuts {
		if c.Outside == "" {
			continue
		}
		if c.Reference {
			out = append(out, fmt.Sprintf("cut %q is the reference and declares itself outside it", c.Name))
			continue
		}
		mine := map[int]bool{}
		for i := range facts {
			f := &facts[i]
			if !c.admits(f) {
				continue
			}
			if f.Fund == nil {
				out = append(out, fmt.Sprintf("fact %s is in cut %q, which declares itself outside the "+
					"reference, and carries no fund to hold that claim to", f.ID, c.Name))
				continue
			}
			mine[*f.Fund] = true
		}
		shared := map[int][]string{}
		for i := range facts {
			f := &facts[i]
			if f.Fund == nil || !mine[*f.Fund] || c.admits(f) {
				continue
			}
			for _, o := range cuts {
				if o.Name != c.Name && o.admits(f) && !contains(shared[*f.Fund], o.Name) {
					shared[*f.Fund] = append(shared[*f.Fund], o.Name)
				}
			}
		}
		funds := make([]int, 0, len(shared))
		for fund := range shared {
			funds = append(funds, fund)
		}
		sort.Ints(funds)
		for _, fund := range funds {
			others := shared[fund]
			out = append(out, fmt.Sprintf("cut %q declares itself outside the reference and fund %d is "+
				"also carried by %s; money in both cannot be outside one and inside the other",
				c.Name, fund, strings.Join(others, ", ")))
		}
	}
	return out
}
