// Package reader — читалка: полноэкранный слой поверх вкладок с постраничным
// режимом и лентой. Всё состояние меняется только в UI-потоке.
package reader

import (
	"log"
	"runtime/debug"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/pages"
	"mangareader/internal/storage"
)

// Режимы чтения (значения хранятся в Preferences).
const (
	ModePaged = "paged"
	ModeStrip = "strip"

	prefMode = "reader.mode"
)

// Лимиты кэша страниц.
const (
	cacheDesktop = 160 << 20
	cacheMobile  = 80 << 20
	// maxTexture — наибольшая сторона изображения в постраничном режиме
	// (физ. px); ниже типичного ограничения размера текстуры GPU.
	maxTexture = 4096
)

// view — общий интерфейс постраничного вида и ленты.
type view interface {
	fyne.CanvasObject
	// show открывает галерею на странице page (sizes может быть nil).
	show(g model.Gallery, page int)
	// goTo переходит к странице.
	goTo(page int)
	// setSizes передаёт размеры страниц, когда они загружены.
	setSizes(sizes []library.PageSize)
	// typedKey обрабатывает клавиши навигации.
	typedKey(k fyne.KeyName)
	// reset освобождает изображения.
	reset()
}

// Reader — слой читалки.
type Reader struct {
	app      fyne.App
	win      fyne.Window
	src      *library.Source
	loader   *pages.Loader
	notify   func(string)
	settings storage.Settings
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах,
	// где тестовый драйвер Fyne не сериализует вызовы.
	do func(func())

	g       model.Gallery
	sizes   []library.PageSize
	cur     int
	mode    string
	visible bool
	token   int // номер открытия: отбрасывает результаты от прошлой галереи

	paged *pagedView
	strip *stripView
	panel *panel
	views *fyne.Container
	layer *fyne.Container
}

// New создаёт скрытый слой читалки; режим хранится в settings.
func New(a fyne.App, win fyne.Window, src *library.Source, settings storage.Settings, notify func(string)) *Reader {
	limit := int64(cacheDesktop)
	if fyne.CurrentDevice().IsMobile() {
		limit = cacheMobile
	}
	r := &Reader{
		app:      a,
		win:      win,
		src:      src,
		loader:   pages.NewLoader(src.OpenPage, limit, 2),
		notify:   notify,
		do:       fyne.Do,
		settings: settings,
		mode:     settings.String(prefMode, ModePaged),
	}
	if r.mode != ModePaged && r.mode != ModeStrip {
		r.mode = ModePaged
	}
	r.paged = newPagedView(r)
	r.strip = newStripView(r)
	r.panel = newPanel(r)

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	r.views = container.NewStack(r.paged, r.strip)
	r.layer = container.NewStack(bg, r.views, r.panel.layer)
	r.layer.Hide()
	return r
}

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (r *Reader) SetDispatcher(do func(func())) { r.do = do }

// Layer — объект для размещения в Stack оболочки.
func (r *Reader) Layer() fyne.CanvasObject { return r.layer }

// Visible сообщает, открыта ли читалка.
func (r *Reader) Visible() bool { return r.visible }

// Open открывает галерею с первой страницы.
func (r *Reader) Open(g model.Gallery) {
	r.token++
	r.g = g
	r.sizes = nil
	r.cur = 0
	r.visible = true
	r.win.Canvas().Unfocus()
	r.loader.NewGeneration()

	r.panel.setGallery(g)
	r.panel.hide()
	r.layer.Show()
	r.repaint(r.layer)
	r.applyMode()

	// размеры страниц нужны ленте; читаем заголовки в фоне
	token := r.token
	go func() {
		sizes, err := r.src.PageSizes(g.Key)
		r.do(func() {
			if token != r.token || !r.visible {
				return
			}
			if err != nil {
				log.Printf("читалка: размеры страниц %s: %v", g.Key, err)
				sizes = make([]library.PageSize, len(g.Pages))
				for i := range sizes {
					sizes[i].Err = err
				}
			}
			r.sizes = sizes
			r.strip.setSizes(sizes)
			r.paged.setSizes(sizes)
		})
	}()
}

// Close закрывает читалку и освобождает память страниц.
func (r *Reader) Close() {
	if !r.visible {
		return
	}
	r.token++
	r.visible = false
	r.layer.Hide()
	r.paged.reset()
	r.strip.reset()
	r.loader.Clear()
	r.g = model.Gallery{}
	r.sizes = nil
	// вернуть освобождённую память системе сразу, а не по таймеру сборщика
	go debug.FreeOSMemory()
}

// TypedKey обрабатывает клавиши навигации открытой читалки.
func (r *Reader) TypedKey(k fyne.KeyName) {
	if r.visible {
		r.current().typedKey(k)
	}
}

func (r *Reader) current() view {
	if r.mode == ModeStrip {
		return r.strip
	}
	return r.paged
}

// setMode переключает режим с сохранением текущей страницы.
func (r *Reader) setMode(mode string) {
	if mode == r.mode || (mode != ModePaged && mode != ModeStrip) {
		return
	}
	r.current().reset()
	r.mode = mode
	r.settings.SetString(prefMode, mode)
	r.loader.NewGeneration()
	r.applyMode()
}

func (r *Reader) applyMode() {
	if r.mode == ModeStrip {
		r.paged.Hide()
		r.strip.Show()
	} else {
		r.strip.Hide()
		r.paged.Show()
	}
	r.repaint(r.views)
	r.panel.setMode(r.mode)
	r.current().show(r.g, r.cur)
	if r.sizes != nil {
		r.current().setSizes(r.sizes)
	}
}

// pageChanged вызывается видом при смене текущей страницы.
func (r *Reader) pageChanged(page int) {
	r.cur = page
	r.panel.setPage(page, len(r.g.Pages))
}

// goTo — переход к странице из панели.
func (r *Reader) goTo(page int) {
	if page < 0 || page >= len(r.g.Pages) {
		return
	}
	r.current().goTo(page)
}

// togglePanel показывает или скрывает панель.
func (r *Reader) togglePanel() { r.panel.toggle() }

// repaint перерисовывает объект через canvas окна. Container.Show/Refresh
// не достаточно: объект, скрытый с момента создания, ещё не известен
// драйверу, и его собственный Refresh не доходит до canvas.
func (r *Reader) repaint(o fyne.CanvasObject) {
	o.Refresh()
	r.win.Canvas().Refresh(o)
}

// scale — физических пикселей на логическую единицу.
func (r *Reader) scale() float32 {
	if s := r.win.Canvas().Scale(); s > 0 {
		return s
	}
	return 1
}

// viewSize — размер области читалки в логических единицах.
func (r *Reader) viewSize() fyne.Size {
	if s := r.layer.Size(); s.Width > 0 && s.Height > 0 {
		return s
	}
	return r.win.Canvas().Size()
}

// physical переводит логический размер в физические пиксели с ограничением.
func (r *Reader) physical(v float32, limit int) int {
	px := int(v*r.scale() + 0.5)
	if limit > 0 {
		px = min(px, limit)
	}
	return max(1, px)
}
