//go:build sqlite_fts5

package stats

import (
	"context"
	"path/filepath"
	"testing"

	"mangareader/internal/catalog"
)

func TestConformanceCatalog(t *testing.T) {
	c, err := catalog.Open(filepath.Join(t.TempDir(), catalog.FileName), "root")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	idx := c.Index()
	for _, gl := range testLibrary() {
		if err := idx.Upsert(context.Background(), gl); err != nil {
			t.Fatal(err)
		}
	}
	CheckConformance(t, idx)
}
