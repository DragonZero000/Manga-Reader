package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"mangareader/internal/storage"
)

// memStore — ScanStore в памяти, как каталог между «запусками».
type memStore struct {
	mu      sync.Mutex
	entries map[string]StoredEntry
	links   map[string]string
	saves   []ScanDelta
}

func newMemStore() *memStore {
	return &memStore{entries: map[string]StoredEntry{}, links: map[string]string{}}
}

func (m *memStore) Load() (map[string]StoredEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]StoredEntry{}
	for k, v := range m.entries {
		// как каталог: ошибка переживает сохранение по виду и тексту
		if v.Err != nil {
			v.Err = RestoreError(ErrorKind(v.Err), v.Err.Error())
		}
		out[k] = v
	}
	return out, nil
}

func (m *memStore) Save(d ScanDelta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves = append(m.saves, d)
	for rel, e := range d.Put {
		m.entries[rel] = e
	}
	for _, rel := range d.Delete {
		delete(m.entries, rel)
		delete(m.links, rel)
	}
	for rel, u := range d.Links {
		m.links[rel] = u
	}
	return nil
}

func (m *memStore) StoredLink(rel string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.links[rel]
}

func TestScannerStorePersists(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "a.zip", file{"1.png", img})
	writeZip(t, dir, "b.zip", file{"1.png", img})
	writeFile(t, dir, "page.html", []byte("<!doctype html><html><body>captcha</body></html>"))
	store := newMemStore()

	res1, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.saves) != 1 || len(store.saves[0].Put) != 3 {
		t.Fatalf("первое сканирование: %d сохранений, %v", len(store.saves), store.saves)
	}

	// «перезапуск»: новый сканер над тем же хранилищем — ни одного открытия
	opens := countOpens(t)
	s2 := NewScannerWithStore(storage.NewFS(dir), store)
	if cached := s2.Cached(); len(cached.Galleries) != 2 || len(cached.Errors) != 1 {
		t.Fatalf("из хранилища: %d галерей, %d ошибок", len(cached.Galleries), len(cached.Errors))
	}
	res2, err := s2.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 0 || res2.Opened != 0 {
		t.Fatalf("после перезапуска открыто %d архивов", opens.Load())
	}
	if !reflect.DeepEqual(keys(res1.Galleries), keys(res2.Galleries)) || len(res2.Errors) != 1 {
		t.Fatalf("результат изменился: %v → %v", keys(res1.Galleries), keys(res2.Galleries))
	}
	if res2.Errors[0].Err.Error() != res1.Errors[0].Err.Error() {
		t.Fatalf("причина ошибки: %q → %q", res1.Errors[0].Err, res2.Errors[0].Err)
	}
	if len(store.saves) != 1 {
		t.Fatalf("без изменений сохранять нечего: %d сохранений", len(store.saves))
	}

	// удалён a.zip, добавлен c.zip
	os.Remove(filepath.Join(dir, "a.zip"))
	writeZip(t, dir, "c.zip", file{"1.png", img})
	if _, err := s2.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := store.saves[len(store.saves)-1]
	if _, ok := last.Put["c.zip"]; !ok || len(last.Put) != 1 || !reflect.DeepEqual(last.Delete, []string{"a.zip"}) {
		t.Fatalf("изменения: put %v, delete %v", last.Put, last.Delete)
	}
}

func TestErrorKindRoundTrip(t *testing.T) {
	for _, err := range []error{
		ErrEmpty,
		ErrNoImages,
		ErrNotZip,
		fmt.Errorf("%w: zip: not a valid zip file", ErrNotZip),
		&UnsupportedError{Kind: KindHTML},
		&UnsupportedError{Kind: KindZip, Ext: ".rar"},
		errors.New("не удалось открыть: нет доступа"),
	} {
		got := RestoreError(ErrorKind(err), err.Error())
		if got.Error() != err.Error() {
			t.Errorf("%v: текст %q", err, got)
		}
		for _, base := range []error{ErrEmpty, ErrNoImages, ErrNotZip} {
			if errors.Is(err, base) != errors.Is(got, base) {
				t.Errorf("%v: errors.Is(%v) изменился", err, base)
			}
		}
		var ue1, ue2 *UnsupportedError
		if errors.As(err, &ue1) != errors.As(got, &ue2) || (ue1 != nil && *ue1 != *ue2) {
			t.Errorf("%v: UnsupportedError изменился: %v → %v", err, ue1, ue2)
		}
	}
}

// Ссылка скачанного файла: вторая копия в хранилище и восстановление,
// когда её больше нет в links.json / метке загрузки.
func TestScannerLinkCopy(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "g.zip", file{"1.png", img})
	links := LoadLinks(filepath.Join(t.TempDir(), "links.json"))
	links.Set("g.zip", "https://site.example/g/1/")
	store := newMemStore()

	if _, err := NewScannerWithStore(WithLinks(storage.NewFS(dir), links), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.StoredLink("g.zip") != "https://site.example/g/1/" {
		t.Fatalf("ссылка не сохранена второй копией: %v", store.links)
	}

	// links.json пуст (повреждён), файл изменён — ссылка берётся из хранилища
	empty := LoadLinks(filepath.Join(t.TempDir(), "links.json"))
	writeZip(t, dir, "g.zip", file{"1.png", img}, file{"2.png", img})
	res, err := NewScannerWithStore(WithLinks(storage.NewFS(dir), empty), store).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries) != 1 || res.Galleries[0].SourceURL != "https://site.example/g/1/" {
		t.Fatalf("ссылка после потери links.json: %+v", res.Galleries)
	}

	// файл удалён — копия удаляется
	os.Remove(filepath.Join(dir, "g.zip"))
	if _, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.StoredLink("g.zip") != "" {
		t.Fatal("ссылка удалённого файла осталась")
	}
}
