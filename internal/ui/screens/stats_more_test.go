package screens

import (
	"fmt"
	"reflect"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"mangareader/internal/model"
	"mangareader/internal/stats"
	"mangareader/internal/storage"
)

// confirmWith подменяет диалог подтверждения ответом ok и считает вызовы.
func confirmWith(t *testing.T, ok bool) *int {
	n := new(int)
	prev := statsConfirm
	statsConfirm = func(_, _, _ string, cb func(bool), _ fyne.Window) {
		*n++
		cb(ok)
	}
	t.Cleanup(func() { statsConfirm = prev })
	return n
}

func TestStatsPermanentExclusion(t *testing.T) {
	gs := []model.Gallery{sg("1.zip", 1, 1, "artist:x"), sg("2.zip", 1, 1, "artist:y")}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()

	// отмена ничего не меняет
	asked := confirmWith(t, false)
	e.ExcludeTagRowForever("artist", 0)
	e.settle()
	if *asked != 1 || len(e.TagRows("artist")) != 2 || e.Excluded() != 0 {
		t.Fatalf("после отмены: %v, исключено %d", e.TagRows("artist"), e.Excluded())
	}

	confirmWith(t, true)
	e.ExcludeTagRowForever("artist", 0) // x
	e.settle()
	if got := e.TagRows("artist"); len(got) != 1 || got[0] != "y" {
		t.Errorf("после переноса: %v", got)
	}
	if len(e.ChipTexts()) != 0 {
		t.Errorf("постоянные исключения не висят сверху: %v", e.ChipTexts())
	}
	if got := e.Permanent(); !reflect.DeepEqual(got, []string{"artist:x — Авторы"}) {
		t.Errorf("постоянные: %v", got)
	}
	// после перезапуска исключено
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if got := e2.TagRows("artist"); len(got) != 1 {
		t.Errorf("после перезапуска: %v", got)
	}
	// снятие в «Настроить» без подтверждения
	e2.showConfig()
	if got := e2.ConfigPermanent(); !reflect.DeepEqual(got, []string{"artist:x — Авторы"}) {
		t.Fatalf("в «Настроить»: %v", got)
	}
	if e2.PermanentOpen() || e2.PermanentTitle() != "Постоянные исключения (1)" {
		t.Errorf("раздел свёрнут, в заголовке число: %q открыт=%v", e2.PermanentTitle(), e2.PermanentOpen())
	}
	e2.RemovePermanent(0)
	if e2.PermanentTitle() != "Постоянные исключения (0)" {
		t.Errorf("заголовок после снятия: %q", e2.PermanentTitle())
	}
	e2.CloseConfig()
	e2.settle()
	if got := e2.TagRows("artist"); len(got) != 2 || len(e2.Permanent()) != 0 {
		t.Errorf("после снятия: %v", got)
	}
}

func TestStatsChipForeverAndCollapse(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 6; i++ {
		gs = append(gs, sg(fmt.Sprintf("%d.zip", i), 1, 1, fmt.Sprintf("tag:t%d", i)))
	}
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	for i := 0; i < 3; i++ {
		e.ExcludeTagRow("tag", 0)
		e.settle()
	}
	if len(e.ChipTexts()) != 3 || e.ChipsHeader() != "" {
		t.Fatalf("три исключения — чипы без свёртки: %v / %q", e.ChipTexts(), e.ChipsHeader())
	}
	e.ExcludeTagRow("tag", 0)
	e.settle()
	if e.ChipsHeader() != "Исключено: 4" || len(e.ChipTexts()) != 0 {
		t.Fatalf("четыре — свёрнуто: %q, %v", e.ChipsHeader(), e.ChipTexts())
	}
	e.TapChipsHeader()
	if len(e.ChipTexts()) != 4 {
		t.Fatalf("раскрыто: %v", e.ChipTexts())
	}
	e.TapChipsHeader()
	if len(e.ChipTexts()) != 0 {
		t.Fatal("повторное нажатие сворачивает")
	}
	// «Навсегда» у чипа — с подтверждением
	e.TapChipsHeader()
	asked := confirmWith(t, true)
	e.TapChipForever(0)
	e.settle()
	if *asked != 1 || e.ChipsHeader() != "" || len(e.ChipTexts()) != 3 || len(e.Permanent()) != 1 {
		t.Errorf("после «Навсегда»: %q %v %v", e.ChipsHeader(), e.ChipTexts(), e.Permanent())
	}
}

func TestStatsTopFiveAndFullHeight(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(7)...)
	e.open()
	if got := len(e.TagRows("artist")); got != 5 || !e.ShowAllVisible("artist") {
		t.Fatalf("топ-5: %d", got)
	}
	l := e.topOf("artist").rows
	collapsed := l.box.MinSize().Height
	e.ShowAll("artist")
	full := l.fullH.MinSize().Height
	row := newStatRow().MinSize().Height
	if full < row*7 || full > row*8 {
		t.Errorf("высота раскрытого списка на 7 строк: %v (строка %v)", full, row)
	}
	if l.box.MinSize().Height < collapsed {
		t.Errorf("раскрытый блок ниже свёрнутого: %v < %v", l.box.MinSize().Height, collapsed)
	}
	e.FindInList("artist", "a03")
	if l.fullH.MinSize().Height != full {
		t.Error("высота меняется при поиске в списке")
	}
}

func TestStatsOrderToggle(t *testing.T) {
	gs := []model.Gallery{sg("1.zip", 1, 1, "artist:a"), sg("2.zip", 1, 1, "artist:a"), sg("3.zip", 1, 1, "artist:b")}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	e.TapOrder("artist")
	e.settle()
	if got := e.TagRows("artist"); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Errorf("по возрастанию: %v", got)
	}
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if got := e2.TagRows("artist"); got[0] != "b" {
		t.Errorf("порядок не сохранился: %v", got)
	}
}

func TestStatsLargestByPagesAndOrder(t *testing.T) {
	gs := []model.Gallery{sg("short.zip", 8, 30<<20), sg("long.zip", 110, 20<<20)}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	if got := e.LargestRows(); got[0][0] != "short.zip" || got[0][1] != "30,00 МБ" {
		t.Fatalf("по объёму: %v", got)
	}
	e.SelectLargestBy(stats.ModePages)
	e.settle()
	if got := e.LargestRows(); got[0][0] != "long.zip" || got[0][1] != "110" {
		t.Errorf("по страницам: %v", got)
	}
	e.TapLargestOrder()
	e.settle()
	if got := e.LargestRows(); got[0][0] != "short.zip" {
		t.Errorf("по возрастанию: %v", got)
	}
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if got := e2.LargestRows(); got[0][0] != "short.zip" || got[0][1] != "8" {
		t.Errorf("после перезапуска: %v", got)
	}
}

func TestStatsSummaryFields(t *testing.T) {
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, sg("a.zip", 10, 1363148800))
	e.open()
	rows := e.RowLabels(stats.BlockSummary)
	if rows[2][0] != "Общий объём" || rows[2][1] != "1,27 ГБ" {
		t.Errorf("объём с двумя знаками: %v", rows[2])
	}
	e.showConfig()
	if !e.SummaryFieldsVisible() {
		t.Fatal("«Поля сводки» видны при включённой Сводке")
	}
	e.ConfigSummaryField(stats.SummaryMedianPages).SetChecked(false)
	e.ConfigCheck(stats.BlockSummary).SetChecked(false)
	if e.SummaryFieldsVisible() {
		t.Error("«Поля сводки» скрыты при выключенной Сводке")
	}
	e.ConfigCheck(stats.BlockSummary).SetChecked(true)
	e.CloseConfig()
	e.settle()
	for _, r := range e.RowLabels(stats.BlockSummary) {
		if r[0] == "Медиана числа страниц" {
			t.Error("скрытое поле показано")
		}
	}
	// все поля скрыты — блока нет
	e.showConfig()
	for _, f := range stats.SummaryFields() {
		e.ConfigSummaryField(f).SetChecked(false)
	}
	e.CloseConfig()
	e.settle()
	if e.BlockVisible(stats.BlockSummary) {
		t.Error("Сводка без полей не показывается")
	}
}

func TestStatsRowHighlightWhileMenuOpen(t *testing.T) {
	var closed func()
	prev := showRowMenu
	showRowMenu = func(_ *fyne.Menu, _ fyne.Canvas, _ fyne.Position, c func()) { closed = c }
	t.Cleanup(func() { showRowMenu = prev })

	e := newStatsEnv(t, storage.NewMemSettings(), sg("1.zip", 1, 1, "artist:x"))
	e.open()
	r := e.topOf("artist").rows.list.Row(0)
	r.TappedSecondary(&fyne.PointEvent{})
	if !r.menuOpen || r.hover.FillColor == nil || closed == nil {
		t.Fatal("строка подсвечена, пока меню открыто")
	}
	closed()
	if r.menuOpen {
		t.Error("после закрытия меню подсветка снята")
	}
}

func TestStatsModeSelectFitsLongestOption(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), sg("1.zip", 1, 1, "artist:x"))
	e.open()
	sel := e.topOf("artist").mode
	need := fyne.MeasureText("Произведения", theme.TextSize(), fyne.TextStyle{}).Width
	if sel.MinSize().Width < need {
		t.Errorf("переключатель уже самого длинного варианта: %v < %v", sel.MinSize().Width, need)
	}
}
