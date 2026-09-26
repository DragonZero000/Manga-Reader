package search_test

import (
	"testing"

	"mangareader/internal/search"
	"mangareader/internal/search/indextest"
)

// MemIndex — эталон общего набора проверок индексов.
func TestMemIndexConformance(t *testing.T) {
	indextest.Run(t, func(*testing.T) search.Index { return search.NewMemIndex() })
}
