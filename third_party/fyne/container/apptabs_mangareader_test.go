// mangareader: тест правки applyTheme (third_party/fyne/PATCHES.md).

package container

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// refreshCounter — виджет, считающий вызовы Refresh.
type refreshCounter struct {
	widget.Label
	refreshes int
}

func (r *refreshCounter) Refresh() {
	r.refreshes++
	r.Label.Refresh()
}

func newRefreshCounter() *refreshCounter {
	r := &refreshCounter{}
	r.ExtendBaseWidget(r)
	return r
}

// Выбор вкладки не обновляет содержимое вкладок (Refresh освобождал бы
// текстуры изображений); смена темы — обновляет.
func TestAppTabsSelectKeepsContent(t *testing.T) {
	a := test.NewTempApp(t)
	first, second := newRefreshCounter(), newRefreshCounter()
	tabs := NewAppTabs(NewTabItem("1", NewStack(first)), NewTabItem("2", NewStack(second)))
	w := test.NewTempWindow(t, tabs)
	w.Resize(fyne.NewSize(200, 200))

	first.refreshes, second.refreshes = 0, 0
	tabs.SelectIndex(1)
	tabs.SelectIndex(0)
	if first.refreshes != 0 || second.refreshes != 0 {
		t.Fatalf("выбор вкладки обновил содержимое: %d и %d раз", first.refreshes, second.refreshes)
	}

	a.Settings().SetTheme(theme.DarkTheme())
	tabs.Refresh()
	if first.refreshes == 0 || second.refreshes == 0 {
		t.Fatal("смена темы не обновила содержимое вкладок")
	}
}
