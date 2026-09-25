package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceURLFromZoneStream(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "g-535147.zip")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := NewFS(dir)
	if got := fs.SourceURL("g-535147.zip"); got != "" {
		t.Fatalf("без метки: %q", got)
	}
	zone := "[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://site.example/g/535147/\r\n"
	if err := os.WriteFile(p+":Zone.Identifier", []byte(zone), 0o644); err != nil {
		t.Skipf("файловая система без альтернативных потоков: %v", err)
	}
	if got := fs.SourceURL("g-535147.zip"); got != "https://site.example/g/535147/" {
		t.Fatalf("с меткой: %q", got)
	}
	// метка переезжает вместе с файлом при переименовании
	if err := os.Rename(p, filepath.Join(dir, "мой.zip")); err != nil {
		t.Fatal(err)
	}
	if got := fs.SourceURL("мой.zip"); got != "https://site.example/g/535147/" {
		t.Fatalf("после переименования: %q", got)
	}
	if got := fs.SourceURL("../x"); got != "" {
		t.Fatalf("недопустимый путь: %q", got)
	}
}
