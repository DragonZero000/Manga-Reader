package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// titles — подписи вкладок: statistics, library, search, errors, settings.
func tabNames(names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = i18n.T("tab." + n)
	}
	return out
}

// На Windows «Статистика» первая, на Android — после «Поиска»; стартовая
// вкладка — «Библиотека» на обеих.
func TestTabOrderPerPlatform(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		want   []string
	}{
		{"windows", false, tabNames("stats", "library", "search", "settings")},
		{"android", true, tabNames("library", "search", "stats", "settings")},
	} {
		old := isMobile
		isMobile = func() bool { return tc.mobile }
		s, _, _ := newTestShell(t)
		isMobile = old
		if got := s.TabTitles(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: вкладки %v, ожидались %v", tc.name, got, tc.want)
		}
		if s.Tabs.Selected() != s.libraryTab {
			t.Errorf("%s: выбрана %q, ожидалась «Библиотека»", tc.name, s.Tabs.Selected().Text)
		}
	}
}

// Вкладка «Ошибки» появляется перед «Настройками», пока есть ошибки,
// не сбивает выбранную вкладку и исчезает вместе с ошибками.
func TestErrorsTabAppearsAndDisappears(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		dir := t.TempDir()
		old := isMobile
		isMobile = func() bool { return mobile }
		e := newErrorsShell(t, dir, storage.NewMemSettings())
		isMobile = old
		e.scan()
		if e.errorsTabShown() || len(e.Tabs.Items) != 4 {
			t.Fatalf("mobile=%v: без ошибок вкладки «Ошибки» быть не должно: %v", mobile, e.TabTitles())
		}

		// появилась ошибка, выбрана «Настройки»: выбор сохраняется
		e.Tabs.Select(e.settingsTab)
		p := filepath.Join(dir, "page.jpg")
		if err := os.WriteFile(p, jpegHeader, 0o644); err != nil {
			t.Fatal(err)
		}
		e.scan()
		titles := e.TabTitles()
		if !e.errorsTabShown() || titles[len(titles)-2] != i18n.T("tab.errors") || titles[len(titles)-1] != i18n.T("tab.settings") {
			t.Fatalf("mobile=%v: после появления ошибки: %v", mobile, titles)
		}
		if e.Tabs.Selected() != e.settingsTab {
			t.Errorf("mobile=%v: вставка вкладки сбила выбор: %q", mobile, e.Tabs.Selected().Text)
		}
		if !strings.Contains(e.badge(), "-1-") {
			t.Errorf("mobile=%v: на иконке нет числа 1: %s", mobile, e.badge())
		}

		// открыли «Ошибки», затем ушли: вкладка остаётся, числа нет
		e.Tabs.Select(e.errorsTab)
		e.Tabs.Select(e.libraryTab)
		if !e.errorsTabShown() || strings.HasPrefix(e.badge(), "badge-") {
			t.Errorf("mobile=%v: просмотренные ошибки: вкладка %v, иконка %s", mobile, e.errorsTabShown(), e.badge())
		}

		// последняя ошибка исчезла, пока вкладка «Ошибки» выбрана
		e.Tabs.Select(e.errorsTab)
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		e.scan()
		if e.errorsTabShown() || len(e.Tabs.Items) != 4 {
			t.Fatalf("mobile=%v: вкладка «Ошибки» осталась: %v", mobile, e.TabTitles())
		}
		if e.Tabs.Selected() != e.statsTab {
			t.Errorf("mobile=%v: выбрана %q, ожидалась «Статистика»", mobile, e.Tabs.Selected().Text)
		}
	}
}

// Исчезновение вкладки «Ошибки» левее выбранной сохраняет выбор.
func TestErrorsTabRemovalKeepsSelection(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "page.jpg")
	if err := os.WriteFile(p, jpegHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	e := newErrorsShell(t, dir, storage.NewMemSettings())
	e.scan()
	e.Tabs.Select(e.settingsTab)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	e.scan()
	if e.errorsTabShown() || e.Tabs.Selected() != e.settingsTab {
		t.Errorf("вкладки %v, выбрана %q", e.TabTitles(), e.Tabs.Selected().Text)
	}
}

// Открытие «Статистики» считает её в фоне; повторное открытие без
// изменений библиотеки не пересчитывает; нажатие на тег открывает «Поиск».
func TestStatsTabComputesAndGoesToSearch(t *testing.T) {
	s, _, pump := newTestShell(t)
	st := s.Stats()
	s.Tabs.Select(s.statsTab)
	waitUI(t, pump, "статистика посчитана", func() bool { return !st.Counting() && st.Result().Works == 1 })
	seq := st.Seq()

	s.Tabs.Select(s.libraryTab)
	s.Tabs.Select(s.statsTab)
	pump()
	if st.Seq() != seq {
		t.Errorf("повторное открытие пересчитало статистику (%d → %d)", seq, st.Seq())
	}

	rows := st.TagRows("artist")
	if len(rows) != 1 || rows[0] != "artist 1" {
		t.Fatalf("авторы: %v", rows)
	}
	st.TapTagRow("artist", 0)
	if s.Tabs.Selected() != s.searchTab || s.Search().Query() != `artist:"artist 1"` {
		t.Errorf("после нажатия: вкладка %q, запрос %q", s.Tabs.Selected().Text, s.Search().Query())
	}
	waitUI(t, pump, "результаты поиска", func() bool { return len(s.Search().Results()) == 1 })
	if s.Search().Results()[0].Key != model.LocalKey("example.zip") {
		t.Errorf("результат: %v", s.Search().Results())
	}
}

// Библиотека изменилась при открытой «Статистике» — пересчёт сразу; при
// закрытой — только при открытии.
func TestStatsTabRecomputesOnLibraryChange(t *testing.T) {
	s, _, pump := newTestShell(t)
	st := s.Stats()
	s.Tabs.Select(s.statsTab)
	waitUI(t, pump, "первый подсчёт", func() bool { return !st.Counting() && st.Result().Works == 1 })
	seq := st.Seq()

	s.libraryChanged()
	if st.Seq() != seq+1 {
		t.Fatalf("открытая вкладка не пересчитана: %d → %d", seq, st.Seq())
	}
	waitUI(t, pump, "второй подсчёт", func() bool { return !st.Counting() })

	s.Tabs.Select(s.libraryTab)
	s.libraryChanged()
	if st.Seq() != seq+1 {
		t.Errorf("закрытая вкладка пересчитана без открытия: %d", st.Seq())
	}
	s.Tabs.Select(s.statsTab)
	if st.Seq() != seq+2 {
		t.Errorf("открытие после изменения не пересчитало: %d", st.Seq())
	}
	waitUI(t, pump, "третий подсчёт", func() bool { return !st.Counting() })
}
