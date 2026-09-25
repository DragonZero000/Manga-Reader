package search

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"mangareader/internal/model"
)

func mustParse(t *testing.T, s string) Query {
	t.Helper()
	q, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return q
}

func parseErr(t *testing.T, s string) *ParseError {
	t.Helper()
	_, err := Parse(s)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("Parse(%q): ожидалась ParseError, получено %v", s, err)
	}
	return pe
}

func TestParseWords(t *testing.T) {
	cases := map[string][]string{
		"english  Name":      {"english", "name"},
		`"english name" tag`: {"english name", "tag"},
		"re:zero":            {"re:zero"},
		`""  x`:              {"x"},
		`"Re:Zero kara"`:     {"re:zero kara"},
		"  ":                 nil,
		`title_like:Школа`:   {"title_like:школа"},
		`Ёлка`:               {"ёлка"},
		`"незакрытая фраза`:  {"незакрытая фраза"},
	}
	for s, want := range cases {
		q := mustParse(t, s)
		if !reflect.DeepEqual(q.Terms, want) {
			t.Errorf("Parse(%q).Terms = %q, want %q", s, q.Terms, want)
		}
		if len(q.Filters) != 0 {
			t.Errorf("Parse(%q): лишние фильтры %v", s, q.Filters)
		}
	}
}

func TestParseTags(t *testing.T) {
	q := mustParse(t, `tag:"tag 1" Artist:"Artist 1" -tag:"tag 3" language:japanese`)
	want := []Filter{
		{FieldTag, OpHas, Value{Tag: model.Tag{Name: "tag 1"}}},
		{FieldTag, OpHas, Value{Tag: model.NewTag("artist", "artist 1")}},
		{FieldTag, OpNotHas, Value{Tag: model.Tag{Name: "tag 3"}}},
		{FieldTag, OpHas, Value{Tag: model.NewTag("language", "japanese")}},
	}
	if !reflect.DeepEqual(q.Filters, want) {
		t.Fatalf("фильтры:\n got %+v\nwant %+v", q.Filters, want)
	}
	// незакрытая кавычка закрывается в конце строки
	q = mustParse(t, `tag:"tag 1`)
	if len(q.Filters) != 1 || q.Filters[0].Value.Tag.Name != "tag 1" {
		t.Fatalf("незакрытая кавычка: %+v", q.Filters)
	}
}

func TestParseCombined(t *testing.T) {
	q := mustParse(t, "school pages:>20 language:japanese")
	if !reflect.DeepEqual(q.Terms, []string{"school"}) || len(q.Filters) != 2 {
		t.Fatalf("got terms=%q filters=%+v", q.Terms, q.Filters)
	}
	if q.Filters[0] != (Filter{FieldPages, OpGt, Value{Num: 20}}) {
		t.Errorf("pages: %+v", q.Filters[0])
	}
	if q.Text != "school pages:>20 language:japanese" {
		t.Errorf("Text = %q", q.Text)
	}
}

func TestParseNumbers(t *testing.T) {
	cases := map[string]Filter{
		"pages:>20":          {FieldPages, OpGt, Value{Num: 20}},
		"pages:<5":           {FieldPages, OpLt, Value{Num: 5}},
		"pages:>=20":         {FieldPages, OpGt, Value{Num: 19}},
		"pages:<=20":         {FieldPages, OpLt, Value{Num: 21}},
		"pages:20":           {FieldPages, OpEq, Value{Num: 20}},
		"PAGES:10..50":       {FieldPages, OpBetween, Value{Num: 10, Num2: 50}},
		"favorites:100..500": {FieldFavorites, OpBetween, Value{Num: 100, Num2: 500}},
		"favorites:806":      {FieldFavorites, OpBetween, Value{Num: 806, Num2: 806}},
		"id:535147":          {FieldID, OpEq, Value{Num: 535147}},
		"size:>1,5mb":        {FieldSize, OpGt, Value{Num: 1572864}},
		"size:<10MB":         {FieldSize, OpLt, Value{Num: 10 << 20}},
		"size:500k":          {FieldSize, OpBetween, Value{Num: 500 << 10, Num2: 500 << 10}},
		"size:>2.5g":         {FieldSize, OpGt, Value{Num: 2684354560}},
		"size:>100":          {FieldSize, OpGt, Value{Num: 100}},
	}
	for s, want := range cases {
		q := mustParse(t, s)
		if len(q.Filters) != 1 || q.Filters[0] != want {
			t.Errorf("Parse(%q) = %+v, want %+v", s, q.Filters, want)
		}
	}
}

func TestParseDates(t *testing.T) {
	utc := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
	ns := time.Nanosecond
	cases := map[string]Filter{
		"uploaded:2024":             {FieldUploaded, OpBetween, Value{Time: utc(2024, 1, 1), Time2: utc(2025, 1, 1).Add(-ns)}},
		"uploaded:>2024-06":         {FieldUploaded, OpGt, Value{Time: utc(2024, 7, 1).Add(-ns)}},
		"uploaded:<2024-06-15":      {FieldUploaded, OpLt, Value{Time: utc(2024, 6, 15)}},
		"uploaded:>=2024":           {FieldUploaded, OpGt, Value{Time: utc(2024, 1, 1).Add(-ns)}},
		"uploaded:<=2024-02":        {FieldUploaded, OpLt, Value{Time: utc(2024, 3, 1)}},
		"uploaded:2024-01..2024-03": {FieldUploaded, OpBetween, Value{Time: utc(2024, 1, 1), Time2: utc(2024, 4, 1).Add(-ns)}},
	}
	for s, want := range cases {
		q := mustParse(t, s)
		if len(q.Filters) != 1 || !reflect.DeepEqual(q.Filters[0], want) {
			t.Errorf("Parse(%q) = %+v, want %+v", s, q.Filters, want)
		}
	}
	q := mustParse(t, "added:2026-09")
	if f := q.Filters[0]; f.Value.Time.Location() != time.Local || f.Value.Time.Day() != 1 {
		t.Errorf("added в местном времени: %+v", f.Value.Time)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"-school":             "только для тегов",
		"-pages:>2":           "только для тегов",
		"pages:>много":        "число",
		"pages:":              "не указано значение",
		`tag:""`:              "не указано значение",
		"uploaded:2024-13":    "дата",
		"uploaded:24":         "дата",
		"pages:50..10":        "диапазон",
		"id:>5":               "id",
		"size:>lots":          "размер",
		"uploaded:2025..2024": "диапазон дат",
	}
	for s, want := range cases {
		pe := parseErr(t, s)
		if pe.Token != s || !strings.Contains(pe.Error(), want) {
			t.Errorf("Parse(%q): ошибка %q (фрагмент %q), ожидалось «%s»", s, pe.Error(), pe.Token, want)
		}
	}
	if pe := parseErr(t, "school pages:>много tag:x"); pe.Token != "pages:>много" {
		t.Errorf("фрагмент ошибки: %q", pe.Token)
	}
	if got := parseErr(t, "pages:").Error(); got != "«pages:» — не указано значение" {
		t.Errorf("текст ошибки: %q", got)
	}
}
