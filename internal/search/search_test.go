package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"mangareader/internal/model"
)

func TestFieldsDescribed(t *testing.T) {
	all := Fields()
	if len(all) != 9 {
		t.Fatalf("ожидалось 9 полей, получено %d", len(all))
	}
	seen := map[string]bool{}
	for _, fi := range all {
		if fi.Name == "" || fi.Label == "" || fi.Kind == 0 || len(fi.Ops) == 0 {
			t.Errorf("неполное описание поля: %+v", fi)
		}
		if seen[fi.Name] {
			t.Errorf("дубликат имени %q", fi.Name)
		}
		seen[fi.Name] = true
		if f, ok := FieldByName(fi.Name); !ok || f != fi.Field {
			t.Errorf("FieldByName(%q) не находит поле", fi.Name)
		}
	}
	if !DefaultSort.Field.mustInfo(t).Sortable {
		t.Fatal("поле сортировки по умолчанию должно быть сортируемым")
	}
}

func (f Field) mustInfo(t *testing.T) FieldInfo {
	t.Helper()
	fi, ok := f.Info()
	if !ok {
		t.Fatalf("нет описания поля %d", f)
	}
	return fi
}

func TestEmptyQueryValid(t *testing.T) {
	q := NewQuery()
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	if q.Sort != (Sort{Field: FieldAdded, Desc: true}) {
		t.Fatalf("сортировка по умолчанию: %+v", q.Sort)
	}
}

func TestCombinedQueryValid(t *testing.T) {
	q := NewQuery()
	q.Filters = []Filter{
		{FieldTag, OpHas, Value{Tag: model.Tag{Type: "Artist", Name: `Artist 1`}}},
		{FieldTag, OpNotHas, Value{Tag: model.Tag{Type: "tag", Name: "tag 3"}}},
		{FieldPages, OpGt, Value{Num: 20}},
	}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(q.Filters) != 3 {
		t.Fatalf("фильтров: %d", len(q.Filters))
	}
	if got := q.Filters[0].Value.Tag.String(); got != "artist:artist 1" {
		t.Fatalf("тег не нормализован: %q", got)
	}
}

func TestValidateErrors(t *testing.T) {
	day := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		filter Filter
		want   string
	}{
		{"операция для title", Filter{FieldTitle, OpGt, Value{Num: 5}}, "больше"},
		{"диапазон страниц", Filter{FieldPages, OpBetween, Value{Num: 50, Num2: 10}}, "диапазон"},
		{"пустой тег", Filter{FieldTag, OpHas, Value{}}, "пустой тег"},
		{"пустой текст", Filter{FieldScanlator, OpEq, Value{Text: " "}}, "пустой текст"},
		{"отрицательное", Filter{FieldSize, OpLt, Value{Num: -1}}, "отрицательное"},
		{"нет даты", Filter{FieldUploaded, OpGt, Value{}}, "дата"},
		{"диапазон дат", Filter{FieldAdded, OpBetween, Value{Time: day.AddDate(0, 1, 0), Time2: day}}, "диапазон дат"},
		{"неизвестное поле", Filter{Field(99), OpEq, Value{}}, "неизвестное поле"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := NewQuery()
			q.Filters = []Filter{{FieldPages, OpGt, Value{Num: 1}}, c.filter}
			err := q.Validate()
			var fe *FilterError
			if !errors.As(err, &fe) {
				t.Fatalf("ожидалась FilterError, получено %v", err)
			}
			if fe.Index != 1 {
				t.Errorf("индекс фильтра = %d, ожидался 1", fe.Index)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("ошибка %q не содержит %q", err, c.want)
			}
		})
	}
}

func TestTitleErrorMentionsField(t *testing.T) {
	q := NewQuery()
	q.Filters = []Filter{{FieldTitle, OpGt, Value{Num: 5}}}
	err := q.Validate()
	if err == nil || !strings.Contains(err.Error(), "title") || !strings.Contains(err.Error(), "больше") {
		t.Fatalf("ошибка должна указывать поле title и операцию «больше»: %v", err)
	}
}

func TestTagWithoutType(t *testing.T) {
	q := NewQuery()
	q.Filters = []Filter{{FieldTag, OpHas, Value{Tag: model.Tag{Name: "Tag 1"}}}}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := q.Filters[0].Value.Tag; got.Type != "" || got.Name != "tag 1" {
		t.Fatalf("тег без типа: %+v", got)
	}
}

func TestValidatePaging(t *testing.T) {
	for _, q := range []Query{{Limit: -1}, {Offset: -1}} {
		if err := q.Validate(); err == nil {
			t.Errorf("ожидалась ошибка для %+v", q)
		}
	}
	q := Query{Sort: Sort{Field: FieldTag}}
	if err := q.Validate(); err == nil {
		t.Error("сортировка по тегу недоступна")
	}
}

func TestNopIndex(t *testing.T) {
	var idx Index = NopIndex{}
	ctx := context.Background()
	if err := idx.Upsert(ctx, model.Gallery{Key: model.LocalKey("a.zip")}); err != nil {
		t.Fatal(err)
	}
	keys, total, err := idx.Search(ctx, NewQuery())
	if err != nil || total != 0 || len(keys) != 0 {
		t.Fatalf("Search = %v, %d, %v", keys, total, err)
	}
	tags, err := idx.SuggestTags(ctx, "", "a", 10)
	if err != nil || len(tags) != 0 {
		t.Fatalf("SuggestTags = %v, %v", tags, err)
	}
}
