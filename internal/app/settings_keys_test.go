package app

import (
	"testing"
	"time"

	"mangareader/internal/i18n"
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

func TestRandomMode(t *testing.T) {
	s := storage.NewMemSettings()
	if m := RandomMode(s); m != RandomModeRepeat {
		t.Fatalf("по умолчанию %q", m)
	}
	SetRandomMode(s, RandomModeNoRepeat)
	if m := RandomMode(s); m != RandomModeNoRepeat || s.String(KeyRandomMode, "") != RandomModeNoRepeat {
		t.Fatalf("после выбора %q", m)
	}
	s.SetString(KeyRandomMode, "что-то")
	if m := RandomMode(s); m != RandomModeRepeat {
		t.Fatalf("неизвестное значение %q", m)
	}
}

func TestUILanguage(t *testing.T) {
	s := storage.NewMemSettings()
	if l := UILanguage(s); l != i18n.Auto {
		t.Fatalf("по умолчанию %q", l)
	}
	SetUILanguage(s, "en")
	if l := UILanguage(s); l != "en" || s.String(KeyUILanguage, "") != "en" {
		t.Fatalf("после выбора %q", l)
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

func TestGridColumns(t *testing.T) {
	s := storage.NewMemSettings()
	if n := GridColumns(s); n != 3 {
		t.Fatalf("по умолчанию %d", n)
	}
	for _, n := range []int{2, 4, 3} {
		SetGridColumns(s, n)
		if got := GridColumns(s); got != n {
			t.Fatalf("после выбора %d: %d", n, got)
		}
	}
	for _, v := range []string{"1", "5", "0", "что-то"} {
		s.SetString(KeyGridColumns, v)
		if n := GridColumns(s); n != 3 {
			t.Fatalf("неизвестное значение %q: %d", v, n)
		}
	}
}

func TestGridSize(t *testing.T) {
	s := storage.NewMemSettings()
	if v := GridSize(s); v != GridSizeMedium {
		t.Fatalf("по умолчанию %q", v)
	}
	for _, v := range []string{GridSizeSmall, GridSizeLarge, GridSizeMedium} {
		SetGridSize(s, v)
		if got := GridSize(s); got != v {
			t.Fatalf("после выбора %q: %q", v, got)
		}
	}
	s.SetString(KeyGridSize, "xl")
	if v := GridSize(s); v != GridSizeMedium {
		t.Fatalf("неизвестное значение %q", v)
	}
}

func TestRetention(t *testing.T) {
	s := storage.NewMemSettings()
	if d := RetentionDays(s); d != 30 || Retention(s) != 30*24*time.Hour {
		t.Fatalf("по умолчанию %d дней, %v", d, Retention(s))
	}
	for _, n := range []int{1, 45, 0, MaxRetentionDays} {
		SetRetentionDays(s, n)
		if got := RetentionDays(s); got != n || Retention(s) != time.Duration(n)*24*time.Hour {
			t.Fatalf("после выбора %d: %d, %v", n, got, Retention(s))
		}
	}
	for _, v := range []string{"-1", "36501", "abc", "1.5", ""} {
		s.SetString(KeyRetention, v)
		if d := RetentionDays(s); d != DefaultRetentionDays {
			t.Fatalf("недопустимое значение %q: %d", v, d)
		}
	}
}
