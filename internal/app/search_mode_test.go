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
