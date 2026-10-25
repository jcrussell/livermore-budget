package mapping

import (
	"os"
	"strings"
	"testing"

	"github.com/jcrussell/livermore-budget/internal/vocab"
)

// TestTheMessagesThatSayFiveStillMeanFive: adding a sixth kind reddens this
// rather than leaving "is not one of the five" wrong; the messages are counted,
// not named.
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
