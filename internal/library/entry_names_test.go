package library

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// sjis — строка в Shift-JIS: имя записи zip без флага UTF-8, как в архивах,
// собранных на японской Windows.
func sjis(t testing.TB, s string) string {
	t.Helper()
	out, err := japanese.ShiftJIS.NewEncoder().String(s)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.ValidString(out) {
		t.Fatalf("%q в Shift-JIS — корректная UTF-8", s)
	}
	return out
}

// nameCase — архив с необычными именами записей и ожидаемые страницы.
type nameCase struct {
	name  string // имя архива
	files []file
	pages []string // ожидаемые имена страниц по порядку
	title string   // ожидаемое название («» — не проверять)
}

// nameCases — архивы из сценариев spec: «\», служебные файлы с «\»,
// Shift-JIS (в том числе с байтом 0x5C внутри символа), «./», «/» в начале,
// повторяющиеся имена. Картинки разного размера: по размеру видно, какая
// запись открыта.
func nameCases(t testing.TB) []nameCase {
	t.Helper()
	img := func(n int) []byte { return pngBytes(t, n, n+1) }
	sakuhin, manga := sjis(t, "作品"), sjis(t, "漫画")
	hyoushi := sjis(t, "表紙")
	if !strings.Contains(hyoushi, `\`) {
		t.Fatalf("«表紙» в Shift-JIS без байта 0x5C: % x", hyoushi)
	}
	return []nameCase{
		{
			name: "backslash.zip",
			files: []file{
				{`ch1\meta.json`, []byte(`{"title":{"english":"from meta"}}`)},
				{`ch1\001.jpg`, img(2)}, {`ch1\002.jpg`, img(3)},
			},
			pages: []string{`ch1\001.jpg`, `ch1\002.jpg`}, title: "from meta",
		},
		{
			name:  "macosx.zip",
			files: []file{{"001.jpg", img(2)}, {`__MACOSX\._001.jpg`, []byte("junk")}},
			pages: []string{"001.jpg"},
		},
		{
			name:  "sjis.zip",
			files: []file{{sakuhin + "/001.jpg", img(2)}, {manga + "/001.jpg", img(3)}},
			pages: []string{sakuhin + "/001.jpg", manga + "/001.jpg"},
		},
		{
			name:  "sjis-5c.zip",
			files: []file{{"001.jpg", img(2)}, {hyoushi + ".jpg", img(3)}},
			pages: []string{"001.jpg", hyoushi + ".jpg"},
		},
		{
			name:  "dot.zip",
			files: []file{{"./001.jpg", img(2)}, {"./002.jpg", img(3)}},
			pages: []string{"./001.jpg", "./002.jpg"},
		},
		{
			name:  "slash.zip",
			files: []file{{"/001.jpg", img(2)}},
			pages: []string{"/001.jpg"},
		},
		{
			name:  "dup.zip",
			files: []file{{"001.jpg", img(2)}, {"001.jpg", img(4)}, {"002.jpg", img(3)}},
			pages: []string{"001.jpg", "002.jpg"},
		},
	}
}

func pageNames(g model.Gallery) []string {
	var out []string
	for _, p := range g.Pages {
		out = append(out, p.Name)
	}
	return out
}

// firstEntry — данные первой записи с именем name среди файлов архива.
func firstEntry(files []file, name string) []byte {
	for _, f := range files {
		if f.name == name {
			return f.data
		}
	}
	return nil
}

func TestReadArchiveEntryNames(t *testing.T) {
	dir := t.TempDir()
	for _, c := range nameCases(t) {
		t.Run(c.name, func(t *testing.T) {
			p := writeZip(t, dir, c.name, c.files...)
			g, warnings, err := readArchivePath(t, p, c.name, stat(t, p))
			if err != nil {
				t.Fatal(err)
			}
			if got := pageNames(g); !reflect.DeepEqual(got, c.pages) {
				t.Fatalf("страницы %q, ожидались %q", got, c.pages)
			}
			if c.title != "" && g.Title != c.title {
				t.Errorf("название %q, ожидалось %q (meta.json не найден)", g.Title, c.title)
			}
			dupWarned := len(warnings) == 1 && strings.Contains(warnings[0], "duplicate entry")
			if (c.name == "dup.zip") != dupWarned {
				t.Errorf("предупреждения %q", warnings)
			}
		})
	}
}

// Обычный архив: страницы и отпечаток — как до изменения правил.
func TestReadArchiveEntryNamesExampleUnchanged(t *testing.T) {
	p := copyExample(t, t.TempDir(), "example.zip")
	g, _, err := readArchivePath(t, p, "example.zip", stat(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if got := pageNames(g); !reflect.DeepEqual(got, []string{"example/1.jpg", "example/2.jpg"}) {
		t.Errorf("страницы %q", got)
	}
	// отпечаток эталонного архива, вычисленный кодом до изменения правил
	// разбора: записи user.db обычных архивов остаются за ними
	const want = "32503a71a5f32251d18a842d19521276e894345157d1725074f6a56ca759c5cc"
	if g.Fingerprint != want {
		t.Errorf("отпечаток %s, ожидался %s", g.Fingerprint, want)
	}
}

func TestOpenPageAndSizesEntryNames(t *testing.T) {
	dir := t.TempDir()
	cases := nameCases(t)
	for _, c := range cases {
		writeZip(t, dir, c.name, c.files...)
	}
	src := NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			k := model.LocalKey(c.name)
			sizes, err := src.PageSizes(k)
			if err != nil {
				t.Fatal(err)
			}
			for i, page := range c.pages {
				rc, err := src.OpenPage(k, page)
				if err != nil {
					t.Fatalf("%q: %v", page, err)
				}
				var got bytes.Buffer
				got.ReadFrom(rc)
				rc.Close()
				want := firstEntry(c.files, page)
				if !bytes.Equal(got.Bytes(), want) {
					t.Errorf("%q: прочитаны не те байты", page)
				}
				if sizes[i].Err != nil || sizes[i].Width == 0 || sizes[i].Height != sizes[i].Width+1 {
					t.Errorf("%q: размер %+v", page, sizes[i])
				}
			}
		})
	}
	// повтор: открывается первая запись (2×3), а не вторая (4×5)
	sizes, _ := src.PageSizes(model.LocalKey("dup.zip"))
	if sizes[0].Width != 2 {
		t.Errorf("повтор: размер %+v, ожидалась первая запись 2×3", sizes[0])
	}
	if _, err := src.OpenPage(model.LocalKey("slash.zip"), "001.jpg"); err == nil {
		t.Error("страница, которой нет в галерее, открылась")
	}
}

// --- перепроверка после изменения правил разбора ---

// oldRevision — каталог «прежней версии приложения»: ревизия правил 0.
func oldRevision(store *memStore) {
	store.mu.Lock()
	store.rev = 0
	store.mu.Unlock()
}

// Сохранённые результаты ревизии 0: перечитывается только архив со «\»,
// обычный — нет; ревизия сохраняется, и новый сканер ничего не открывает.
func TestRecheckOnlyAffected(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "a.zip", file{`ch1\001.jpg`, img}, file{`ch1\002.jpg`, img})
	writeZip(t, dir, "b.zip", file{"1.jpg", img}, file{"2.jpg", img})
	store := newMemStore()
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rev != ParseRevision {
		t.Fatalf("первое сканирование: ревизия %d", store.rev)
	}
	oldRevision(store)

	opens := countOpens(t)
	res, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 1 || len(res.Changed) != 1 || res.Changed[0].Key.ID != "a.zip" {
		t.Fatalf("открыто %d, изменены %v", opens.Load(), keys(res.Changed))
	}
	if store.rev != ParseRevision {
		t.Errorf("ревизия не сохранена: %d", store.rev)
	}

	opens.Store(0)
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 0 {
		t.Errorf("после сохранения ревизии открыто %d", opens.Load())
	}
}

// Ошибка «нет изображений» ревизии 0 для архива из «./»-записей
// перепроверяется: архив становится галереей.
func TestRecheckNoImages(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	p := writeZip(t, dir, "dot.zip", file{"./001.jpg", img}, file{"./002.jpg", img})
	info := stat(t, p)
	store := newMemStore()
	store.entries["dot.zip"] = StoredEntry{Size: info.Size(), ModTime: info.ModTime(), Err: ErrNoImages}

	src := NewSourceWithStore(storage.NewFS(dir), nil, store)
	if res := src.LoadCatalog(); len(res.Errors) != 1 {
		t.Fatalf("до перепроверки ошибки из каталога: %v", res.Errors)
	}
	res, err := src.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Added) != 1 || len(res.Added[0].Pages) != 2 {
		t.Fatalf("после перепроверки: ошибки %v, добавлены %v", res.Errors, keys(res.Added))
	}
}

// Галерея ревизии 0 с именами, искажёнными прежним сохранением (U+FFFD),
// показывается из каталога до сканирования, затем разбирается заново, и
// её страницы открываются.
func TestRecheckMangledNames(t *testing.T) {
	dir := t.TempDir()
	a, b := sjis(t, "作品"), sjis(t, "漫画")
	writeZip(t, dir, "sjis.zip", file{a + "/001.jpg", pngBytes(t, 2, 3)}, file{b + "/001.jpg", pngBytes(t, 3, 4)})
	store := newMemStore()
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	// как прежний каталог: encoding/json заменял байты не в UTF-8 на U+FFFD
	e := store.entries["sjis.zip"]
	pages := make([]model.Page, len(e.Gallery.Pages))
	for i, p := range e.Gallery.Pages {
		pages[i] = model.Page{Name: strings.ToValidUTF8(p.Name, "�")}
	}
	e.Gallery.Pages = pages
	store.entries["sjis.zip"] = e
	oldRevision(store)

	src := NewSourceWithStore(storage.NewFS(dir), nil, store)
	if res := src.LoadCatalog(); len(res.Galleries) != 1 {
		t.Fatalf("до перепроверки галереи из каталога: %v", keys(res.Galleries))
	}
	res, err := src.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Changed) != 1 {
		t.Fatalf("галерея не перепроверена: изменены %v", keys(res.Changed))
	}
	g, _ := src.Get(model.LocalKey("sjis.zip"))
	if got := pageNames(g); !reflect.DeepEqual(got, []string{a + "/001.jpg", b + "/001.jpg"}) {
		t.Fatalf("страницы %q", got)
	}
	for _, p := range g.Pages {
		rc, err := src.OpenPage(g.Key, p.Name)
		if err != nil {
			t.Fatalf("%q: %v", p.Name, err)
		}
		rc.Close()
	}
}

// Файл занят при перепроверке (в том числе как ошибка BusyAsError) —
// ревизия не сохраняется, пока файл не перепроверен.
func TestRecheckBusy(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "a.zip", file{`ch1\001.jpg`, img})
	store := newMemStore()
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldRevision(store)

	st := &busyStorage{Storage: storage.NewFS(dir), busy: map[string]bool{"a.zip": true}}
	sc := NewScannerWithStore(st, store)
	for _, asError := range []bool{false, true} {
		sc.BusyAsError = asError
		if _, err := sc.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		if store.rev != 0 {
			t.Fatalf("файл занят (BusyAsError=%v), а ревизия сохранена", asError)
		}
	}
	st.setBusy("a.zip", false)
	sc.BusyAsError = false
	res, err := sc.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries) != 1 || store.rev != ParseRevision {
		t.Fatalf("после освобождения: галереи %v, ревизия %d", keys(res.Galleries), store.rev)
	}
}

// Результаты перепроверки не сохранились — ревизия в этом запуске не
// записывается, следующий запуск перепроверяет заново.
func TestRecheckSaveFailure(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "a.zip", file{`ch1\001.jpg`, pngBytes(t, 2, 2)})
	store := newMemStore()
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldRevision(store)

	sc := NewScannerWithStore(storage.NewFS(dir), store)
	store.failSave = true
	if _, err := sc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	store.failSave = false
	if _, err := sc.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.rev != 0 {
		t.Fatal("ревизия записана, хотя результаты перепроверки не сохранились")
	}
	opens := countOpens(t)
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 1 || store.rev != ParseRevision {
		t.Errorf("следующий запуск: открыто %d, ревизия %d", opens.Load(), store.rev)
	}
}

func TestNeedsRecheck(t *testing.T) {
	gal := func(names ...string) StoredEntry {
		var g model.Gallery
		for _, n := range names {
			g.Pages = append(g.Pages, model.Page{Name: n})
		}
		return StoredEntry{Gallery: g}
	}
	hyoushi := sjis(t, "表紙")
	for _, c := range []struct {
		name string
		e    StoredEntry
		want bool
	}{
		{"обычная", gal("a/1.jpg", "a/2.jpg"), false},
		{"«/» в начале", gal("/1.jpg"), false},
		{"«\\» в UTF-8", gal(`a\1.jpg`), true},
		{"0x5C внутри Shift-JIS", gal(hyoushi + ".jpg"), false},
		{"U+FFFD", gal("��/1.jpg"), true},
		{"повтор", gal("1.jpg", "1.jpg"), true},
		{"нет изображений", StoredEntry{Err: ErrNoImages}, true},
		{"другая ошибка", StoredEntry{Err: ErrNotZip}, false},
		{"восстановленная ошибка", StoredEntry{Err: RestoreError(ErrorKind(ErrNoImages), ErrNoImages.Error())}, true},
		{"обёрнутая ошибка", StoredEntry{Err: errors.Join(errors.New("x"), ErrNoImages)}, true},
	} {
		if got := needsRecheck(c.e); got != c.want {
			t.Errorf("%s: %v, ожидалось %v", c.name, got, c.want)
		}
	}
}
