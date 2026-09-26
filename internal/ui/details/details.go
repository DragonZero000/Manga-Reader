// Package details — страница произведения: обложка, название, теги и сведения.
// Слой поверх вкладок; состояние меняется только в UI-потоке.
package details

import (
	"errors"
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/pages"
	"mangareader/internal/thumbs"
)

const (
	// coverMaxShare — наибольшая высота обложки как доля высоты окна.
	coverMaxShare = 0.6
	// coverCache — кэш изображений обложки страницы произведения.
	coverCache = 32 << 20
	// defaultAspect — высота/ширина до того, как известен размер обложки.
	defaultAspect = 1.414
)

// Details — слой страницы произведения.
type Details struct {
	win    fyne.Window
	thumbs *thumbs.Cache
	loader *pages.Loader
	onRead func(model.Gallery)
	// onSearch открывает поиск по строке запроса (нажатие на тег).
	onSearch func(string)
	// onOpenURL открывает ссылку на произведение; nil — кнопки нет.
	onOpenURL func(string)
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	g       model.Gallery
	visible bool
	token   int

	title  *widget.Label
	scroll *container.Scroll
	body   *fyne.Container
	cover  *cover
	layer  *fyne.Container
	// linkBtn — кнопка «Открыть в браузере» текущей страницы (nil — нет ссылки).
	linkBtn *widget.Button
}

// New создаёт скрытый слой. onRead открывает читалку, onSearch — поиск
// по строке запроса (нажатие на тег).
func New(win fyne.Window, src *library.Source, th *thumbs.Cache, onRead func(model.Gallery), onSearch func(string)) *Details {
	d := &Details{
		win:      win,
		thumbs:   th,
		loader:   pages.NewLoader(src.OpenPage, coverCache, 1),
		onRead:   onRead,
		onSearch: onSearch,
		do:       fyne.Do,
		title:    widget.NewLabel(""),
		body:     container.NewVBox(),
	}
	d.title.Truncation = fyne.TextTruncateEllipsis
	d.title.TextStyle.Bold = true
	d.cover = newCover(d)

	back := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), d.Close)
	topBg := canvas.NewRectangle(theme.Color(theme.ColorNameHeaderBackground))
	top := container.NewStack(topBg, container.NewBorder(nil, nil, back, nil, d.title))

	d.scroll = container.NewVScroll(container.NewPadded(d.body))
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	d.layer = container.NewStack(bg, container.NewBorder(top, nil, nil, nil, d.scroll))
	d.layer.Hide()
	return d
}

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (d *Details) SetDispatcher(do func(func())) { d.do = do }

// SetOpenURL задаёт открытие ссылки на произведение (кнопка «Открыть в
// браузере»); nil — кнопка не показывается.
func (d *Details) SetOpenURL(open func(string)) { d.onOpenURL = open }

// OpenInBrowserButton — кнопка «Открыть в браузере» (nil — не показана).
func (d *Details) OpenInBrowserButton() *widget.Button { return d.linkBtn }

// Layer — объект для размещения в Stack оболочки.
func (d *Details) Layer() fyne.CanvasObject { return d.layer }

// Visible сообщает, открыта ли страница произведения.
func (d *Details) Visible() bool { return d.visible }

// Gallery — открытое произведение.
func (d *Details) Gallery() model.Gallery { return d.g }

// Open показывает страницу произведения с начала.
func (d *Details) Open(g model.Gallery) {
	d.token++
	d.g = g
	d.visible = true
	d.win.Canvas().Unfocus()
	d.loader.NewGeneration()

	d.title.SetText(g.Title)
	d.build()
	d.scroll.ScrollToOffset(fyne.Position{})
	d.layer.Show()
	d.repaint(d.layer)
	d.cover.load()
}

// Close закрывает страницу произведения.
func (d *Details) Close() {
	if !d.visible {
		return
	}
	d.token++
	d.visible = false
	d.layer.Hide()
	d.win.Canvas().Refresh(d.layer)
	d.cover.reset()
	d.loader.Clear()
	d.g = model.Gallery{}
}

func (d *Details) read() {
	if d.visible {
		d.onRead(d.g)
	}
}

// build собирает содержимое: обложка, названия, «Читать», теги, сведения.
// Пустые разделы не добавляются.
func (d *Details) build() {
	g := d.g
	objs := []fyne.CanvasObject{d.cover}

	title := widget.NewLabelWithStyle(g.Title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Wrapping = fyne.TextWrapWord
	title.SizeName = theme.SizeNameSubHeadingText
	objs = append(objs, title)
	if alt := g.AltTitle; alt != "" {
		altLbl := widget.NewLabelWithStyle(alt, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
		altLbl.Wrapping = fyne.TextWrapWord
		objs = append(objs, altLbl)
	}

	readBtn := widget.NewButtonWithIcon("Читать", theme.MediaPlayIcon(), d.read)
	readBtn.Importance = widget.HighImportance
	buttons := container.NewHBox(readBtn)
	d.linkBtn = nil
	if u := g.SourceURL; u != "" && d.onOpenURL != nil {
		d.linkBtn = widget.NewButtonWithIcon("Открыть в браузере", theme.ComputerIcon(), func() { d.onOpenURL(u) })
		buttons.Add(d.linkBtn)
	}
	objs = append(objs, buttons)

	if groups := TagGroups(g.Tags); len(groups) > 0 {
		objs = append(objs, widget.NewSeparator())
		form := container.New(layout.NewFormLayout())
		for _, gr := range groups {
			caption := widget.NewLabelWithStyle(gr.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			form.Add(caption)
			form.Add(d.tagChips(gr))
		}
		objs = append(objs, form)
	}

	if rows := InfoRows(g); len(rows) > 0 {
		objs = append(objs, widget.NewSeparator())
		form := container.New(layout.NewFormLayout())
		for _, r := range rows {
			lbl := widget.NewLabelWithStyle(r.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			val := widget.NewLabel(r.Value)
			val.Wrapping = fyne.TextWrapBreak
			form.Add(lbl)
			form.Add(val)
		}
		objs = append(objs, form)
	}
	d.body.Objects = objs
	d.body.Refresh()
	// RowWrapLayout узнаёт свою высоту только после первой раскладки —
	// нужен ещё один проход, чтобы строки тегов не занимали лишнего места
	token := d.token
	d.do(func() {
		if token == d.token {
			d.body.Refresh()
		}
	})
}

// tagChips — «чипы» тегов группы с переносом по строкам.
func (d *Details) tagChips(gr Group) fyne.CanvasObject {
	chips := container.New(layout.NewRowWrapLayout())
	for _, name := range gr.Names {
		query := TagQuery(gr.Type, name)
		chip := widget.NewButton(name, func() { d.onSearch(query) })
		chip.Importance = widget.LowImportance
		bg := canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground))
		bg.CornerRadius = theme.InputRadiusSize() * 2
		chips.Add(container.NewStack(bg, chip))
	}
	return chips
}

// repaint перерисовывает объект через canvas окна: Container.Show сам не
// перерисовывает, а объект, скрытый с создания, ещё не известен драйверу.
func (d *Details) repaint(o fyne.CanvasObject) {
	o.Refresh()
	d.win.Canvas().Refresh(o)
}

// --- обложка ---

// cover — первая страница: вписана по ширине, не выше coverMaxShare окна.
// Сначала показывается миниатюра из кэша библиотеки, затем полный размер.
type cover struct {
	widget.BaseWidget
	d *Details

	img     *canvas.Image
	icon    *widget.Icon
	spinner *widget.Activity
	aspect  float32 // высота/ширина изображения
	want    pages.Request
	height  float32 // последняя вычисленная высота
}

func newCover(d *Details) *cover {
	c := &cover{d: d, img: canvas.NewImageFromImage(nil), icon: widget.NewIcon(theme.BrokenImageIcon()),
		spinner: widget.NewActivity(), aspect: defaultAspect}
	c.img.FillMode = canvas.ImageFillContain
	c.img.ScaleMode = canvas.ImageScaleFastest // картинка уже нужного размера: без пересчёта в UI-потоке
	c.icon.Hide()
	c.spinner.Hide()
	c.ExtendBaseWidget(c)
	return c
}

func (c *cover) CreateRenderer() fyne.WidgetRenderer {
	return &coverRenderer{c: c, center: container.NewCenter(container.NewStack(c.spinner,
		container.New(layout.NewGridWrapLayout(fyne.NewSquareSize(64)), c.icon)))}
}

func (c *cover) Tapped(*fyne.PointEvent) { c.d.read() }

// boxHeight — высота области обложки для ширины w.
func (c *cover) boxHeight(w float32) float32 {
	maxH := c.d.win.Canvas().Size().Height * coverMaxShare
	if maxH <= 0 {
		maxH = w * defaultAspect
	}
	return max(64, min(w*c.aspect, maxH))
}

func (c *cover) width() float32 {
	if w := c.Size().Width; w > 0 {
		return w
	}
	return c.d.win.Canvas().Size().Width - 4*theme.Padding()
}

func (c *cover) reset() {
	c.want = pages.Request{}
	c.img.Image = nil
	c.img.Hide()
	c.icon.Hide()
	c.setLoading(false)
	c.aspect = defaultAspect
}

// load показывает миниатюру (если есть) и запрашивает полный размер.
func (c *cover) load() {
	c.reset()
	g := c.d.g
	page, ok := g.Cover()
	if !ok {
		c.showError()
		return
	}
	if th, err, ok := c.d.thumbs.Cached(g); ok && err == nil && th != nil {
		c.show(th)
	} else {
		c.setLoading(true)
	}

	w := c.width()
	scale := c.d.win.Canvas().Scale()
	if scale <= 0 {
		scale = 1
	}
	// высота области ещё не известна точно (пропорции) — берём предел по окну
	maxH := c.d.win.Canvas().Size().Height * coverMaxShare
	req := pages.Normalize(pages.Request{Key: g.Key, Page: page.Name,
		W: int(w*scale + 0.5), H: int(max(maxH, w)*scale + 0.5)})
	c.want = req
	if res, ok := c.d.loader.Cached(req); ok {
		c.apply(req, res)
		return
	}
	token := c.d.token
	c.d.loader.Load(req, pages.PriorityCurrent, func(res pages.Result) {
		c.d.do(func() {
			if token == c.d.token {
				c.apply(req, res)
			}
		})
	})
}

func (c *cover) apply(req pages.Request, res pages.Result) {
	if req != c.want || errors.Is(res.Err, pages.ErrStale) {
		return
	}
	c.setLoading(false)
	if res.Err != nil || len(res.Parts) == 0 {
		if c.img.Image == nil { // миниатюры нет — показываем ошибку
			c.showError()
		}
		return
	}
	c.show(res.Parts[0])
}

func (c *cover) show(img image.Image) {
	b := img.Bounds()
	if b.Dx() > 0 {
		c.aspect = float32(b.Dy()) / float32(b.Dx())
	}
	c.icon.Hide()
	c.img.Image = img
	c.img.Show()
	c.img.Refresh()
	c.relayout()
}

func (c *cover) showError() {
	c.setLoading(false)
	c.img.Image = nil
	c.img.Hide()
	c.icon.Show()
	c.aspect = 0.5
	c.relayout()
}

func (c *cover) setLoading(on bool) {
	if on {
		c.spinner.Show()
		c.spinner.Start()
	} else {
		c.spinner.Stop()
		c.spinner.Hide()
	}
}

// relayout пересчитывает высоту обложки и раскладку страницы.
func (c *cover) relayout() {
	c.height = c.boxHeight(c.width())
	c.d.body.Refresh()
	c.d.repaint(c)
}

func (c *cover) MinSize() fyne.Size {
	return fyne.NewSize(64, c.boxHeight(c.width()))
}

func (c *cover) Resize(size fyne.Size) {
	c.BaseWidget.Resize(size)
	// ширина изменилась — высота тоже (пропорции обложки)
	if h := c.boxHeight(size.Width); abs(h-c.height) > 0.5 {
		c.height = h
		c.d.body.Refresh()
	}
}

type coverRenderer struct {
	c      *cover
	center *fyne.Container
}

func (r *coverRenderer) Layout(size fyne.Size) {
	r.c.img.Move(fyne.Position{})
	r.c.img.Resize(size)
	r.center.Resize(size)
}

func (r *coverRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *coverRenderer) Refresh()           { r.Layout(r.c.Size()); r.c.d.win.Canvas().Refresh(r.c) }
func (r *coverRenderer) Destroy()           {}
func (r *coverRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.c.img, r.center}
}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}
