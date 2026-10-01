package screens

import (
	"io"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
)

// noOpen — страницы не открываются (обложки в этих тестах не нужны).
func noOpen(model.Key, string) (io.ReadCloser, error) { return nil, storage.ErrUnavailable }

// menuLabels — подписи пунктов меню; разделитель — "-".
func menuLabels(m *fyne.Menu) []string {
	var out []string
	for _, it := range m.Items {
		if it.IsSeparator {
			out = append(out, "-")
		} else {
			out = append(out, it.Label)
		}
	}
	return out
}

func labels(keys ...string) []string {
	var out []string
	for _, k := range keys {
		if k == "-" {
			out = append(out, k)
		} else {
			out = append(out, i18n.T(k))
		}
	}
	return out
}

func allActions() *GalleryActions {
	f := func(model.Gallery) {}
	return &GalleryActions{Open: f, Read: f, OpenURL: f, CopyTitle: f, ShowInFolder: f, Delete: f}
}

func TestCardMenuItems(t *testing.T) {
	linked := model.Gallery{Title: "t", SourceURL: "https://site.example/g/1/"}
	plain := model.Gallery{Title: "t"}

	// Windows, ссылка есть — все семь пунктов
	win := allActions()
	want := labels("menu.open", "menu.read", "menu.open_in_browser", "menu.copy_title", "menu.show_in_folder", "-", "menu.delete")
	if got := menuLabels(win.CardMenu(linked)); !slices.Equal(got, want) {
		t.Errorf("Windows со ссылкой: %v", got)
	}
	// без ссылки «Открыть в браузере» нет
	want = labels("menu.open", "menu.read", "menu.copy_title", "menu.show_in_folder", "-", "menu.delete")
	if got := menuLabels(win.CardMenu(plain)); !slices.Equal(got, want) {
		t.Errorf("Windows без ссылки: %v", got)
	}

	// Android: «Показать в папке» нет
	android := allActions()
	android.ShowInFolder = nil
	want = labels("menu.open", "menu.read", "menu.copy_title", "-", "menu.delete")
	if got := menuLabels(android.CardMenu(plain)); !slices.Equal(got, want) {
		t.Errorf("Android без ссылки: %v", got)
	}
	want = labels("menu.open", "menu.read", "menu.open_in_browser", "menu.copy_title", "-", "menu.delete")
	if got := menuLabels(android.CardMenu(linked)); !slices.Equal(got, want) {
		t.Errorf("Android со ссылкой: %v", got)
	}

	// без удаления (прочие платформы) — нет ни разделителя, ни «Удалить…»
	other := allActions()
	other.ShowInFolder, other.Delete = nil, nil
	want = labels("menu.open", "menu.read", "menu.copy_title")
	if got := menuLabels(other.CardMenu(plain)); !slices.Equal(got, want) {
		t.Errorf("без удаления: %v", got)
	}
}

func TestDetailsMenuItems(t *testing.T) {
	g := model.Gallery{Title: "t", SourceURL: "https://site.example/g/1/"}
	want := labels("menu.copy_title", "menu.show_in_folder", "-", "menu.delete")
	if got := menuLabels(allActions().DetailsMenu(g)); !slices.Equal(got, want) {
		t.Errorf("Windows: %v", got)
	}
	android := allActions()
	android.ShowInFolder = nil
	want = labels("menu.copy_title", "-", "menu.delete")
	if got := menuLabels(android.DetailsMenu(g)); !slices.Equal(got, want) {
		t.Errorf("Android: %v", got)
	}
}

// Пункт меню вызывает действие для той галереи, для которой меню построено.
func TestCardMenuAction(t *testing.T) {
	var got []string
	act := allActions()
	act.Read = func(g model.Gallery) { got = append(got, "read "+g.Key.ID) }
	act.Delete = func(g model.Gallery) { got = append(got, "delete "+g.Key.ID) }
	m := act.CardMenu(galleryOf("a.zip"))
	for _, it := range m.Items {
		if it.Action != nil && (it.Label == i18n.T("menu.read") || it.Label == i18n.T("menu.delete")) {
			it.Action()
		}
	}
	if !slices.Equal(got, []string{"read a.zip", "delete a.zip"}) {
		t.Fatalf("действия: %v", got)
	}
}

// findCards — карточки сетки на холсте.
func findCards(o fyne.CanvasObject) []*galleryCard {
	var out []*galleryCard
	switch v := o.(type) {
	case *galleryCard:
		return append(out, v)
	case *fyne.Container:
		for _, c := range v.Objects {
			out = append(out, findCards(c)...)
		}
	case fyne.Widget:
		for _, c := range test.WidgetRenderer(v).Objects() {
			out = append(out, findCards(c)...)
		}
	}
	return out
}

// shownMenu подменяет показ меню и возвращает последнее показанное.
func shownMenu(t *testing.T) *[]*fyne.Menu {
	var menus []*fyne.Menu
	prev := showMenu
	showMenu = func(m *fyne.Menu, _ fyne.Canvas, _ fyne.Position) { menus = append(menus, m) }
	t.Cleanup(func() { showMenu = prev })
	return &menus
}

// Нажатие на «⋮» открывает меню той галереи и не открывает страницу
// произведения; нажатие на карточку вне «⋮» — открывает; правый клик и
// долгое нажатие (вторичное касание) — меню.
func TestCardMenuButton(t *testing.T) {
	a := test.NewTempApp(t)
	menus := shownMenu(t)
	var opened []string
	act := allActions()
	act.Open = func(g model.Gallery) { opened = append(opened, g.Key.ID) }
	g := newGalleryGrid(thumbs.New(noOpen, 1<<20, 1), NewGridMetrics(storage.NewMemSettings()), act)
	g.do = func(func()) {} // обложки не нужны
	w := a.NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(fyne.NewSize(600, 600))
	g.SetItems([]model.Gallery{galleryOf("a.zip"), galleryOf("b.zip")})
	g.Widget().Refresh()

	var card *galleryCard
	for _, c := range findCards(w.Canvas().Content()) {
		if c.Visible() && c.id == 1 {
			card = c
		}
	}
	if card == nil {
		t.Fatal("карточка b.zip не найдена")
	}
	d := a.Driver()
	btn := d.AbsolutePositionForObject(card.menu)
	if s := card.menu.Size(); s.Width < menuTapSize || s.Height < menuTapSize {
		t.Fatalf("область нажатия «⋮» %v", s)
	}
	cardPos := d.AbsolutePositionForObject(card)
	if right := btn.X + card.menu.Size().Width; right-(cardPos.X+card.Size().Width) > 0.5 || btn.Y != cardPos.Y {
		t.Fatalf("«⋮» не в правом верхнем углу: кнопка %v, карточка %v %v", btn, cardPos, card.Size())
	}

	test.TapCanvas(w.Canvas(), btn.AddXY(menuTapSize/2, menuTapSize/2))
	if len(opened) != 0 {
		t.Fatalf("нажатие на «⋮» открыло страницу: %v", opened)
	}
	if len(*menus) != 1 || len((*menus)[0].Items) == 0 {
		t.Fatalf("меню не показано: %v", *menus)
	}

	// нажатие на обложку вне «⋮»
	test.TapCanvas(w.Canvas(), cardPos.AddXY(5, card.Size().Height/2))
	if !slices.Equal(opened, []string{"b.zip"}) {
		t.Fatalf("нажатие на карточку: %v", opened)
	}
	if len(*menus) != 1 {
		t.Fatal("нажатие на карточку показало меню")
	}

	test.TapSecondaryAt(card, fyne.NewPos(10, 10))
	if len(*menus) != 2 {
		t.Fatal("вторичное касание не показало меню")
	}
	if len(opened) != 1 {
		t.Fatal("вторичное касание открыло страницу")
	}
}

func TestGridRemove(t *testing.T) {
	test.NewTempApp(t)
	g := newGalleryGrid(thumbs.New(noOpen, 1<<20, 1), NewGridMetrics(storage.NewMemSettings()), allActions())
	g.SetItems([]model.Gallery{galleryOf("a.zip"), galleryOf("b.zip"), galleryOf("c.zip")})
	if !g.Remove(model.LocalKey("b.zip")) {
		t.Fatal("галерея не убрана")
	}
	if g.Remove(model.LocalKey("b.zip")) {
		t.Fatal("повторно убрана отсутствующая галерея")
	}
	var ids []string
	for _, it := range g.Items() {
		ids = append(ids, it.Key.ID)
	}
	if !slices.Equal(ids, []string{"a.zip", "c.zip"}) {
		t.Fatalf("сетка: %v", ids)
	}
}

var _ fyne.Tappable = (*galleryCard)(nil)
var _ fyne.SecondaryTappable = (*galleryCard)(nil)
var _ fyne.Tappable = (*widget.Button)(nil)

// «Сбросить прогресс» — перед разделителем и только при наличии прогресса.
func TestResetProgressItem(t *testing.T) {
	act := allActions()
	reset := ""
	act.ResetProgress = func(g model.Gallery) { reset = g.Key.ID }
	read := galleryOf("read.zip")
	act.HasProgress = func(g model.Gallery) bool { return g.Key == read.Key }

	want := labels("menu.copy_title", "menu.show_in_folder", "-", "menu.delete")
	if got := menuLabels(act.DetailsMenu(galleryOf("new.zip"))); !slices.Equal(got, want) {
		t.Fatalf("без прогресса: %v", got)
	}
	want = labels("menu.copy_title", "menu.show_in_folder", "menu.reset_progress", "-", "menu.delete")
	if got := menuLabels(act.DetailsMenu(read)); !slices.Equal(got, want) {
		t.Fatalf("страница с прогрессом: %v", got)
	}
	m := act.CardMenu(read)
	for _, it := range m.Items {
		if it.Label == i18n.T("menu.reset_progress") {
			it.Action()
		}
	}
	if reset != "read.zip" {
		t.Fatalf("пункт карточки сбросил %q", reset)
	}
}
