package library

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"mangareader/internal/model"
)

func TestReadExampleArchive(t *testing.T) {
	dir := t.TempDir()
	p := copyExample(t, dir, "example.zip")

	g, warnings, err := readArchivePath(t, p, "example.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("предупреждения: %v", warnings)
	}
	if g.Key.String() != "local:example.zip" {
		t.Errorf("ключ = %s", g.Key)
	}
	if g.Title != "english name" || g.AltTitle != "japanease name" {
		t.Errorf("название = %q / %q", g.Title, g.AltTitle)
	}
	if g.ExternalID != 535147 {
		t.Errorf("ID = %d", g.ExternalID)
	}
	if len(g.Pages) != 2 || g.Pages[0].Name != "example/1.jpg" || g.Pages[1].Name != "example/2.jpg" {
		t.Errorf("страницы = %v", g.Pages)
	}
	if c, ok := g.Cover(); !ok || c.Name != "example/1.jpg" {
		t.Errorf("обложка = %v", c)
	}
	if len(g.Tags) != 8 {
		t.Errorf("тегов %d: %v", len(g.Tags), g.Tags)
	}
	for _, want := range []string{"artist:artist 1", "language:japanese", "category:doujinshi"} {
		if !g.HasTag(model.ParseTag(want)) {
			t.Errorf("нет тега %s", want)
		}
	}
	if y, m, d := g.Uploaded.Date(); y != 2024 || m != time.October || d != 15 || g.Uploaded.Location() != time.UTC {
		t.Errorf("дата загрузки = %v", g.Uploaded)
	}
	if g.NumPages != 2 || g.Favorites != 806 {
		t.Errorf("NumPages=%d Favorites=%d", g.NumPages, g.Favorites)
	}
	if g.File.Path != p || g.File.Size == 0 || g.File.ModTime.IsZero() {
		t.Errorf("FileInfo = %+v", g.File)
	}
}

func TestReadArchiveManyPages(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 4, 4)
	var files []file
	for _, n := range []int{10, 2, 12, 1, 11, 3, 4, 5, 6, 7, 8, 9} {
		files = append(files, file{fmt.Sprintf("%d.jpg", n), img})
	}
	p := writeZip(t, dir, "many.zip", files...)

	g, _, err := readArchivePath(t, p, "many.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, pg := range g.Pages {
		names = append(names, pg.Name)
	}
	want := "1.jpg 2.jpg 3.jpg 4.jpg 5.jpg 6.jpg 7.jpg 8.jpg 9.jpg 10.jpg 11.jpg 12.jpg"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("порядок:\n got %s\nwant %s", got, want)
	}
}

func TestReadArchiveWithoutMeta(t *testing.T) {
	dir := t.TempDir()
	p := writeZip(t, dir, "my manga.zip", file{"a.png", pngBytes(t, 2, 2)})

	g, warnings, err := readArchivePath(t, p, "my manga.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if g.Title != "my manga" || g.ExternalID != 0 || len(g.Tags) != 0 || len(warnings) != 0 {
		t.Fatalf("got %+v, warnings %v", g, warnings)
	}
}

func TestReadArchiveBrokenMeta(t *testing.T) {
	dir := t.TempDir()
	p := writeZip(t, dir, "bad.zip",
		file{"meta.json", []byte("{not json")},
		file{"1.jpg", pngBytes(t, 2, 2)},
	)
	g, warnings, err := readArchivePath(t, p, "bad.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if g.Title != "bad" || len(warnings) != 1 || !strings.Contains(warnings[0], "meta.json") {
		t.Fatalf("title=%q warnings=%v", g.Title, warnings)
	}
}

func TestReadArchivePageCountMismatch(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	p := writeZip(t, dir, "m.zip",
		file{"g/meta.json", []byte(`{"id":"42","title":{"english":"E"},"num_pages":5}`)},
		file{"g/1.jpg", img}, file{"g/2.jpg", img}, file{"g/3.jpg", img},
	)
	g, _, err := readArchivePath(t, p, "m.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Pages) != 3 || g.NumPages != 5 {
		t.Fatalf("страниц %d, NumPages %d", len(g.Pages), g.NumPages)
	}
	if g.ExternalID != 42 {
		t.Fatalf("id строкой: %d", g.ExternalID)
	}
}

func TestReadArchiveShallowestMetaAndServiceFiles(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	p := writeZip(t, dir, "x.zip",
		file{"a/b/meta.json", []byte(`{"title":{"english":"deep"}}`)},
		file{"a/meta.json", []byte(`{"title":{"english":"shallow"}}`)},
		file{"__MACOSX/a/._1.jpg", []byte("junk")},
		file{"a/.hidden.jpg", img},
		file{"a/1.JPG", img},
		file{"a/notes.txt", []byte("x")},
	)
	g, _, err := readArchivePath(t, p, "x.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if g.Title != "shallow" {
		t.Errorf("использован не самый верхний meta.json: %q", g.Title)
	}
	if len(g.Pages) != 1 || g.Pages[0].Name != "a/1.JPG" {
		t.Errorf("страницы = %v", g.Pages)
	}
}

func TestReadArchiveErrors(t *testing.T) {
	dir := t.TempDir()
	notZip := writeFile(t, dir, "broken.zip", []byte("это не zip"))
	if _, _, err := readArchivePath(t, notZip, "broken.zip", stat(t, notZip)); !errors.Is(err, ErrNotZip) {
		t.Errorf("не zip: %v", err)
	}
	onlyMeta := writeZip(t, dir, "meta-only.zip", file{"meta.json", []byte(`{}`)})
	if _, _, err := readArchivePath(t, onlyMeta, "meta-only.zip", stat(t, onlyMeta)); !errors.Is(err, ErrNoImages) {
		t.Errorf("без изображений: %v", err)
	}
}

func TestFlexInt(t *testing.T) {
	for in, want := range map[string]int64{`1`: 1, `"2"`: 2, `null`: 0, `""`: 0, `3.0`: 3} {
		var f flexInt
		if err := f.UnmarshalJSON([]byte(in)); err != nil || int64(f) != want {
			t.Errorf("%s → %d, %v", in, f, err)
		}
	}
	var f flexInt
	if err := f.UnmarshalJSON([]byte(`"abc"`)); err == nil {
		t.Error("ожидалась ошибка")
	}
}
