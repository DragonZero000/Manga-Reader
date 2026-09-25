package model

import (
	"reflect"
	"sort"
	"testing"
)

func TestNewTagNormalizes(t *testing.T) {
	tag := NewTag("Artist", "  Artist   1 ")
	if tag.Type != "artist" || tag.Name != "artist 1" {
		t.Fatalf("got %+v", tag)
	}
	if got := tag.String(); got != "artist:artist 1" {
		t.Fatalf("String() = %q", got)
	}
	if !tag.Known() {
		t.Fatal("artist должен быть известным типом")
	}
}

func TestUnknownTagTypeKept(t *testing.T) {
	tag := NewTag("Circle", "x")
	if tag.Type != "circle" || tag.Known() {
		t.Fatalf("got %+v known=%v", tag, tag.Known())
	}
}

func TestParseTag(t *testing.T) {
	if got := ParseTag(`Artist:Artist 1`); got != NewTag("artist", "artist 1") {
		t.Fatalf("got %+v", got)
	}
	if got := ParseTag("tag 1"); got != NewTag("tag", "tag 1") {
		t.Fatalf("без типа: got %+v", got)
	}
}

func TestAddTagDeduplicates(t *testing.T) {
	var g Gallery
	g.AddTag(Tag{Type: "Tag", Name: "Tag 1"})
	g.AddTag(Tag{Type: "tag", Name: "tag 1"})
	g.AddTag(Tag{Type: "tag", Name: "   "})
	if len(g.Tags) != 1 || g.Tags[0].String() != "tag:tag 1" {
		t.Fatalf("got %+v", g.Tags)
	}
	if !g.HasTag(Tag{Type: "TAG", Name: "Tag 1"}) {
		t.Fatal("HasTag должен учитывать нормализацию")
	}
}

func TestKey(t *testing.T) {
	k := LocalKey("example.zip")
	if k.String() != "local:example.zip" {
		t.Fatalf("String() = %q", k.String())
	}
	parsed, err := ParseKey("local:example.zip")
	if err != nil || parsed != k {
		t.Fatalf("ParseKey = %+v, %v", parsed, err)
	}
	// идентификатор может содержать ':' — разделяется только первый
	if p, err := ParseKey("local:a:b.zip"); err != nil || p.ID != "a:b.zip" {
		t.Fatalf("ParseKey с ':' в ID = %+v, %v", p, err)
	}
	for _, bad := range []string{"example.zip", ":example.zip", "local:"} {
		if _, err := ParseKey(bad); err == nil {
			t.Errorf("ParseKey(%q): ожидалась ошибка", bad)
		}
	}
}

func TestNaturalLess(t *testing.T) {
	got := []string{"10.jpg", "2.jpg", "1.jpg", "B.jpg", "a.jpg", "page10", "Page2", "02.jpg", "cover"}
	sort.SliceStable(got, func(i, j int) bool { return NaturalLess(got[i], got[j]) })
	want := []string{"1.jpg", "2.jpg", "02.jpg", "10.jpg", "a.jpg", "B.jpg", "cover", "Page2", "page10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
	if !NaturalLess("dir/9.jpg", "dir/10.jpg") {
		t.Fatal("путь с каталогом")
	}
	if NaturalLess("a", "a") {
		t.Fatal("строка не может быть меньше себя")
	}
	if !NaturalLess("99999999999999999999999.jpg", "100000000000000000000000.jpg") {
		t.Fatal("длинные числа без переполнения")
	}
}

func TestSortPagesAndCover(t *testing.T) {
	g := Gallery{Pages: []Page{{"10.jpg"}, {"2.jpg"}, {"1.jpg"}}}
	g.SortPages()
	want := []Page{{"1.jpg"}, {"2.jpg"}, {"10.jpg"}}
	if !reflect.DeepEqual(g.Pages, want) {
		t.Fatalf("got %v", g.Pages)
	}
	if c, ok := g.Cover(); !ok || c.Name != "1.jpg" {
		t.Fatalf("Cover = %v, %v", c, ok)
	}
}

func TestCoverEmpty(t *testing.T) {
	var g Gallery
	if _, ok := g.Cover(); ok {
		t.Fatal("у пустой галереи нет обложки")
	}
}

func TestChooseTitle(t *testing.T) {
	cases := []struct {
		en, jp, file   string
		wantT, wantAlt string
	}{
		{"english name", "japanease name", "x.zip", "english name", "japanease name"},
		{"", "japanease name", "x.zip", "japanease name", ""},
		{"  ", "", "example.zip", "example", ""},
		{"", "", `sub\dir\example.zip`, "example", ""},
		{"english name", "", "x.zip", "english name", ""},
	}
	for _, c := range cases {
		gotT, gotAlt := ChooseTitle(c.en, c.jp, c.file)
		if gotT != c.wantT || gotAlt != c.wantAlt {
			t.Errorf("ChooseTitle(%q,%q,%q) = %q,%q; want %q,%q",
				c.en, c.jp, c.file, gotT, gotAlt, c.wantT, c.wantAlt)
		}
	}
}

func TestZeroGallery(t *testing.T) {
	g := Gallery{Key: LocalKey("a.zip"), Title: "a"}
	if g.ExternalID != 0 || len(g.Tags) != 0 || !g.Uploaded.IsZero() {
		t.Fatalf("необязательные поля должны быть нулевыми: %+v", g)
	}
}
