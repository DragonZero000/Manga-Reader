package search

import (
	"context"

	"mangareader/internal/model"
)

// TagCount — тег и количество произведений с ним (для подсказок).
type TagCount struct {
	Tag   model.Tag
	Count int
}

// Index — контракт индекса поиска. UI зависит только от этого интерфейса.
type Index interface {
	// Upsert добавляет или обновляет произведение.
	Upsert(ctx context.Context, g model.Gallery) error
	// Remove удаляет произведение по ключу. Отсутствие ключа — не ошибка.
	Remove(ctx context.Context, k model.Key) error
	// Search возвращает ключи страницы выдачи и общее число совпадений.
	Search(ctx context.Context, q Query) (keys []model.Key, total int, err error)
	// SuggestTags возвращает теги по типу (пусто — любой) и префиксу имени.
	SuggestTags(ctx context.Context, tagType, prefix string, limit int) ([]TagCount, error)
}

// NopIndex — пустой индекс: ничего не хранит, ничего не находит.
type NopIndex struct{}

var _ Index = NopIndex{}

func (NopIndex) Upsert(context.Context, model.Gallery) error { return nil }

func (NopIndex) Remove(context.Context, model.Key) error { return nil }

func (NopIndex) Search(_ context.Context, q Query) ([]model.Key, int, error) {
	if err := q.Validate(); err != nil {
		return nil, 0, err
	}
	return nil, 0, nil
}

func (NopIndex) SuggestTags(context.Context, string, string, int) ([]TagCount, error) {
	return nil, nil
}
