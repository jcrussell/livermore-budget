package structure

// Reference is the cut declared the reference among cuts: the one whose
// columns every agreement is held to, pp.66-67. false when none is.
func Reference(cuts []Cut) (Cut, bool) {
	for _, c := range cuts {
		if c.Reference {
			return c, true
		}
	}
	return Cut{}, false
}

// ExceptionsOn is the exceptions declared on one comparison at one level, in
// declaration order.
func ExceptionsOn(exceptions []Exception, cut, against string, at Level) []Exception {
	var out []Exception
	for _, e := range exceptions {
		if e.Cut == cut && e.Against == against && e.At == at {
			out = append(out, e)
		}
	}
	return out
}
