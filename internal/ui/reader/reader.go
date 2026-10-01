// Package reader — читалка: полноэкранный слой поверх вкладок с постраничным
// режимом и лентой. Всё состояние меняется только в UI-потоке.
package reader

import (
	"log"
	"runtime/debug"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
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
	// maxTextureMobile — то же на телефоне: увеличенная страница 4096² (64 МБ)
	// вытеснила бы весь кэш.
	maxTextureMobile = 3072
)

const (
	// flipInterval — наименьший интервал между перелистываниями колесом
	// и автоповтором зажатой клавиши.
	flipInterval = 150 * time.Millisecond
	// finishGuard — пауза перед подтверждением завершения: серия событий
	// колеса или автоповтор не закрывают читалку.
	finishGuard = 400 * time.Millisecond
	// finishTTL — сколько ждать подтверждения завершения после попытки.
	finishTTL = 3 * time.Second
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
	// typedKey обрабатывает клавиши навигации; repeat — автоповтор
	// зажатой клавиши.
	typedKey(k fyne.KeyName, repeat bool)
	// preview показывает страницу во время перемещения ползунка.
	preview(page int)
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
	rtl     bool
	// doubleTap — масштаб двойного тапа (0 — двойной тап выключен).
	doubleTap float32

	// now — текущее время; подменяется в тестах.
	now func() time.Time
	// lastFlip — время последнего перелистывания (колесо, клавиши).
	lastFlip time.Time
	// keyDowns — оболочка сообщает о нажатиях (KeyDown, только ПК):
	// по ним отличается автоповтор; freshKey — нажатая, ещё не набранная клавиша.
	keyDowns bool
	freshKey fyne.KeyName
	// finishAt — время последней попытки перейти за последнюю страницу
	// (ноль — подтверждение не ожидается).
	finishAt time.Time

	onHome     func()
	onFinished func(model.Gallery)
	onPage     func(model.Gallery, int)
	// moved — в этом открытии страница менялась; reported — последняя
	// страница, о которой сообщено onPage (−1 — ни одной).
	moved    bool
	reported int

	paged *pagedView
	strip *stripView
	panel *panel
	badge *zoomBadge
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
		now:      time.Now,
		settings: settings,
		mode:     settings.String(prefMode, ModePaged),
	}
	if r.mode != ModePaged && r.mode != ModeStrip {
		r.mode = ModePaged
	}
	r.paged = newPagedView(r)
	r.strip = newStripView(r)
	r.panel = newPanel(r)
	r.badge = newZoomBadge(r)

	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	r.views = container.NewStack(r.paged, r.strip)
	// индикатор масштаба — над страницей, под панелью
	r.layer = container.NewStack(bg, r.views, container.NewWithoutLayout(r.badge), r.panel.layer)
	r.layer.Hide()
	return r
}

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (r *Reader) SetDispatcher(do func(func())) { r.do = do }

// SetOnHome задаёт действие кнопки «Домой» (закрыть читалку и страницу
// произведения); nil — только закрыть читалку.
func (r *Reader) SetOnHome(f func()) { r.onHome = f }

// SetOnFinished задаёт обработчик «произведение дочитано»: вызывается после
// закрытия читалки подтверждённым переходом за последнюю страницу.
func (r *Reader) SetOnFinished(f func(model.Gallery)) { r.onFinished = f }

// HomeButton — кнопка «Домой» на панели (для тестов).
func (r *Reader) HomeButton() *widget.Button { return r.panel.homeBtn }

// SetOnPage задаёт обработчик смены страницы (позиция чтения). Вызывается
// только после первого перелистывания в текущем открытии, не во время
// перемещения ползунка, и при закрытии читалки.
func (r *Reader) SetOnPage(f func(model.Gallery, int)) { r.onPage = f }

// Layer — объект для размещения в Stack оболочки.
func (r *Reader) Layer() fyne.CanvasObject { return r.layer }

// Visible сообщает, открыта ли читалка.
func (r *Reader) Visible() bool { return r.visible }

// Gallery — открытое произведение (пустое, если читалка закрыта).
func (r *Reader) Gallery() model.Gallery { return r.g }

// Page — текущая страница (с 0).
func (r *Reader) Page() int { return r.cur }

// Open открывает галерею с первой страницы.
func (r *Reader) Open(g model.Gallery) { r.OpenAt(g, 0) }

// OpenAt открывает галерею на странице page (с 0; вне диапазона — первая).
func (r *Reader) OpenAt(g model.Gallery, page int) {
	if page < 0 || page >= len(g.Pages) {
		page = 0
	}
	r.token++
	r.g = g
	r.sizes = nil
	r.cur = page
	r.moved, r.reported = false, -1
	r.visible = true
	r.rtl = app.ReaderDirection(r.settings) == app.ReaderDirectionRTL
	r.doubleTap = app.ReaderDoubleTap(r.settings)
	r.lastFlip, r.finishAt = time.Time{}, time.Time{}
	r.win.Canvas().Unfocus()
	r.loader.NewGeneration()

	r.panel.setGallery(g)
	r.panel.setPage(page, len(g.Pages))
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
				log.Printf("reader: page sizes %s: %v", g.Key, err)
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
	r.panel.cancelEdit()
	r.panel.endScrub()
	if r.moved && r.onPage != nil {
		r.onPage(r.g, r.cur) // позиция при закрытии
	}
	r.layer.Hide()
	r.paged.reset()
	r.strip.reset()
	r.loader.Clear()
	r.g = model.Gallery{}
	r.sizes = nil
	// вернуть освобождённую память системе сразу, а не по таймеру сборщика
	go debug.FreeOSMemory()
}

// KeyDown сообщает о нажатии клавиши (desktop.Canvas.SetOnKeyDown, только
// ПК): следующий TypedKey этой клавиши — нажатие, остальные — автоповтор.
func (r *Reader) KeyDown(k fyne.KeyName) {
	r.keyDowns = true
	r.freshKey = k
}

// TypedKey обрабатывает клавиши навигации открытой читалки. G — ввод номера
// страницы.
func (r *Reader) TypedKey(k fyne.KeyName) {
	if !r.visible {
		return
	}
	repeat := r.keyDowns && r.freshKey != k
	r.freshKey = ""
	if k == fyne.KeyG {
		r.panel.editPage()
		return
	}
	r.current().typedKey(k, repeat)
}

// throttled сообщает, что перелистывание нужно пропустить: limited — колесо
// или автоповтор, и с прошлого перелистывания не прошло flipInterval.
// Иначе запоминает время перелистывания.
func (r *Reader) throttled(limited bool) bool {
	now := r.now()
	if limited && now.Sub(r.lastFlip) < flipInterval {
		return true
	}
	r.lastFlip = now
	return false
}

// tryFinish — переход «вперёд» с последней страницы: первая попытка
// показывает подсказку, повторная после паузы finishGuard (но не позже
// finishTTL) завершает чтение. Попытки чаще finishGuard (инерция колеса,
// автоповтор) только продлевают ожидание.
func (r *Reader) tryFinish() {
	now := r.now()
	prev := r.finishAt
	r.finishAt = now
	if !prev.IsZero() {
		gap := now.Sub(prev)
		if gap < finishGuard {
			return
		}
		if gap <= finishTTL {
			r.finish()
			return
		}
	}
	r.notify(i18n.T("reader.end_hint"))
}

// finish закрывает читалку и сообщает, что произведение дочитано.
func (r *Reader) finish() {
	g := r.g
	r.Close()
	if r.onFinished != nil {
		r.onFinished(g)
	}
}

// home — кнопка «Домой».
func (r *Reader) home() {
	if r.onHome != nil {
		r.onHome()
		return
	}
	r.Close()
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
	r.panel.setRTL(r.rtl)
	r.badge.hideNow() // масштаб — только в постраничном режиме
	r.current().show(r.g, r.cur)
	if r.sizes != nil {
		r.current().setSizes(r.sizes)
	}
}

// pageChanged вызывается видом при смене текущей страницы. Во время
// перемещения ползунка номер на панели не меняется.
func (r *Reader) pageChanged(page int) {
	if page != r.cur {
		r.finishAt = time.Time{} // ожидание подтверждения завершения отменено
		r.moved = true
	}
	r.cur = page
	if r.panel.scrubbing {
		return
	}
	r.panel.setPage(page, len(r.g.Pages))
	if r.moved && page != r.reported && r.onPage != nil {
		r.reported = page
		r.onPage(r.g, page)
	}
}

// goTo — переход к странице из панели.
func (r *Reader) goTo(page int) {
	if page < 0 || page >= len(r.g.Pages) {
		return
	}
	r.current().goTo(page)
}

// preview — показ страницы во время перемещения ползунка.
func (r *Reader) preview(page int) {
	if page < 0 || page >= len(r.g.Pages) {
		return
	}
	r.current().preview(page)
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
