//go:build sqlite_fts5

package catalog

import (
	"path/filepath"
	"testing"

	"mangareader/internal/search"
	"mangareader/internal/search/indextest"
)

// Индекс каталога проходит тот же набор проверок, что и MemIndex.
func TestIndexConformance(t *testing.T) {
	indextest.Run(t, func(t *testing.T) search.Index {
		return openTest(t, filepath.Join(t.TempDir(), FileName), "root").Index()
	})
}
