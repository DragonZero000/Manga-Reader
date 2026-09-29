package screens

import (
	"strings"
	"testing"

	"mangareader/internal/search"
)

// Примеры справки разбираются без ошибок; среди них свои и скрытые теги.
func TestHelpExamples(t *testing.T) {
	var all []string
	for _, ex := range helpExamples {
		if _, err := search.Parse(ex[0]); err != nil {
			t.Errorf("пример %q: %v", ex[0], err)
		}
		all = append(all, ex[0])
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"custom-tag:", "hidden-tag:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("в справке нет %q", want)
		}
	}
}
