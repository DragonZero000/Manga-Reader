package library

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"mangareader/internal/model"
)

func readWith(t *testing.T, files ...file) (model.Gallery, []string) {
	t.Helper()
	dir := t.TempDir()
	files = append([]file{{"1.png", pngBytes(t, 2, 2)}}, files...)
	p := writeZip(t, dir, "a.zip", files...)
	g, w, err := readArchivePath(t, p, "a.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	return g, w
}

func ci(body string) file {
	return file{"ComicInfo.xml", []byte(`<?xml version="1.0" encoding="utf-8"?><ComicInfo>` + body + `</ComicInfo>`)}
}

func tagStrings(g model.Gallery) []string {
	var out []string
	for _, tg := range g.Tags {
		out = append(out, tg.String())
	}
	sort.Strings(out)
	return out
}

func TestComicInfoFields(t *testing.T) {
	g, w := readWith(t, ci(`<Title> X </Title><AlternateSeries>Y</AlternateSeries>
<Tags>horror, , supernatural ,mystery</Tags><Characters>a, b</Characters><LanguageISO>ja</LanguageISO>
<Format>Manga</Format><Writer>W1, W2</Writer><Penciller>P</Penciller><Publisher>Pub</Publisher>
<Year>2024</Year><Month>7</Month><Day>3</Day><PageCount>12</PageCount><Web>https://s.example/g/1</Web>
<AgeRating>18+</AgeRating><Manga>YesAndRightToLeft</Manga>`))
	if len(w) != 0 || !g.HasMeta || g.Title != "X" || g.AltTitle != "Y" || g.NumPages != 12 || g.SourceURL != "https://s.example/g/1" {
		t.Fatalf("%+v %v", g, w)
	}
	if !g.Uploaded.Equal(time.Date(2024, 7, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("дата %v", g.Uploaded)
	}
	want := []string{"artist:p", "artist:w1", "artist:w2", "category:manga", "character:a", "character:b",
		"group:pub", "language:japanese", "tag:horror", "tag:mystery", "tag:supernatural"}
	if got := tagStrings(g); !reflect.DeepEqual(got, want) {
		t.Errorf("теги %v", got)
	}
}

func TestComicInfoTitleFromSeries(t *testing.T) {
	g, _ := readWith(t, ci(`<Series>S</Series><Number>3</Number>`))
	if g.Title != "S 3" {
		t.Errorf("название %q", g.Title)
	}
}

func TestComicInfoLinkAndDate(t *testing.T) {
	for web, want := range map[string]string{
		"https://examplesite.com/": "", "https://examplesite.com": "", "ftp://a.example/x": "",
		"https://examplesite.com/g/123": "https://examplesite.com/g/123",
	} {
		if g, _ := readWith(t, ci(`<Web>`+web+`</Web>`)); g.SourceURL != want {
			t.Errorf("%s: %q", web, g.SourceURL)
		}
	}
	if g, _ := readWith(t, ci(`<Year>2020</Year>`)); !g.Uploaded.Equal(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("год без месяца: %v", g.Uploaded)
	}
	if g, _ := readWith(t, ci(`<Month>5</Month>`)); !g.Uploaded.IsZero() {
		t.Errorf("без года: %v", g.Uploaded)
	}
}

func TestComicInfoOnlyAndBOM(t *testing.T) {
	data := append([]byte{0xEF, 0xBB, 0xBF}, ciData(`<Title>X</Title>`)...)
	g, w := readWith(t, file{"sub/comicinfo.XML", data})
	if len(w) != 0 || g.Title != "X" || !g.HasMeta {
		t.Fatalf("%+v %v", g, w)
	}
}

func TestComicInfoShallowest(t *testing.T) {
	g, _ := readWith(t, file{"a/b/ComicInfo.xml", ciData(`<Title>deep</Title>`)}, file{"ComicInfo.xml", ciData(`<Title>top</Title>`)})
	if g.Title != "top" {
		t.Errorf("название %q", g.Title)
	}
}

func TestComicInfoBroken(t *testing.T) {
	g, w := readWith(t, file{"ComicInfo.xml", []byte("<ComicInfo><Title>x")})
	if len(w) != 1 || !strings.Contains(w[0], "ComicInfo.xml") || g.HasMeta || g.Title != "a" {
		t.Fatalf("%+v %v", g, w)
	}
}

func TestComicInfoMetaPriority(t *testing.T) {
	meta := file{"meta.json", []byte(`{"title":{"english":"A"},"tags":[{"type":"tag","name":"horror"}]}`)}
	g, _ := readWith(t, meta, ci(`<Title>B</Title><AlternateSeries>J</AlternateSeries><Tags>Horror, mystery</Tags><Web>https://site.example/g/1</Web>`))
	if g.Title != "A" || g.AltTitle != "J" || g.SourceURL != "https://site.example/g/1" {
		t.Errorf("%q %q %q", g.Title, g.AltTitle, g.SourceURL)
	}
	if got := tagStrings(g); !reflect.DeepEqual(got, []string{"tag:horror", "tag:mystery"}) {
		t.Errorf("теги %v", got)
	}
}

func TestComicInfoExampleCbz(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cbz-example.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFile(t, dir, "x.cbz", data)
	res := scanDir(t, dir)
	if len(res.Galleries) != 1 {
		t.Fatalf("%v", res.Errors)
	}
	g := res.Galleries[0]
	if g.Title != "The Hollow Shrine" || g.AltTitle != "うつろの社" || len(g.Pages) != 5 || g.NumPages != 5 || !g.HasMeta {
		t.Errorf("%+v", g)
	}
	want := []string{"artist:s. watanuki", "category:manga", "character:shizuka", "language:english",
		"tag:horror", "tag:mystery", "tag:supernatural"}
	if got := tagStrings(g); !reflect.DeepEqual(got, want) {
		t.Errorf("теги %v", got)
	}
}

func ciData(body string) []byte { return ci(body).data }
