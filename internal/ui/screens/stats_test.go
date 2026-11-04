package screens

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/stats"
	"mangareader/internal/storage"
)

type fakeStatsSource struct{ gs []model.Gallery }

func (f *fakeStatsSource) Galleries() []model.Gallery { return f.gs }

func (f *fakeStatsSource) Get(k model.Key) (model.Gallery, bool) {
	for _, g := range f.gs {
		if g.Key == k {
			return g, true
		}
	}
	return model.Gallery{}, false
}

// sg — галерея: pages страниц, size байт, теги "тип:имя".
func sg(id string, pages int, size int64, tags ...string) model.Gallery {
	g := model.Gallery{
		Key: model.LocalKey(id), Title: id, HasMeta: true,
		File: model.FileInfo{Size: size, ModTime: time.Date(2026, 1, 15, 12, 0, 0, 0, time.Local)},
	}
	for i := 0; i < pages; i++ {
		g.Pages = append(g.Pages, model.Page{Name: fmt.Sprintf("%d.jpg", i+1)})
	}
	for _, t := range tags {
		g.AddTag(model.ParseTag(t))
	}
	return g
}

// statsEnv — экран статистики над галереями gs; UI-функции копятся в
// очереди, которую разбирает pump.
type statsEnv struct {
	*Stats
	t        *testing.T
	settings *storage.MemSettings
	src      *fakeStatsSource
	idx      *search.MemIndex
	q        chan func()
	searches []string
	opened   []model.Key
	rev      int
}

func newStatsEnv(t *testing.T, settings *storage.MemSettings, gs ...model.Gallery) *statsEnv {
	t.Helper()
	test.NewTempApp(t)
	e := &statsEnv{t: t, settings: settings, src: &fakeStatsSource{gs: gs}, idx: search.NewMemIndex(), q: make(chan func(), 1024)}
	e.reload(gs)
	w := test.NewTempWindow(t, nil)
	e.Stats = newStats(settings, e.src, e.idx, w,
		func(q string) { e.searches = append(e.searches, q) },
		func(g model.Gallery) { e.opened = append(e.opened, g.Key) })
	w.SetContent(e.Stats.Content())
	e.SetDispatcher(func(f func()) { e.q <- f })
	e.SetDelay(20 * time.Millisecond)
	e.SetRevision(func() int { return e.rev })
	return e
}

// reload заменяет библиотеку (как сканирование) и увеличивает номер изменения.
func (e *statsEnv) reload(gs []model.Gallery) {
	e.src.gs = gs
	for _, g := range gs {
		if err := e.idx.Upsert(context.Background(), g); err != nil {
			e.t.Fatal(err)
		}
	}
	e.rev++
}

func (e *statsEnv) pump() {
	for {
		select {
		case f := <-e.q:
			f()
		default:
			return
		}
	}
}

// settle ждёт, пока UI-функции закончатся и подсчёт не идёт.
func (e *statsEnv) settle() {
	e.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		e.pump()
		if !e.Counting() && len(e.q) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	e.t.Fatal("подсчёт не закончился")
}

// open считает статистику, как при открытии вкладки.
func (e *statsEnv) open() {
	e.Rerun()
	e.settle()
}

func authors(n int) []model.Gallery {
	var gs []model.Gallery
	for i := 0; i < n; i++ {
		gs = append(gs, sg(fmt.Sprintf("w%02d.zip", i), 10, 1<<20, fmt.Sprintf("artist:a%02d", i), "language:japanese"))
	}
	return gs
}

func TestStatsBlocksOrderAndSummary(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(3)...)
	if e.BlockVisible(stats.BlockSummary) {
		t.Fatal("до подсчёта блоки не показываются")
	}
	e.open()
	for _, id := range stats.AllBlocks() {
		if !e.BlockVisible(id) {
			t.Errorf("блок %s не показан", id)
		}
	}
	if e.Result().Summary.Works != 3 || e.MessageText() != "" {
		t.Errorf("сводка %+v, сообщение %q", e.Result().Summary, e.MessageText())
	}
	rows := e.RowLabels(stats.BlockSummary)
	if rows[0] != [2]string{"Произведений", "3"} {
		t.Errorf("первая строка сводки: %v", rows[0])
	}
}

func TestStatsBarsProportional(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 10; i++ {
		gs = append(gs, sg(fmt.Sprintf("a%d.zip", i), 5, 100, "artist:a"))
	}
	for i := 0; i < 5; i++ {
		gs = append(gs, sg(fmt.Sprintf("b%d.zip", i), 5, 100, "artist:b"))
	}
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	top := e.topOf("artist")
	if top.rows.list.Row(0).data.Frac != 1 || top.rows.list.Row(1).data.Frac != 0.5 {
		t.Errorf("полоски: %v и %v", top.rows.list.Row(0).data.Frac, top.rows.list.Row(1).data.Frac)
	}
	if got := top.rows.list.Row(1).value.Text; got != "5 (33%)" {
		t.Errorf("значение: %q", got)
	}
}

func TestStatsOverlayTags(t *testing.T) {
	own := sg("own.zip", 1, 1)
	own.Custom = []model.Tag{model.NewTag("artist", "alice")}
	hid := sg("hid.zip", 1, 1, "language:english")
	hid.Hidden = []model.Tag{model.NewTag("language", "english")}
	e := newStatsEnv(t, storage.NewMemSettings(), own, hid)
	e.open()
	if got := e.TagRows("artist"); len(got) != 1 || got[0] != "alice" {
		t.Errorf("авторы: %v", got)
	}
	if got := e.TagRows("language"); len(got) != 0 {
		t.Errorf("языки: %v", got)
	}
}

func TestStatsFilterAndPersistence(t *testing.T) {
	gs := authors(4)
	for i := 0; i < 6; i++ {
		gs = append(gs, sg(fmt.Sprintf("x%d.zip", i), 3, 10, "artist:x", "language:english"))
	}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	if e.Result().Summary.Works != 10 {
		t.Fatalf("без фильтра %d", e.Result().Summary.Works)
	}
	e.SetFilter("language:japanese")
	e.settle()
	if e.Result().Summary.Works != 4 {
		t.Errorf("с фильтром %d", e.Result().Summary.Works)
	}
	// перезапуск: поле и статистика по сохранённому фильтру
	e2 := newStatsEnv(t, settings, gs...)
	if e2.Filter() != "language:japanese" || e2.entry.Text != "language:japanese" {
		t.Errorf("после перезапуска фильтр %q, поле %q", e2.Filter(), e2.entry.Text)
	}
	e2.open()
	if e2.Result().Summary.Works != 4 {
		t.Errorf("после перезапуска %d", e2.Result().Summary.Works)
	}
	// исключение произведений
	e.SetFilter("-tag:anthology")
	e.settle()
	if e.Result().Summary.Works != 10 {
		t.Errorf("-tag:anthology: %d", e.Result().Summary.Works)
	}
}

func TestStatsFilterDebounce(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(3)...)
	e.open()
	seq := e.Seq()
	e.entry.SetText("language:j")
	e.entry.SetText("language:japanese")
	if e.Seq() != seq {
		t.Fatal("пересчёт без паузы ввода")
	}
	deadline := time.Now().Add(2 * time.Second)
	for e.Seq() == seq && time.Now().Before(deadline) {
		e.pump()
		time.Sleep(5 * time.Millisecond)
	}
	e.settle()
	if e.Seq() != seq+1 || e.Filter() != "language:japanese" {
		t.Errorf("запросов %d, фильтр %q", e.Seq()-seq, e.Filter())
	}
	// Enter — сразу
	e.entry.SetText("language:english")
	e.entry.OnSubmitted("language:english")
	e.settle()
	if e.Filter() != "language:english" {
		t.Errorf("по Enter фильтр %q", e.Filter())
	}
}

func TestStatsEmptyStates(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings())
	e.open()
	if e.MessageText() != "Библиотека пуста" || e.BlockVisible(stats.BlockSummary) {
		t.Errorf("пустая библиотека: %q", e.MessageText())
	}

	e = newStatsEnv(t, storage.NewMemSettings(), authors(2)...)
	e.SetFilter("artist:nobody")
	e.settle()
	if e.MessageText() != "По фильтру ничего не найдено" || !e.ResetButton().Visible() || e.BlockVisible(stats.BlockSummary) {
		t.Errorf("ничего не найдено: %q, кнопка %v", e.MessageText(), e.ResetButton().Visible())
	}
	test.Tap(e.ResetButton())
	e.settle()
	if e.Filter() != "" || e.MessageText() != "" || !e.BlockVisible(stats.BlockSummary) {
		t.Errorf("после сброса: фильтр %q, сообщение %q", e.Filter(), e.MessageText())
	}

	e.SetFilter("pages:>много")
	e.settle()
	if m := e.MessageText(); !strings.Contains(m, "pages:>много") || e.BlockVisible(stats.BlockSummary) {
		t.Errorf("ошибка разбора: %q", m)
	}
	if e.Result().Summary == nil {
		t.Error("прежний результат потерян")
	}
}

func TestStatsTopFiveAndShowAll(t *testing.T) {
	gs := authors(25)
	gs = append(gs, sg("yo.zip", 1, 1, "tag:ежик"), sg("hedgehog.zip", 1, 1, "tag:ёлка"), sg("other.zip", 1, 1, "tag:cat"))
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	if got := len(e.TagRows("artist")); got != 5 || !e.ShowAllVisible("artist") {
		t.Fatalf("топ авторов: %d строк, «Показать все» %v", got, e.ShowAllVisible("artist"))
	}
	if e.ShowAllVisible("tag") {
		t.Error("у топа тегов из трёх строк нет «Показать все»")
	}
	e.ShowAll("artist")
	if e.topOf("artist").rows.fullBox.Visible() != true || e.topOf("artist").rows.list.box.Visible() {
		t.Fatal("после «Показать все» должен быть виден полный список")
	}
	if got := len(e.FullRows("artist")); got != 25 {
		t.Errorf("полный список: %d", got)
	}
	e.FindInList("artist", "a1")
	if got := e.FullRows("artist"); len(got) != 10 { // a10…a19
		t.Errorf("поиск a1: %v", got)
	}
	e.FindInList("artist", "A07")
	if got := e.FullRows("artist"); len(got) != 1 || got[0] != "a07" {
		t.Errorf("поиск A07: %v", got)
	}
}

func TestStatsEmptyTopShowsNoData(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), sg("a.zip", 1, 1, "artist:a"))
	e.open()
	if !e.topOf("group").rows.list.empty.Visible() {
		t.Error("пустой топ групп должен показывать «нет данных»")
	}
	if e.topOf("artist").rows.list.empty.Visible() {
		t.Error("у непустого топа авторов «нет данных» не показывается")
	}
}

func TestStatsFindInListNormalizes(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 12; i++ {
		gs = append(gs, sg(fmt.Sprintf("t%d.zip", i), 1, 1, fmt.Sprintf("tag:t%02d", i)))
	}
	gs = append(gs, sg("e1.zip", 1, 1, "tag:ежик"), sg("e2.zip", 1, 1, "tag:ёлка"), sg("e3.zip", 1, 1, "tag:cat"))
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	e.ShowAll("tag")
	e.FindInList("tag", "ёж")
	if got := e.FullRows("tag"); len(got) != 1 || got[0] != "ежик" {
		t.Errorf("ёж: %v", got)
	}
	e.FindInList("tag", "ел")
	if got := e.FullRows("tag"); len(got) != 1 || got[0] != "ёлка" {
		t.Errorf("ел: %v", got)
	}
}

func TestStatsModeSwitchPersists(t *testing.T) {
	gs := []model.Gallery{
		sg("1.zip", 5, 10<<20, "artist:a"), sg("2.zip", 5, 10<<20, "artist:a"), sg("3.zip", 5, 10<<20, "artist:a"),
		sg("4.zip", 5, 50<<20, "artist:b"),
	}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	if got := e.TagRows("artist"); got[0] != "a" {
		t.Fatalf("по произведениям: %v", got)
	}
	e.SelectMode("artist", stats.ModeSize)
	e.settle()
	if got := e.TagRows("artist"); got[0] != "b" {
		t.Errorf("по объёму: %v", got)
	}
	if v := e.topOf("artist").rows.list.Row(0).value.Text; !strings.HasPrefix(v, "50,00 МБ") {
		t.Errorf("значение в режиме объёма: %q", v)
	}
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if got := e2.TagRows("artist"); got[0] != "b" || e2.topOf("artist").mode.Selected != "Объём" {
		t.Errorf("после перезапуска: %v, переключатель %q", got, e2.topOf("artist").mode.Selected)
	}
}

func TestStatsGoSearch(t *testing.T) {
	gs := authors(3)
	gs = append(gs, sg("big.zip", 600, 30<<20+300<<10, "artist:z"), sg("bare.zip", 5, 7<<20))
	gs[len(gs)-1].HasMeta = false
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.SetFilter("-tag:none")
	e.settle()
	e.TapTagRow("artist", 0)
	if got := e.searches; len(got) != 1 || got[0] != `-tag:none artist:"a00"` {
		t.Errorf("запрос тега: %v", got)
	}
	e.searches = nil
	e.ClearFilter()
	e.settle()
	rows := e.RowLabels(stats.BlockSize)
	if rows[1][0] != "5–10" {
		t.Fatalf("корзины объёма: %v", rows)
	}
	e.TapRow(stats.BlockSize, 1)
	e.TapRow(stats.BlockPages, 5) // больше 500
	e.TapRow(stats.BlockQuality, 1)
	want := []string{"size:>5mb size:<=10mb", "pages:>500", "missing:artist"}
	if fmt.Sprint(e.searches) != fmt.Sprint(want) {
		t.Errorf("запросы: %q, ожидалось %q", e.searches, want)
	}
	e.TapLargest(0)
	if len(e.opened) != 1 || e.opened[0] != model.LocalKey("big.zip") {
		t.Errorf("самое большое: %v", e.opened)
	}
}

func TestStatsExcludeTag(t *testing.T) {
	gs := []model.Gallery{
		sg("1.zip", 1, 1, "language:japanese", "language:translated"),
		sg("2.zip", 1, 1, "language:japanese"),
	}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	if got := e.TagRows("language"); len(got) != 2 || e.Result().Summary.Unique["language"] != 2 {
		t.Fatalf("языки: %v", got)
	}
	e.ExcludeTagRow("language", 1) // translated
	e.settle()
	if got := e.TagRows("language"); len(got) != 1 || got[0] != "japanese" {
		t.Errorf("после исключения: %v", got)
	}
	if e.Result().Summary.Unique["language"] != 1 || e.Result().Summary.Works != 2 {
		t.Errorf("сводка: %+v", e.Result().Summary)
	}
	if chips := e.ChipTexts(); len(chips) != 1 || chips[0] != "language:translated — Языки" {
		t.Errorf("чипы: %q", chips)
	}
	// временное исключение живёт до перезапуска
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if got := e2.TagRows("language"); len(got) != 2 || e2.Excluded() != 0 {
		t.Errorf("после перезапуска: %v", got)
	}
	// возврат
	e.TapChip(0)
	e.settle()
	if got := e.TagRows("language"); len(got) != 2 || e.Excluded() != 0 || len(e.ChipTexts()) != 0 {
		t.Errorf("после возврата: %v", got)
	}
}

func TestStatsExcludeOnlyInOwnBlock(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 3; i++ {
		g := sg(fmt.Sprintf("%d.zip", i), 1, 1)
		g.Custom = []model.Tag{model.NewTag("tag", "fav")}
		gs = append(gs, g)
	}
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	e.ExcludeTagRow("tag", 0)
	e.settle()
	if got := e.TagRows("tag"); len(got) != 0 {
		t.Errorf("топ tag: %v", got)
	}
	if got := e.TagRows(stats.ExcludeCustom); len(got) != 1 || got[0] != "tag:fav" {
		t.Errorf("«Ваши теги»: %v", got)
	}
	e.ExcludeTagRow(stats.ExcludeCustom, 0)
	e.settle()
	if got := e.TagRows(stats.ExcludeCustom); len(got) != 0 {
		t.Errorf("после исключения в «Ваших»: %v", got)
	}
	if chips := e.ChipTexts(); len(chips) != 2 || chips[1] != "tag:fav — Ваши теги: свои" {
		t.Errorf("чипы: %q", chips)
	}
}

func TestStatsConfigBlocksAndBounds(t *testing.T) {
	gs := []model.Gallery{sg("a.zip", 8, 2<<20+800<<10, "character:c"), sg("b.zip", 30, 5<<20), sg("c.zip", 90, 30<<20)}
	settings := storage.NewMemSettings()
	e := newStatsEnv(t, settings, gs...)
	e.open()
	e.showConfig()
	if len(e.lastConfig.checks) != 14 {
		t.Fatalf("галочек %d", len(e.lastConfig.checks))
	}
	e.ConfigCheck(stats.BlockID("character")).SetChecked(false)
	e.ConfigSize().SetText("1, 2.5, 10")
	if e.ConfigSizeError() != "" {
		t.Errorf("ошибка для верных границ: %q", e.ConfigSizeError())
	}
	// неверные границы страниц: ошибка под полем, прежние остаются
	e.ConfigPages().SetText("50, 20")
	if e.ConfigPagesError() == "" {
		t.Error("для «50, 20» нет ошибки")
	}
	e.CloseConfig()
	e.settle()
	if e.BlockVisible("character") || e.Result().Tops["character"] != nil || !e.BlockVisible("artist") {
		t.Error("блок «Персонажи» не скрыт или скрыт лишний")
	}
	var labels []string
	for _, r := range e.RowLabels(stats.BlockSize) {
		labels = append(labels, r[0])
	}
	if want := "до 1 1–2,5 2,5–10 больше 10"; strings.Join(labels, " ") != want {
		t.Errorf("корзины объёма: %v", labels)
	}
	pages := e.RowLabels(stats.BlockPages)
	if pages[0][0] != "1–20" { // прежние границы по умолчанию
		t.Errorf("страницы: %v", pages)
	}
	// после перезапуска блок по-прежнему скрыт, границы те же
	e2 := newStatsEnv(t, settings, gs...)
	e2.open()
	if e2.BlockVisible("character") || len(e2.RowLabels(stats.BlockSize)) != 4 {
		t.Error("настройки не сохранились")
	}
	// «По умолчанию»
	e2.showConfig()
	e2.ConfigDefaultPages()
	if e2.ConfigPages().Text != "20, 50, 100, 200, 500" {
		t.Errorf("по умолчанию: %q", e2.ConfigPages().Text)
	}
}

func TestStatsAllBlocksHidden(t *testing.T) {
	settings := storage.NewMemSettings()
	off := map[stats.BlockID]bool{}
	for _, id := range stats.AllBlocks() {
		off[id] = true
	}
	app.SetStatsDisabled(settings, off)
	e := newStatsEnv(t, settings, authors(2)...)
	e.open()
	if !e.HintVisible() {
		t.Error("нет подсказки, когда все блоки выключены")
	}
}

func TestStatsRecomputeOnLibraryChange(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(2)...)
	e.open()
	seq := e.Seq()
	e.Rerun()
	e.settle()
	if e.Seq() != seq {
		t.Fatal("без изменений статистика пересчитана")
	}
	e.reload(authors(3)) // новый архив
	e.Rerun()
	e.settle()
	if e.Seq() != seq+1 || e.Result().Summary.Works != 3 {
		t.Errorf("запросов %d, произведений %d", e.Seq()-seq, e.Result().Summary.Works)
	}
}

// Результат не последнего запроса не показывается.
func TestStatsStaleResultDropped(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(2)...)
	e.open()
	cur := e.Seq()
	e.apply(app.StatsResponse{Seq: cur - 1, Library: 99, Result: stats.Result{Works: 99, Summary: &stats.Summary{Works: 99}}})
	if e.Result().Summary.Works != 2 {
		t.Error("показан результат устаревшего запроса")
	}
}

func TestStatsCountingIndicator(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), authors(2)...)
	e.Rerun()
	if !e.Counting() {
		t.Error("индикатор подсчёта не показан")
	}
	e.settle()
	if e.Counting() {
		t.Error("индикатор остался после подсчёта")
	}
	// прежние цифры остаются, пока считается новое
	e.SetFilter("artist:a00")
	if e.Result().Summary.Works != 2 || !e.BlockVisible(stats.BlockSummary) {
		t.Error("прежние цифры пропали во время подсчёта")
	}
	e.settle()
}

func TestStatsGrowthYearsExpand(t *testing.T) {
	var gs []model.Gallery
	for _, d := range [][2]int{{2022, 5}, {2024, 3}, {2026, 11}} {
		g := sg(fmt.Sprintf("%d-%d.zip", d[0], d[1]), 1, 1)
		g.File.ModTime = time.Date(d[0], time.Month(d[1]), 10, 12, 0, 0, 0, time.Local)
		gs = append(gs, g)
	}
	e := newStatsEnv(t, storage.NewMemSettings(), gs...)
	e.open()
	rows := e.RowLabels(stats.BlockGrowth)
	if len(rows) != 5 || rows[0][0] != "2022" || rows[4][0] != "2026" {
		t.Fatalf("годы: %v", rows)
	}
	e.ToggleGrowthYear(2) // 2024
	rows = e.RowLabels(stats.BlockGrowth)
	if len(rows) != 17 || rows[3][0] != "2024-01" || rows[5][1] != "1 (33%)" {
		t.Errorf("после раскрытия 2024: %d строк, %v", len(rows), rows[3:6])
	}
	e.TapRow(stats.BlockGrowth, 5) // март 2024
	if len(e.searches) != 1 || e.searches[0] != "added:2024-03" {
		t.Errorf("запрос месяца: %v", e.searches)
	}
	e.ToggleGrowthYear(2)
	if got := len(e.RowLabels(stats.BlockGrowth)); got != 5 {
		t.Errorf("после сворачивания %d строк", got)
	}
}

func TestStatsUploadedNoDateNotTappable(t *testing.T) {
	a, b := sg("a.zip", 1, 1), sg("b.zip", 1, 1)
	a.Uploaded = time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	e := newStatsEnv(t, storage.NewMemSettings(), a, b)
	e.open()
	rows := e.RowLabels(stats.BlockUploaded)
	if len(rows) != 2 || rows[0][0] != "2020" || rows[1][0] != "Без даты" {
		t.Fatalf("годы: %v", rows)
	}
	e.TapRow(stats.BlockUploaded, 1)
	if len(e.searches) != 0 {
		t.Errorf("«Без даты» открыла поиск: %v", e.searches)
	}
	e.TapRow(stats.BlockUploaded, 0)
	if len(e.searches) != 1 || e.searches[0] != "uploaded:2020" {
		t.Errorf("запрос года: %v", e.searches)
	}
}

func TestStatsRowAppearsEmptyBucketsShown(t *testing.T) {
	e := newStatsEnv(t, storage.NewMemSettings(), sg("a.zip", 8, 1), sg("b.zip", 600, 1))
	e.open()
	rows := e.RowLabels(stats.BlockPages)
	if len(rows) != 6 || rows[1][1] != "0 (0%)" || rows[5][0] != "больше 500" {
		t.Errorf("корзины страниц: %v", rows)
	}
}

func TestPercentAndBucketLabels(t *testing.T) {
	for share, want := range map[float64]string{0: "0%", 0.005: "0,5%", 0.0999: "10%", 0.333: "33%", 1: "100%"} {
		if got := percent(share); got != want {
			t.Errorf("percent(%v) = %q, want %q", share, got, want)
		}
	}
	for _, c := range []struct {
		ref  stats.RowRef
		want string
	}{
		{stats.RowRef{Kind: stats.RowPages, Hi: 20}, "1–20"},
		{stats.RowRef{Kind: stats.RowPages, Lo: 20, Hi: 21}, "21"},
		{stats.RowRef{Kind: stats.RowPages, Lo: 500}, "больше 500"},
		{stats.RowRef{Kind: stats.RowSize, Hi: 2.5}, "до 2,5"},
		{stats.RowRef{Kind: stats.RowSize, Lo: 2.5, Hi: 10}, "2,5–10"},
	} {
		if got := bucketLabel(c.ref); got != c.want {
			t.Errorf("bucketLabel(%+v) = %q, want %q", c.ref, got, c.want)
		}
	}
}
