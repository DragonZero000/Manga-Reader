//go:build frameprobe

package ui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/devreport"
	"mangareader/internal/display"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/paths"
	"mangareader/internal/storage"
	"mangareader/internal/ui/frameclock"
	"mangareader/internal/ui/reader"
	"mangareader/internal/ui/screens"
)

// Сценарии замера кадров (design D2 изменения perf-and-layers): одинаковые
// до и после правок, на ПК и Android.
const (
	// probeSpeed — скорость автопрокрутки, логических px/с.
	probeSpeed = 1500
	// probeLeg — длительность прокрутки в одну сторону.
	probeLeg = 8 * time.Second
	// probeDensityLeg — то же для каждого значения плотности сетки.
	probeDensityLeg = 4 * time.Second
	// probeFlip — шаг листания постраничного режима.
	probeFlip = 300 * time.Millisecond
	// probeTab — шаг переключения вкладок.
	probeTab = 500 * time.Millisecond
	// probeSettle — пауза на подготовку сценария (загрузка, результаты поиска).
	probeSettle = 1500 * time.Millisecond
	// probeGap — пауза между сценариями.
	probeGap = time.Second
	// probeQuery — запрос сценария search: все галереи.
	probeQuery = "pages:>0"
	// probeRunEnv — переменная окружения: «1» — запустить тесты через
	// probeAutoDelay после показа окна, как кнопкой «Запустить тесты».
	probeRunEnv = "MANGAREADER_PROBE_RUN"
	// probeAutoDelay — пауза перед тестами по probeRunEnv (сканирование библиотеки).
	probeAutoDelay = 5 * time.Second
)

// devTools — раздел «Разработка» в настройках dev-сборки: тесты по кнопке,
// ручной замер, отчёт в файл, «Поделиться». Только UI-поток.
type devTools struct {
	s      *Shell
	run    *widget.Button
	manual *widget.Button
	share  *widget.Button // «Поделиться» (Android) или «Открыть папку» (ПК); nil — нет
	status *widget.Label
	// busy — идут тесты или ручной замер; recording — ручной замер
	busy, recording bool
	// report — текст последнего отчёта, file — его имя
	report, file string
}

// startProbe добавляет в «Настройки» раздел «Разработка». Сами тесты не
// запускаются: только по кнопке. Вызывается из UI-потока после показа окна.
func (s *Shell) startProbe() {
	d := &devTools{s: s}
	d.run = widget.NewButtonWithIcon(i18n.T("dev.run"), theme.MediaPlayIcon(), d.runTests)
	d.manual = widget.NewButtonWithIcon(i18n.T("dev.manual_start"), theme.MediaRecordIcon(), d.toggleManual)
	d.status = widget.NewLabel(i18n.T("dev.no_report"))
	d.status.Wrapping = fyne.TextWrapWord
	hint := widget.NewLabel(i18n.T("dev.hint", "Dir", d.place("")))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	items := []fyne.CanvasObject{d.run, d.manual}
	switch {
	case devreport.CanShare:
		d.share = widget.NewButtonWithIcon(i18n.T("dev.share"), theme.MailSendIcon(), d.shareReport)
	case storage.CanReveal:
		d.share = widget.NewButtonWithIcon(i18n.T("dev.open_folder"), theme.FolderOpenIcon(), d.openFolder)
	}
	if d.share != nil {
		d.share.Disable()
		items = append(items, d.share)
	}
	items = append(items, d.status, hint)
	s.settings.AddCard(widget.NewCard(i18n.T("dev.title"), "", container.NewVBox(items...)))
	log.Print("frameprobe: dev tools enabled")
	// Замер без нажатия кнопки (ПК, из командной строки): только если
	// переменная задана явно.
	if os.Getenv(probeRunEnv) == "1" {
		probeAfter(probeAutoDelay, d.runTests)
	}
}

// runTests выполняет сценарии один за другим; по окончании — отчёт и
// возврат на вкладку «Настройки».
func (d *devTools) runTests() {
	if d.busy {
		return
	}
	d.setBusy(true)
	d.status.SetText(i18n.T("dev.running"))
	startFrames(d.s.Window.Canvas())
	s := d.s
	steps := []func(next func()){
		s.probeGrid, s.probeSearch, s.probeDetails, s.probeSettings,
		s.probeStrip, s.probePaged, s.probeTabs, s.probeDensity,
	}
	var run func(i int)
	run = func(i int) {
		if i == len(steps) {
			log.Print("frameprobe: scenarios done")
			s.Tabs.SelectIndex(len(s.Tabs.Items) - 1)
			d.finish("tests", stopFrames())
			return
		}
		steps[i](func() { probeAfter(probeGap, func() { run(i + 1) }) })
	}
	probeAfter(probeGap, func() { run(0) })
}

// toggleManual начинает или заканчивает ручной замер: кадры пишутся, пока
// пользователь сам листает; при остановке — сводка «manual» и отчёт.
func (d *devTools) toggleManual() {
	if !d.recording {
		if d.busy {
			return
		}
		d.recording = true
		d.setBusy(true)
		d.manual.SetText(i18n.T("dev.manual_stop"))
		d.manual.SetIcon(theme.MediaStopIcon())
		d.status.SetText(i18n.T("dev.recording"))
		startFrames(d.s.Window.Canvas())
		probeBegin("manual", d.s.visibleOffset)
		return
	}
	d.recording = false
	probeEnd()
	d.manual.SetText(i18n.T("dev.manual_start"))
	d.manual.SetIcon(theme.MediaRecordIcon())
	d.finish("manual", stopFrames())
}

// setBusy блокирует кнопки на время тестов; во время ручного замера
// доступна только его кнопка.
func (d *devTools) setBusy(on bool) {
	d.busy = on
	for _, b := range []*widget.Button{d.run, d.manual, d.share} {
		if b == nil {
			continue
		}
		if (on && !(b == d.manual && d.recording)) || (b == d.share && d.report == "") {
			b.Disable()
		} else {
			b.Enable()
		}
	}
}

// finish собирает отчёт и сохраняет его в файл вне UI-потока.
func (d *devTools) finish(kind string, lines []string) {
	s := d.s
	st := s.svc.Settings
	c := s.Window.Canvas()
	now := time.Now()
	h := reportHeader{Kind: kind, Version: s.svc.Version, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Time: now, Width: c.Size().Width, Height: c.Size().Height, Scale: c.Scale(),
		Refresh: frameclock.RefreshRate(), Pacing: frameclock.Mode(), Present: frameclock.PresentSource(),
		Galleries: len(s.library.Galleries())}
	if display.Supported {
		h.Max60 = map[bool]string{true: "on", false: "off"}[app.DisplayMax60(st)]
	}
	if fyne.CurrentDevice().IsMobile() {
		h.Grid = fmt.Sprintf("%d columns", app.GridColumns(st))
	} else {
		h.Grid = "cards " + app.GridSize(st)
	}
	d.report, d.file = formatReport(h, lines), reportName(now)
	d.status.SetText(i18n.T("dev.saving"))
	dir, report, file := d.dir(), d.report, d.file
	go func() {
		err := devreport.Save(dir, file, report)
		fyne.Do(func() {
			d.setBusy(false)
			if err != nil {
				log.Printf("frameprobe: saving the report: %v", err)
				d.status.SetText(i18n.T("dev.save_failed", "Error", screens.ErrorText(err)))
				s.Toast.Show(i18n.T("dev.save_failed", "Error", screens.ErrorText(err)))
				return
			}
			d.status.SetText(i18n.T("dev.saved", "File", d.place(file)))
			s.Toast.Show(i18n.T("dev.done", "File", file))
		})
	}()
}

// dir — куда сохранять отчёты: Android — tree-URI папки библиотеки (папка
// .perf внутри), ПК — папка приложения (папка perf внутри).
func (d *devTools) dir() string {
	if devreport.CanShare {
		return d.s.svc.Settings.String(app.KeyLibraryTree, "")
	}
	dir, err := paths.AppDir()
	if err != nil {
		log.Printf("frameprobe: app folder: %v", err)
	}
	return dir
}

// place — путь отчёта file для показа (file == "" — папка отчётов).
func (d *devTools) place(file string) string {
	if devreport.CanShare {
		return d.s.svc.Library.Root() + "/" + devreport.DirName + "/" + file
	}
	return filepath.Join(d.dir(), devreport.DirName, file)
}

func (d *devTools) shareReport() {
	if d.report == "" {
		return
	}
	if err := devreport.Share("MangaReader "+d.file, d.report); err != nil {
		d.s.Toast.Show(i18n.T("dev.share_failed", "Error", screens.ErrorText(err)))
	}
}

func (d *devTools) openFolder() {
	if d.file == "" {
		return
	}
	if err := storage.Reveal(filepath.Join(d.dir(), devreport.DirName), d.file); err != nil {
		d.s.Toast.Show(i18n.T("dev.share_failed", "Error", screens.ErrorText(err)))
	}
}

// probeAfter выполняет f в UI-потоке через d.
func probeAfter(d time.Duration, f func()) {
	time.AfterFunc(d, func() { fyne.Do(f) })
}

// probeAnimate вызывает tick с прошедшим временем на каждом кадре в течение
// d, затем done.
func probeAnimate(d time.Duration, tick func(elapsed time.Duration), done func()) {
	start := frameclock.Now()
	a := fyne.NewAnimation(d, func(f float32) {
		tick(frameclock.Now().Sub(start))
		if f >= 1 {
			done()
		}
	})
	a.Curve = fyne.AnimationLinear
	a.Start()
}

// probeScroll прокручивает t вниз leg со скоростью probeSpeed, затем столько
// же вверх, и пишет сводку label.
func probeScroll(label string, t screens.ScrollTarget, leg time.Duration, done func()) {
	probeBegin(label, t.ScrollOffset)
	t.ScrollToOffset(0)
	turn := float32(-1) // смещение в момент разворота
	probeAnimate(2*leg, func(el time.Duration) {
		if el < leg {
			t.ScrollToOffset(probeSpeed * float32(el.Seconds()))
			return
		}
		if turn < 0 {
			turn = t.ScrollOffset()
		}
		t.ScrollToOffset(turn - probeSpeed*float32((el-leg).Seconds()))
	}, func() {
		probeEnd()
		done()
	})
}

func (s *Shell) probeGrid(next func()) {
	s.Tabs.SelectIndex(0)
	probeScroll("grid", s.library.Scroller(), probeLeg, next)
}

func (s *Shell) probeSearch(next func()) {
	s.Tabs.Select(s.searchTab)
	s.search.SetQuery(probeQuery)
	probeAfter(probeSettle, func() { probeScroll("search", s.search.Scroller(), probeLeg, next) })
}

func (s *Shell) probeDetails(next func()) {
	gs := s.library.Galleries()
	if len(gs) == 0 {
		next()
		return
	}
	s.Tabs.SelectIndex(0)
	s.Details.Open(gs[0])
	probeAfter(probeSettle, func() {
		probeScroll("details", s.Details.Scroller(), probeLeg, func() {
			s.Details.Close()
			next()
		})
	})
}

func (s *Shell) probeSettings(next func()) {
	s.Tabs.SelectIndex(len(s.Tabs.Items) - 1)
	probeScroll("settings", s.settings.Scroller(), probeLeg, next)
}

// probeGallery — галерея с наибольшим числом страниц.
func (s *Shell) probeGallery() (model.Gallery, bool) {
	gs := s.library.Galleries()
	if len(gs) == 0 {
		return model.Gallery{}, false
	}
	best := gs[0]
	for _, g := range gs {
		if len(g.Pages) > len(best.Pages) {
			best = g
		}
	}
	return best, true
}

// probeReader открывает читалку в режиме mode, после подготовки вызывает
// run; по окончании читалка закрывается, режим пользователя возвращается.
func (s *Shell) probeReader(mode string, run func(done func()), next func()) {
	g, ok := s.probeGallery()
	if !ok {
		next()
		return
	}
	prev := s.Reader.Mode()
	s.Reader.SetMode(mode)
	s.Reader.Open(g)
	probeAfter(probeSettle, func() {
		run(func() {
			s.Reader.Close()
			s.Reader.SetMode(prev)
			next()
		})
	})
}

func (s *Shell) probeStrip(next func()) {
	s.probeReader(reader.ModeStrip, func(done func()) {
		probeScroll("strip", s.Reader.StripScroller(), probeLeg, done)
	}, next)
}

func (s *Shell) probePaged(next func()) {
	s.probeReader(reader.ModePaged, func(done func()) {
		probeBegin("paged", nil)
		flips := 0
		probeAnimate(2*probeLeg, func(el time.Duration) {
			for ; flips < int(el/probeFlip); flips++ {
				k := fyne.KeyPageDown
				if time.Duration(flips)*probeFlip >= probeLeg {
					k = fyne.KeyPageUp
				}
				s.Reader.KeyDown(k)
				s.Reader.TypedKey(k)
			}
		}, func() {
			probeEnd()
			done()
		})
	}, next)
}

func (s *Shell) probeTabs(next func()) {
	probeBegin("tabs", nil)
	n := 0
	probeAnimate(probeLeg, func(el time.Duration) {
		for ; n < int(el/probeTab); n++ {
			i := (n + 1) % len(s.Tabs.Items)
			probeTabSwitch(i, func() { s.Tabs.SelectIndex(i) })
		}
	}, func() {
		probeEnd()
		s.Tabs.SelectIndex(0)
		next()
	})
}

// visibleOffset — смещение прокрутки того, что сейчас на экране (ручной
// замер): лента читалки, страница произведения или вкладка.
func (s *Shell) visibleOffset() float32 {
	switch {
	case s.Reader.Visible():
		if s.Reader.Mode() == reader.ModeStrip {
			return s.Reader.StripScroller().ScrollOffset()
		}
		return 0
	case s.Details.Visible():
		return s.Details.Scroller().ScrollOffset()
	}
	switch s.Tabs.Selected() {
	case s.searchTab:
		return s.search.Scroller().ScrollOffset()
	case s.settingsTab:
		return s.settings.Scroller().ScrollOffset()
	case s.Tabs.Items[0]:
		return s.library.Scroller().ScrollOffset()
	}
	return 0
}

// probeDensity прокручивает библиотеку при каждой плотности сетки (телефон —
// 2, 3, 4 колонки; ПК — размеры карточек) и возвращает настройку.
func (s *Shell) probeDensity(next func()) {
	st := s.svc.Settings
	s.Tabs.SelectIndex(0)
	var values []string
	var set func(v string)
	var restore func()
	if fyne.CurrentDevice().IsMobile() {
		prev := app.GridColumns(st)
		values = []string{"2", "3", "4"}
		set = func(v string) { n, _ := strconv.Atoi(v); app.SetGridColumns(st, n) }
		restore = func() { app.SetGridColumns(st, prev) }
	} else {
		prev := app.GridSize(st)
		values = []string{app.GridSizeSmall, app.GridSizeMedium, app.GridSizeLarge}
		set = func(v string) { app.SetGridSize(st, v) }
		restore = func() { app.SetGridSize(st, prev) }
	}
	var run func(i int)
	run = func(i int) {
		if i == len(values) {
			restore()
			s.grid.Reload()
			next()
			return
		}
		set(values[i])
		s.grid.Reload()
		probeAfter(probeSettle, func() {
			probeScroll("density-"+values[i], s.library.Scroller(), probeDensityLeg, func() { run(i + 1) })
		})
	}
	run(0)
}
