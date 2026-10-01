package reader

import (
	"testing"

	"fyne.io/fyne/v2"

	"mangareader/internal/model"
)

// pageLog записывает сообщения onPage.
func pageLog(r *Reader) *[]int {
	var got []int
	r.SetOnPage(func(_ model.Gallery, page int) { got = append(got, page) })
	return &got
}

// Открытие на странице: в постраничном режиме и в ленте.
func TestOpenAt(t *testing.T) {
	r, g := setupPages(t, 87, 10, 10)
	r.OpenAt(g, 33)
	if r.cur != 33 || r.paged.page != 33 || r.panel.pageLbl.Text != "34 / 87" {
		t.Fatalf("постраничный: страница %d, надпись %q", r.paged.page, r.panel.pageLbl.Text)
	}
	r.Close()

	r.setMode(ModeStrip)
	r.OpenAt(g, 33)
	eventually(t, r, "раскладка ленты", func() bool { return r.strip.ready })
	if r.cur != 33 || r.strip.top != 33 || r.panel.pageLbl.Text != "34 / 87" {
		t.Fatalf("лента: верхняя страница %d, надпись %q", r.strip.top, r.panel.pageLbl.Text)
	}
}

// Без листания позиция не сообщается — ни при открытии, ни при закрытии.
func TestNoPageReportWithoutFlip(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	got := pageLog(r)
	r.OpenAt(g, 2)
	r.Close()
	if len(*got) != 0 {
		t.Fatalf("сообщения без листания: %v", *got)
	}
}

// Листание сообщает страницы; возврат на первую — тоже; закрытие — итоговую.
func TestPageReports(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	got := pageLog(r)
	r.Open(g)
	r.TypedKey(fyne.KeyRight)
	r.TypedKey(fyne.KeyLeft)
	r.Close()
	want := []int{1, 0, 0}
	if len(*got) != len(want) {
		t.Fatalf("сообщения %v, ждали %v", *got, want)
	}
	for i := range want {
		if (*got)[i] != want[i] {
			t.Fatalf("сообщения %v, ждали %v", *got, want)
		}
	}
}

// Во время перемещения ползунка позиция не сообщается, после отпускания — да.
func TestNoPageReportWhileScrubbing(t *testing.T) {
	r, g := setupPages(t, 87, 10, 10)
	got := pageLog(r)
	r.Open(g)
	r.panel.show()
	for v := 2.0; v <= 40; v++ {
		r.panel.slider.Value = v
		r.panel.slider.OnChanged(v)
	}
	if len(*got) != 0 {
		t.Fatalf("сообщения во время перемещения: %v", *got)
	}
	r.panel.slider.OnChangeEnded(40)
	if len(*got) != 1 || (*got)[0] != 39 {
		t.Fatalf("после отпускания: %v", *got)
	}
}

// Лента сообщает верхнюю страницу.
func TestStripPageReport(t *testing.T) {
	r, g := setupPages(t, 12, 100, 400)
	got := pageLog(r)
	r.setMode(ModeStrip)
	r.Open(g)
	v := r.strip
	eventually(t, r, "раскладка ленты", func() bool { return v.ready })
	v.scroll.ScrollToOffset(fyne.NewPos(0, v.prefix[11]+1))
	v.updateVisible()
	r.Close()
	if n := len(*got); n == 0 || (*got)[n-1] != 11 {
		t.Fatalf("лента: сообщения %v, ждали последнюю 11", *got)
	}
}
