package search

import (
	"strings"

	"mangareader/internal/model"
)

// TagQuery — строка поиска по тегу: `тип:"имя"` для известных типов,
// `tag:"имя"` (тег любого типа) — для остальных. Кавычки в имени удаляются.
func TagQuery(tagType, name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	for _, typ := range model.KnownTagTypes() {
		if typ == tagType {
			return tagType + `:"` + name + `"`
		}
	}
	return `tag:"` + name + `"`
}
