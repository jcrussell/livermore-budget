// Package geomtest builds geometry artifacts for tests that write their own
// page text rather than copying a committed page.
package geomtest

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Monospaced is the geometry a monospaced page implies, in the extractor's
// format: each word at its character column, six points a character, one line
// every twelve points and ten tall. Words come out in the extractor's (y0, x0)
// order, which geom.ParsePage requires. Each word is written as
// [x0,y0,x1,y1,"text"] on a line of its own, so a test may find and edit one
// by its text.
func Monospaced(docID string, page int, text string) string {
	var words []string
	for i, line := range strings.Split(text, "\n") {
		for c := 0; c < len(line); {
			if line[c] == ' ' {
				c++
				continue
			}
			e := c
			for e < len(line) && line[e] != ' ' {
				e++
			}
			words = append(words, fmt.Sprintf("  [%d,%d,%d,%d,%s]", c*6, i*12, e*6, i*12+10, quote(line[c:e])))
			c = e
		}
	}
	return fmt.Sprintf("{\n \"doc_id\": %s,\n \"height\": 792.0,\n \"page\": %d,\n"+
		" \"schema_version\": 1,\n \"width\": 612.0,\n \"words\": [\n%s\n ]\n}\n",
		quote(docID), page, strings.Join(words, ",\n"))
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err) // a Go string always marshals
	}
	return string(b)
}
