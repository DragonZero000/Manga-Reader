package browser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type bridgeRec struct {
	mu        sync.Mutex
	downloads []string
	shows     int
}

func startBridge(t *testing.T, dir string) (*Bridge, *bridgeRec) {
	t.Helper()
	rec := &bridgeRec{}
	b, err := NewBridge(dir,
		func(rel string) { rec.mu.Lock(); rec.downloads = append(rec.downloads, rel); rec.mu.Unlock() },
		func() { rec.mu.Lock(); rec.shows++; rec.mu.Unlock() })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b, rec
}

func call(t *testing.T, b *Bridge, path, token string, body any) int {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d%s", b.Port(), path), bytes.NewReader(data))
	req.Header.Set("X-MangaReader-Token", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestBridgeAuth(t *testing.T) {
	b, rec := startBridge(t, t.TempDir())
	if len(b.Token()) != 64 {
		t.Fatalf("токен: %q", b.Token())
	}
	if code := call(t, b, "/show", "", nil); code != http.StatusForbidden {
		t.Errorf("без токена: %d", code)
	}
	if code := call(t, b, "/show", "неверный", nil); code != http.StatusForbidden {
		t.Errorf("неверный токен: %d", code)
	}
	if code := call(t, b, "/show", b.Token(), struct{}{}); code != http.StatusNoContent || rec.shows != 1 {
		t.Errorf("show: %d, вызовов %d", code, rec.shows)
	}
}

func TestBridgeDownload(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("адрес страницы хранится в потоке NTFS")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "g-535147.zip")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte("x"), 0o644)
	b, rec := startBridge(t, dir)

	page := "https://site.example/g/535147/"
	if code := call(t, b, "/download", b.Token(), map[string]string{"file": file, "page": page}); code != http.StatusNoContent {
		t.Fatalf("download: %d", code)
	}
	if len(rec.downloads) != 1 || rec.downloads[0] != "sub/g-535147.zip" {
		t.Fatalf("OnDownload: %v", rec.downloads)
	}
	data, err := os.ReadFile(file + ":mangareader.source")
	if err != nil || string(data) != page+"\n" {
		t.Fatalf("поток: %q %v", data, err)
	}

	// вне папки загрузок, не http(s), мусор — отказ без записи
	outside := filepath.Join(t.TempDir(), "x.zip")
	os.WriteFile(outside, []byte("x"), 0o644)
	for _, body := range []map[string]string{
		{"file": outside, "page": page},
		{"file": filepath.Join(dir, "..", "x.zip"), "page": page},
		{"file": "sub/g-535147.zip", "page": page},
		{"file": file, "page": "javascript:alert(1)"},
	} {
		if code := call(t, b, "/download", b.Token(), body); code != http.StatusBadRequest {
			t.Errorf("%v: %d", body, code)
		}
	}
	if _, err := os.Stat(outside + ":mangareader.source"); err == nil {
		t.Error("поток записан вне папки загрузок")
	}
	if len(rec.downloads) != 1 {
		t.Errorf("лишние OnDownload: %v", rec.downloads)
	}
}

func TestInsideDir(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "manga")
	for file, want := range map[string]string{
		filepath.Join(dir, "a.zip"):       "a.zip",
		filepath.Join(dir, "s", "b.zip"):  "s/b.zip",
		filepath.Join(dir, "..", "c.zip"): "",
		dir:                               "",
		filepath.Join(dir+"2", "d.zip"):   "",
		"a.zip":                           "",
	} {
		got, ok := insideDir(dir, file)
		if (want == "") == ok || got != want {
			t.Errorf("%s: %q %v, ожидалось %q", file, got, ok, want)
		}
	}
}
