package details

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mangareader/internal/model"
)

// knownTypeOrder — порядок групп тегов и их подписи.
var knownTypeOrder = []struct{ typ, label string }{
	{model.TagTypeArtist, "Автор"},
	{model.TagTypeGroup, "Группа"},
	{model.TagTypeParody, "Пародия"},
	{model.TagTypeCharacter, "Персонаж"},
	{model.TagTypeLanguage, "Язык"},
	{model.TagTypeCategory, "Категория"},
	{model.TagTypeTag, "Теги"},
}

// Group — теги одного типа.
type Group struct {
	Type  string
	Label string
	Names []string
}

// TagGroups группирует теги по типам: сначала известные типы в фиксированном
// порядке, затем неизвестные по алфавиту. Внутри группы — порядок тегов
// галереи. Пустые группы не возвращаются.
func TagGroups(tags []model.Tag) []Group {
	byType := map[string][]string{}
	for _, t := range tags {
		if strings.TrimSpace(t.Name) == "" {
			continue
		}
		byType[t.Type] = append(byType[t.Type], t.Name)
	}
	var out []Group
	known := map[string]bool{}
	for _, k := range knownTypeOrder {
		known[k.typ] = true
		if names := byType[k.typ]; len(names) > 0 {
			out = append(out, Group{Type: k.typ, Label: k.label, Names: names})
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
		label := typ
		if label == "" {
			label = "Прочее"
		}
		out = append(out, Group{Type: typ, Label: label, Names: byType[typ]})
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
			v += fmt.Sprintf(" (в метаданных: %d)", g.NumPages)
		}
		add("Страниц", v)
	}
	if g.ExternalID > 0 {
		add("ID", strconv.FormatInt(g.ExternalID, 10))
	}
	if !g.Uploaded.IsZero() {
		add("Загружено", FormatDate(g.Uploaded))
	}
	if g.Favorites > 0 {
		add("Избранное", FormatInt(int64(g.Favorites)))
	}
	add("Сканлейтор", strings.TrimSpace(g.Scanlator))
	add("Файл", g.Key.ID)
	if g.File.Size > 0 {
		add("Размер", FormatSize(g.File.Size))
	}
	if !g.File.ModTime.IsZero() {
		add("Изменён", FormatDateTime(g.File.ModTime))
	}
	return rows
}

// FormatDate — дата в UTC в формате ДД.ММ.ГГГГ (дата загрузки хранится в UTC
// и не должна «съезжать» на соседний день из-за часового пояса).
func FormatDate(t time.Time) string {
	return t.UTC().Format("02.01.2006")
}

// FormatDateTime — местные дата и время ДД.ММ.ГГГГ ЧЧ:ММ.
func FormatDateTime(t time.Time) string {
	return t.Local().Format("02.01.2006 15:04")
}

// FormatSize — размер в Б/КБ/МБ/ГБ (основание 1024) с одной цифрой после запятой.
func FormatSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d Б", n)
	}
	units := []string{"КБ", "МБ", "ГБ", "ТБ"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", v, units[i]), ".", ",", 1)
}

// FormatInt — число с неразрывным пробелом между разрядами: 12 345.
func FormatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
