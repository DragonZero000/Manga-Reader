package model

import "strings"

// Известные типы тегов (совпадают с типами в meta.json).
const (
	TagTypeTag       = "tag"
	TagTypeArtist    = "artist"
	TagTypeGroup     = "group"
	TagTypeParody    = "parody"
	TagTypeCharacter = "character"
	TagTypeLanguage  = "language"
	TagTypeCategory  = "category"
)

var knownTagTypes = map[string]bool{
	TagTypeTag:       true,
	TagTypeArtist:    true,
	TagTypeGroup:     true,
	TagTypeParody:    true,
	TagTypeCharacter: true,
	TagTypeLanguage:  true,
	TagTypeCategory:  true,
}

// KnownTagTypes возвращает известные типы тегов в фиксированном порядке.
func KnownTagTypes() []string {
	return []string{TagTypeTag, TagTypeArtist, TagTypeGroup, TagTypeParody,
		TagTypeCharacter, TagTypeLanguage, TagTypeCategory}
}

// Tag — типизированный тег произведения. Создавать через NewTag,
// чтобы тип и имя были нормализованы.
type Tag struct {
	Type string
	Name string
}

// NewTag создаёт тег с нормализованными типом и именем.
func NewTag(tagType, name string) Tag {
	return Tag{Type: normalize(tagType), Name: normalize(name)}
}

// ParseTag разбирает каноническую строку "тип:имя". Без ":" тип считается "tag".
func ParseTag(s string) Tag {
	if t, n, ok := strings.Cut(s, ":"); ok {
		return NewTag(t, n)
	}
	return NewTag(TagTypeTag, s)
}

// String возвращает каноническую строку тега "тип:имя".
func (t Tag) String() string {
	return t.Type + ":" + t.Name
}

// Known сообщает, относится ли тег к известному типу.
func (t Tag) Known() bool {
	return knownTagTypes[t.Type]
}

// IsZero сообщает, пуст ли тег.
func (t Tag) IsZero() bool {
	return t.Type == "" && t.Name == ""
}

// normalize приводит строку к нижнему регистру и схлопывает пробелы.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
