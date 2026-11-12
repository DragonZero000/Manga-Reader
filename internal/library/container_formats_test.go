package library

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// Фикстуры testdata/formats: одинаковое содержимое (три PNG 8×12, 9×12, 10×12
// и meta.json) в RAR 5 и 7z: обычные, solid и с паролем.
var formatFixtures = []string{"plain.rar", "solid.rar", "plain.7z", "solid.7z"}

// formatsDir копирует фикстуры в временную папку.
func formatsDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "formats", n))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, n), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestScanRarAnd7z(t *testing.T) {
	dir := formatsDir(t, formatFixtures...)
	res := scanDir(t, dir)
	if len(res.Galleries) != len(formatFixtures) || len(res.Errors) != 0 {
		t.Fatalf("галереи %v, ошибки %v", keys(res.Galleries), res.Errors)
	}
	for _, g := range res.Galleries {
		if len(g.Pages) != 3 {
			t.Errorf("%s: страниц %d", g.Key, len(g.Pages))
		}
		if g.Title != "Formats Test" || !g.HasMeta {
			t.Errorf("%s: название %q, HasMeta %v", g.Key, g.Title, g.HasMeta)
		}
		if g.Fingerprint == "" {
			t.Errorf("%s: нет отпечатка", g.Key)
		}
	}
}

func TestScanPassword(t *testing.T) {
	dir := formatsDir(t, "password.rar", "password.7z", "plain.rar")
	res := scanDir(t, dir)
	if len(res.Galleries) != 1 || len(res.Errors) != 2 {
		t.Fatalf("галереи %v, ошибки %v", keys(res.Galleries), res.Errors)
	}
	for _, e := range res.Errors {
		if !errors.Is(e.Err, ErrPassword) {
			t.Errorf("%s: %v", e.RelPath, e.Err)
		}
	}
}

// Страницы читаются в любом порядке, с кэшем solid-RAR и без него.
func TestOpenPageRarAnd7z(t *testing.T) {
	for _, withCache := range []bool{true, false} {
		dir := formatsDir(t, formatFixtures...)
		src := NewSource(storage.NewFS(dir), search.NewMemIndex())
		var cacheDir string
		if withCache {
			cacheDir = filepath.Join(t.TempDir(), "solid")
			src.SetSolidCache(NewSolidCache(cacheDir, 0))
		}
		if _, err := src.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		for _, name := range formatFixtures {
			k := model.LocalKey(name)
			g, ok := src.Get(k)
			if !ok {
				t.Fatalf("нет галереи %s", name)
			}
			// с конца и обратно: solid-архивам нужен возврат к началу
			for i := len(g.Pages) - 1; i >= 0; i-- {
				rc, err := src.OpenPage(k, g.Pages[i].Name)
				if err != nil {
					t.Fatalf("cache=%v %s: %v", withCache, g.Pages[i].Name, err)
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					t.Fatal(err)
				}
				cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
				if err != nil || cfg.Width != 8+i || cfg.Height != 12 {
					t.Errorf("cache=%v %s: размер %dx%d, ошибка %v", withCache, g.Pages[i].Name, cfg.Width, cfg.Height, err)
				}
			}
			sizes, err := src.PageSizes(k)
			if err != nil || len(sizes) != 3 || sizes[2].Width != 10 {
				t.Errorf("%s: PageSizes %v, %v", name, sizes, err)
			}
		}
		if withCache {
			// solid-RAR распакован в кэш, обычные архивы — нет
			items, _ := os.ReadDir(cacheDir)
			if len(items) != 1 {
				t.Errorf("в кэше %d папок, ожидалась 1", len(items))
			}
			src.ClearSolidCache()
			if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
				t.Errorf("кэш не очищен: %v", err)
			}
		}
	}
}

// Архив, не помещающийся в лимит кэша, даёт понятную ошибку.
func TestSolidCacheTooLarge(t *testing.T) {
	dir := formatsDir(t, "solid.rar")
	src := NewSource(storage.NewFS(dir), search.NewMemIndex())
	src.SetSolidCache(NewSolidCache(filepath.Join(t.TempDir(), "solid"), 10)) // 10 байт
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	k := model.LocalKey("solid.rar")
	g, _ := src.Get(k)
	if _, err := src.OpenPage(k, g.Pages[0].Name); !errors.Is(err, ErrSolidTooLarge) {
		t.Errorf("ошибка: %v", err)
	}
}

// Вытеснение: при нехватке места удаляется давно не использованный архив.
func TestSolidCacheEvicts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "solid")
	c := NewSolidCache(root, 100)
	put := func(key string, size int) {
		t.Helper()
		_, err := c.ensure(key, int64(size), func(write func(string, io.Reader) error) error {
			return write("f", bytes.NewReader(make([]byte, size)))
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	put("a", 60)
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "a"), old, old); err != nil {
		t.Fatal(err)
	}
	put("b", 60) // вместе не помещаются: «a» вытесняется
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Errorf("«a» не вытеснен: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "b", solidDone)); err != nil {
		t.Errorf("«b»: %v", err)
	}
}

// Ошибка «пароль» сохраняется в каталоге и восстанавливается.
func TestPasswordErrorRoundTrip(t *testing.T) {
	err := RestoreError(ErrorKind(ErrPassword), ErrPassword.Error())
	if !errors.Is(err, ErrPassword) {
		t.Fatalf("восстановлено: %v", err)
	}
}

// Ранее отклонённые RAR/7z перепроверяются после обновления.
func TestRecheckOtherArchive(t *testing.T) {
	e := StoredEntry{Err: &UnsupportedError{Kind: KindOtherArchive}}
	if !needsRecheck(e) {
		t.Error("RAR/7z с прежней ошибкой не перепроверяется")
	}
	if needsRecheck(StoredEntry{Err: &UnsupportedError{Kind: KindImage}}) {
		t.Error("изображение не должно перепроверяться")
	}
}
