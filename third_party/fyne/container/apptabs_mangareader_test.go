// mangareader: тест правки applyTheme (third_party/fyne/PATCHES.md).

package container

import (
	"image/color"
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

// Подпись вкладки с DangerImportance — цвета ошибки темы, выбрана вкладка
// или нет.
func TestTabItemDangerLabel(t *testing.T) {
	test.NewTempApp(t)
	errTab := NewTabItem("err", widget.NewLabel("e"))
	errTab.SetImportance(widget.DangerImportance)
	tabs := NewAppTabs(NewTabItem("1", widget.NewLabel("1")), errTab)
	w := test.NewTempWindow(t, tabs)
	w.Resize(fyne.NewSize(300, 300))

	want := theme.Color(theme.ColorNameError)
	for _, sel := range []int{0, 1} {
		tabs.SelectIndex(sel)
		got := labelColor(t, errTab.button)
		if got != want {
			t.Errorf("выбрана вкладка %d: цвет подписи %v, ожидался %v", sel, got, want)
		}
	}
	if c := labelColor(t, tabs.Items[0].button); c == want {
		t.Errorf("обычная вкладка получила цвет ошибки")
	}
	errTab.SetImportance(widget.MediumImportance)
	tabs.Refresh()
	if c := labelColor(t, errTab.button); c == want {
		t.Error("после снятия важности подпись осталась красной")
	}
}

func labelColor(t *testing.T, b *tabButton) color.Color {
	t.Helper()
	r := test.TempWidgetRenderer(t, b).(*tabButtonRenderer)
	return r.label.Color
}
