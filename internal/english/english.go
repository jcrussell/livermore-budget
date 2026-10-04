// Package english is the wording a message needs and a format verb cannot
// supply.
package english

// Plural is one when n is 1 and many otherwise.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
