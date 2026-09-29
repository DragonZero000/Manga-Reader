package details

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// knownTypeOrder — порядок групп тегов и ключи перевода их подписей.
var knownTypeOrder = []struct{ typ, label string }{
	{model.TagTypeArtist, "tags.artist"},
	{model.TagTypeGroup, "tags.group"},
	{model.TagTypeParody, "tags.parody"},
	{model.TagTypeCharacter, "tags.character"},
	{model.TagTypeLanguage, "tags.language"},
	{model.TagTypeCategory, "tags.category"},
	{model.TagTypeTag, "tags.tag"},
}

// GroupTag — тег группы с происхождением.
type GroupTag struct {
	Name   string
	Origin model.TagOrigin
}

// Group — теги одного типа.
type Group struct {
	Type  string
	Label string
	Tags  []GroupTag
}

// Names — имена тегов группы по порядку.
func (g Group) Names() []string {
	out := make([]string, len(g.Tags))
	for i, t := range g.Tags {
		out[i] = t.Name
	}
	return out
}

// TagGroups группирует теги по типам: сначала известные типы в фиксированном
// порядке, затем неизвестные по алфавиту. Внутри группы — оригинальные теги
// в порядке meta.json, затем свои в порядке добавления. Скрытые теги
// показываются только при editing. Группы без тегов не возвращаются, кроме
// типов extra (поле ввода новой группы).
func TagGroups(views []model.TagView, editing bool, extra ...string) []Group {
	byType := map[string][]GroupTag{}
	for _, t := range views {
		if strings.TrimSpace(t.Name) == "" || (t.Origin == model.OriginHidden && !editing) {
			continue
		}
		byType[t.Type] = append(byType[t.Type], GroupTag{t.Name, t.Origin})
	}
	for _, typ := range extra {
		if _, ok := byType[typ]; !ok {
			byType[typ] = nil
		}
	}
	var out []Group
	known := map[string]bool{}
	for _, k := range knownTypeOrder {
		known[k.typ] = true
		if tags, ok := byType[k.typ]; ok {
			out = append(out, Group{Type: k.typ, Label: i18n.T(k.label), Tags: tags})
		}
	}
	var unknown []string
	for typ := range byType {
		if !known[typ] {
			unknown = append(unknown, typ)
		}
	}
	sort.Strings(unknown)
	for _, typ := range unknown {
		out = append(out, Group{Type: typ, Label: groupLabel(typ), Tags: byType[typ]})
	}
	return out
}

// groupLabel — подпись группы тегов типа typ.
func groupLabel(typ string) string {
	for _, k := range knownTypeOrder {
		if k.typ == typ {
			return i18n.T(k.label)
		}
	}
	if typ == "" {
		return i18n.T("tags.other")
	}
	return typ
}

// missingTypes — известные типы, групп которых нет среди groups (для
// «Добавить в другую группу»), в порядке групп.
func missingTypes(groups []Group) []string {
	shown := map[string]bool{}
	for _, g := range groups {
		shown[g.Type] = true
	}
	var out []string
	for _, k := range knownTypeOrder {
		if !shown[k.typ] {
			out = append(out, k.typ)
		}
	}
	return out
}

// TagQuery — строка поиска по тегу: `тип:"имя"` для известных типов,
// `tag:"имя"` (тег любого типа) — для остальных.
func TagQuery(tagType, name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	for _, k := range knownTypeOrder {
		if k.typ == tagType {
			return tagType + `:"` + name + `"`
		}
	}
	return `tag:"` + name + `"`
}

// Row — строка сведений «подпись — значение».
type Row struct {
	Label string
	Value string
}

// InfoRows возвращает сведения о произведении. Поле с пустым значением
// (пустая или пробельная строка, 0, нулевая дата) не возвращается — без исключений.
func InfoRows(g model.Gallery) []Row {
	var rows []Row
	add := func(label, value string) {
		if strings.TrimSpace(value) != "" {
			rows = append(rows, Row{label, value})
		}
	}
	if n := len(g.Pages); n > 0 {
		v := strconv.Itoa(n)
		if g.NumPages > 0 && g.NumPages != n {
			v = i18n.T("info.pages_meta", "Pages", n, "Meta", g.NumPages)
		}
		add(i18n.T("info.pages"), v)
	}
	if g.ExternalID > 0 {
		add(i18n.T("info.id"), strconv.FormatInt(g.ExternalID, 10))
	}
	if !g.Uploaded.IsZero() {
		add(i18n.T("info.uploaded"), FormatDate(g.Uploaded))
	}
	if g.Favorites > 0 {
		add(i18n.T("info.favorites"), FormatInt(int64(g.Favorites)))
	}
	add(i18n.T("info.scanlator"), strings.TrimSpace(g.Scanlator))
	add(i18n.T("info.file"), g.Key.ID)
	if g.File.Size > 0 {
		add(i18n.T("info.size"), FormatSize(g.File.Size))
	}
	if !g.File.ModTime.IsZero() {
		add(i18n.T("info.modified"), FormatDateTime(g.File.ModTime))
	}
	return rows
}

// FormatDate — дата в UTC по формату языка интерфейса (см. i18n.Date).
func FormatDate(t time.Time) string { return i18n.Date(t) }

// FormatDateTime — местные дата и время по формату языка интерфейса.
func FormatDateTime(t time.Time) string { return i18n.DateTime(t) }

// FormatSize — размер в Б/КБ/МБ/ГБ (основание 1024) с одной цифрой после
// десятичного разделителя языка интерфейса.
func FormatSize(n int64) string { return i18n.Size(n) }

// FormatInt — число с разделителем разрядов языка интерфейса: 12 345.
func FormatInt(n int64) string { return i18n.Int(n) }
