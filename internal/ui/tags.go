package ui

import (
	"context"
	"log"

	"mangareader/internal/app"
	"mangareader/internal/model"
	"mangareader/internal/ui/details"
)

// shellTags — правка тегов страницы произведения поверх сервисов: база и
// индекс — в горутинах, после успешной правки поиск выполняется заново.
type shellTags struct{ s *Shell }

var _ details.TagEditor = shellTags{}

func (t shellTags) Available() bool { return t.s.svc.TagsAvailable() }

func (t shellTags) Edit(k model.Key, op app.TagOp, cb func(model.Gallery, error)) {
	go func() {
		g, err := t.s.svc.EditTags(k, op)
		if err == nil {
			t.s.do(t.s.search.Rerun) // результат поиска отражает новые теги
		}
		cb(g, err)
	}()
}

func (t shellTags) Suggest(tagType, prefix string, limit int, cb func([]string)) {
	go func() {
		res, err := t.s.svc.Index.SuggestTags(context.Background(), tagType, prefix, limit)
		if err != nil {
			log.Printf("tag suggestions: %v", err)
		}
		names := make([]string, 0, len(res))
		for _, tc := range res {
			names = append(names, tc.Tag.Name)
		}
		cb(names)
	}()
}
