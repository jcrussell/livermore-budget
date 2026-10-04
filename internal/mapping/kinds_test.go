package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestTheMessagesThatSayFiveStillMeanFive pins the count parse errors state in
// prose. Adding a sixth kind must redden this rather than leave "is not one of
// the five" quietly wrong on a page a reader is trying to fix.
//
// THE NUMBER OF SUCH MESSAGES IS MEASURED RATHER THAN NAMED: a count in a
// failure message a reader acts on goes stale exactly the way the messages it
// is about would. Counting them here cannot rot, and it costs one file read.
func TestTheMessagesThatSayFiveStillMeanFive(t *testing.T) {
	src, err := os.ReadFile("parse.go")
	if err != nil {
		t.Fatalf("read parse.go: %v", err)
	}
	n := strings.Count(string(src), "not one of the five")
	if n == 0 {
		t.Fatal("no parse error says \"not one of the five\"; either they were reworded " +
			"or this test is reading the wrong file")
	}
	if len(vocab.Kinds()) != 5 {
		t.Errorf("len(vocab.Kinds()) = %d, want 5; parse.go says \"not one of the five\" in "+
			"%d message(s) and they are now wrong", len(vocab.Kinds()), n)
	}
}
