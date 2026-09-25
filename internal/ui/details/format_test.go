package details

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"mangareader/internal/library"
	"mangareader/internal/model"
)

// exampleGallery читает testdata/example.zip через библиотеку.
func exampleGallery(t *testing.T) model.Gallery {
	t.Helper()
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	src := library.NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := src.Get(model.LocalKey("example.zip"))
	if !ok {
		t.Fatal("нет галереи")
	}
	return g
}

func TestTagGroupsExample(t *testing.T) {
	g := exampleGallery(t)
	var got []string
	for _, gr := range TagGroups(g.Tags) {
		got = append(got, gr.Label+": "+strings.Join(gr.Names, ", "))
	}
	want := []string{
		"Автор: artist 1",
		"Пародия: parody 1",
		"Персонаж: character 1",
		"Язык: japanese",
		"Категория: doujinshi",
		"Теги: tag 1, tag 2, tag 3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("группы:\n got %q\nwant %q", got, want)
	}
}

func TestTagGroupsUnknownAndEmpty(t *testing.T) {
	tags := []model.Tag{
		model.NewTag("zeta", "z1"),
		model.NewTag("circle", "c1"),
		model.NewTag("tag", "t1"),
		model.NewTag("circle", "c2"),
		model.NewTag("artist", "   "), // пустое имя — группа не появляется
	}
	got := TagGroups(tags)
	if len(got) != 3 || got[0].Label != "Теги" || got[1].Label != "circle" || got[2].Label != "zeta" {
		t.Fatalf("группы: %+v", got)
	}
	if !reflect.DeepEqual(got[1].Names, []string{"c1", "c2"}) {
		t.Errorf("порядок внутри группы: %v", got[1].Names)
	}
	if len(TagGroups(nil)) != 0 {
		t.Error("без тегов групп быть не должно")
	}
}

func rowsMap(rows []Row) map[string]string {
	m := map[string]string{}
	for _, r := range rows {
		m[r.Label] = r.Value
	}
	return m
}

func TestInfoRowsExample(t *testing.T) {
	g := exampleGallery(t)
	rows := InfoRows(g)
	m := rowsMap(rows)
	want := map[string]string{
		"Страниц":   "2",
		"ID":        "535147",
		"Загружено": "15.10.2024",
		"Избранное": "806",
		"Файл":      "example.zip",
		"Размер":    "31,0 КБ",
	}
	for k, v := range want {
		if m[k] != v {
			t.Errorf("%s = %q, want %q", k, m[k], v)
		}
	}
	if _, ok := m["Сканлейтор"]; ok {
		t.Error("пустой сканлейтор показан")
	}
	if m["Изменён"] == "" {
		t.Error("нет даты изменения файла")
	}
	// порядок строк
	var labels []string
	for _, r := range rows {
		labels = append(labels, r.Label)
	}
	if got := strings.Join(labels, ","); got != "Страниц,ID,Загружено,Избранное,Файл,Размер,Изменён" {
		t.Errorf("порядок: %s", got)
	}
}

func TestInfoRowsEmptyValuesHidden(t *testing.T) {
	g := model.Gallery{
		Key:       model.LocalKey("sub/a.zip"),
		Pages:     []model.Page{{Name: "1.jpg"}, {Name: "2.jpg"}, {Name: "3.jpg"}},
		NumPages:  5,
		Scanlator: "   ",
	}
	m := rowsMap(InfoRows(g))
	if m["Страниц"] != "3 (в метаданных: 5)" {
		t.Errorf("Страниц = %q", m["Страниц"])
	}
	if m["Файл"] != "sub/a.zip" {
		t.Errorf("Файл = %q", m["Файл"])
	}
	for _, hidden := range []string{"ID", "Загружено", "Избранное", "Сканлейтор", "Размер", "Изменён"} {
		if _, ok := m[hidden]; ok {
			t.Errorf("пустое поле %q показано", hidden)
		}
	}
	if rows := InfoRows(model.Gallery{}); len(rows) != 0 {
		t.Errorf("у пустой галереи есть сведения: %v", rows)
	}
}

func TestFormatters(t *testing.T) {
	sizes := map[int64]string{0: "0 Б", 1023: "1023 Б", 1024: "1,0 КБ", 31759: "31,0 КБ",
		5 << 20: "5,0 МБ", 1536 << 20: "1,5 ГБ"}
	for n, want := range sizes {
		if got := FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
	}
	nb := " "
	ints := map[int64]string{0: "0", 806: "806", 1000: "1" + nb + "000", 1234567: "1" + nb + "234" + nb + "567", -1234: "-1" + nb + "234"}
	for n, want := range ints {
		if got := FormatInt(n); got != want {
			t.Errorf("FormatInt(%d) = %q, want %q", n, got, want)
		}
	}
	// 23:30 UTC 14 октября — в UTC это 14.10, в любом поясе дата загрузки та же
	up := time.Date(2024, 10, 14, 23, 30, 0, 0, time.UTC)
	if got := FormatDate(up.In(time.FixedZone("MSK", 3*3600))); got != "14.10.2024" {
		t.Errorf("FormatDate = %q", got)
	}
	loc := time.Date(2026, 9, 24, 11, 3, 0, 0, time.Local)
	if got := FormatDateTime(loc); got != "24.09.2026 11:03" {
		t.Errorf("FormatDateTime = %q", got)
	}
}

func TestTagQuery(t *testing.T) {
	cases := map[[2]string]string{
		{"artist", "artist 1"}: `artist:"artist 1"`,
		{"tag", "tag 1"}:       `tag:"tag 1"`,
		{"circle", "c1"}:       `tag:"c1"`,
		{"language", `a"b`}:    `language:"ab"`,
	}
	for in, want := range cases {
		if got := TagQuery(in[0], in[1]); got != want {
			t.Errorf("TagQuery(%q) = %q, want %q", in, got, want)
		}
	}
}
