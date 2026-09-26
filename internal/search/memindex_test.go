package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mangareader/internal/model"
)

// BenchmarkSearch300 — поиск по 300 произведениям с 5 тегами каждое.
func BenchmarkSearch300(b *testing.B) {
	idx := NewMemIndex()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		g := model.Gallery{Key: model.LocalKey(fmt.Sprintf("g%03d.zip", i)), ExternalID: int64(100000 + i),
			Title: fmt.Sprintf("Gallery %03d — test title", i), Uploaded: base.Add(time.Duration(i) * time.Hour),
			Pages: make([]model.Page, 20), Favorites: i, File: model.FileInfo{Size: int64(i) << 20, ModTime: base}}
		for j := 0; j < 5; j++ {
			g.AddTag(model.NewTag("tag", fmt.Sprintf("tag %d", (i+j)%17)))
		}
		g.AddTag(model.NewTag("artist", fmt.Sprintf("artist %d", i%29)))
		idx.Upsert(context.Background(), g)
	}
	for _, query := range []string{"title", `tag:"tag 1" pages:>10 -artist:"artist 3"`, "zzzz"} {
		q, err := Parse(query)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(query, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				idx.Search(context.Background(), q)
			}
		})
	}
}
