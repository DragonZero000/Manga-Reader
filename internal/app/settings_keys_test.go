package app

import (
	"testing"

	"mangareader/internal/storage"
)

func TestSearchMode(t *testing.T) {
	s := storage.NewMemSettings()
	if m := SearchMode(s); m != SearchModeDynamic {
		t.Fatalf("по умолчанию %q", m)
	}
	s.SetString(KeySearchMode, SearchModeSubmit)
	if m := SearchMode(s); m != SearchModeSubmit {
		t.Fatalf("после выбора %q", m)
	}
	s.SetString(KeySearchMode, "что-то")
	if m := SearchMode(s); m != SearchModeDynamic {
		t.Fatalf("неизвестное значение %q", m)
	}
}

func TestDisplayMax60(t *testing.T) {
	s := storage.NewMemSettings()
	if !DisplayMax60(s) {
		t.Fatal("по умолчанию 60 Гц должно быть включено")
	}
	SetDisplayMax60(s, false)
	if DisplayMax60(s) || s.String(KeyDisplayMax60, "") != "0" {
		t.Fatal("выключение не сохранилось")
	}
	SetDisplayMax60(s, true)
	if !DisplayMax60(s) {
		t.Fatal("включение не сохранилось")
	}
}
