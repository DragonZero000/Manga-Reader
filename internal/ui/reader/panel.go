package reader

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// modeKeys — ключи перевода подписей режимов.
var modeKeys = map[string]string{ModePaged: "reader.mode.paged", ModeStrip: "reader.mode.strip"}

// panel — панель управления поверх страницы: название и «Закрыть» сверху;
// «Домой», номер страницы (тап — ввод номера), ползунок и переключатель
// режима снизу.
type panel struct {
	r       *Reader
	layer   *fyne.Container
	title   *widget.Label
	pageLbl *tapLabel
	entry   *pageEntry
	slider  *widget.Slider
	homeBtn *widget.Button
	mode    *widget.RadioGroup
	// bottomBar — нижняя полоса (над ней — индикатор масштаба)
	bottomBar *bar
	// bubble — подсказка с номером страницы над бегунком во время перемещения
	bubble    *fyne.Container
	bubbleTxt *canvas.Text
	// modeLabels — подписи режимов на языке интерфейса
	modeLabels map[string]string
	suppress   bool // программное изменение — не реагировать
	rtl        bool // ползунок перевёрнут: первая страница справа
	total      int
	// scrubbing — ползунок перемещается: номер на панели не меняется
	scrubbing bool
	editing   bool
}

func newPanel(r *Reader) *panel {
	p := &panel{r: r, title: widget.NewLabel("")}
	p.title.Truncation = fyne.TextTruncateEllipsis
	p.title.TextStyle.Bold = true
	p.pageLbl = newTapLabel(p.editPage)
	p.entry = newPageEntry(p.commitEdit, p.cancelEdit)
	p.entry.Hide()

	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), r.Close)
	top := container.NewBorder(nil, nil, nil, closeBtn, p.title)

	p.slider = widget.NewSlider(1, 2)
	p.slider.Step = 1
	p.slider.OnChanged = func(v float64) {
		if p.suppress {
			return
		}
		page := p.fromSlider(v)
		p.scrubbing = true
		p.showBubble(page)
		r.preview(page)
	}
	p.slider.OnChangeEnded = func(v float64) {
		if p.suppress {
			return
		}
		p.endScrub()
		r.goTo(p.fromSlider(v))
		p.setPage(r.cur, p.total)
		r.win.Canvas().Unfocus()
	}
	p.modeLabels = map[string]string{}
	for mode, key := range modeKeys {
		p.modeLabels[mode] = i18n.T(key)
	}
	p.mode = widget.NewRadioGroup([]string{p.modeLabels[ModePaged], p.modeLabels[ModeStrip]}, func(s string) {
		if p.suppress {
			return
		}
		for mode, label := range p.modeLabels {
			if label == s {
				r.setMode(mode)
			}
		}
		r.win.Canvas().Unfocus()
	})
	p.mode.Horizontal = true
	p.mode.Required = true

	p.homeBtn = widget.NewButtonWithIcon("", theme.HomeIcon(), r.home)
	left := container.NewHBox(p.homeBtn, container.NewStack(p.pageLbl, p.entry))
	bottom := container.NewVBox(
		container.NewBorder(nil, nil, left, nil, p.slider),
		container.NewCenter(p.mode),
	)

	p.bubbleTxt = canvas.NewText("", theme.Color(theme.ColorNameForeground))
	p.bubbleTxt.TextStyle.Bold = true
	p.bubbleTxt.Alignment = fyne.TextAlignCenter
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	bg.CornerRadius = theme.InputRadiusSize()
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	p.bubble = container.NewStack(bg, container.NewPadded(p.bubbleTxt))
	p.bubble.Hide()

	p.bottomBar = newBar(bottom)
	p.layer = container.NewStack(
		container.NewBorder(newBar(top), p.bottomBar, nil, nil),
		container.NewWithoutLayout(p.bubble),
	)
	p.layer.Hide()
	return p
}

func (p *panel) setGallery(g model.Gallery) {
	p.title.SetText(g.Title)
	p.setPage(0, len(g.Pages))
}

func (p *panel) setPage(page, total int) {
	p.suppress = true
	defer func() { p.suppress = false }()
	p.total = total
	p.pageLbl.SetText(pageText(page, total))
	if total <= 1 {
		p.slider.Hide()
		return
	}
	p.slider.Show()
	p.slider.Min, p.slider.Max = 1, float64(total)
	p.slider.SetValue(p.toSlider(page))
}

// setRTL переворачивает ползунок (первая страница справа).
func (p *panel) setRTL(rtl bool) {
	if p.rtl == rtl {
		return
	}
	p.rtl = rtl
	p.setPage(p.r.cur, p.total)
}

// toSlider и fromSlider переводят номер страницы (с 0) в значение ползунка
// (1…total) и обратно с учётом направления.
func (p *panel) toSlider(page int) float64 {
	if p.rtl {
		return float64(p.total - page)
	}
	return float64(page + 1)
}

func (p *panel) fromSlider(v float64) int {
	if p.rtl {
		return p.total - int(v)
	}
	return int(v) - 1
}

func (p *panel) setMode(mode string) {
	p.suppress = true
	p.mode.SetSelected(p.modeLabels[mode])
	p.suppress = false
}

func (p *panel) toggle() {
	if p.layer.Visible() {
		p.hide()
		return
	}
	p.show()
}

func (p *panel) show() {
	if p.layer.Visible() {
		return
	}
	p.layer.Show()
	p.r.repaint(p.layer)
	p.r.badge.reposition()
}

func (p *panel) hide() {
	p.cancelEdit()
	p.layer.Hide()
	p.r.win.Canvas().Refresh(p.layer)
	p.r.badge.reposition()
}

// --- подсказка над бегунком ---

// showBubble показывает номер page над бегунком.
func (p *panel) showBubble(page int) {
	p.bubbleTxt.Text = itoa(page + 1)
	p.bubbleTxt.Refresh()
	size := p.bubble.MinSize()
	size.Width = max(size.Width, size.Height)
	p.bubble.Resize(size)

	d := fyne.CurrentApp().Driver()
	origin := d.AbsolutePositionForObject(p.layer)
	at := d.AbsolutePositionForObject(p.slider).Subtract(origin)
	th := p.slider.Theme()
	// отступ концов дорожки как у widget.Slider (endOffset)
	pad := (th.Size(theme.SizeNameInlineIcon)-4)/2 + th.Size(theme.SizeNameInnerPadding) - 1.5
	w := p.slider.Size().Width
	frac := float32(0)
	if p.slider.Max > p.slider.Min {
		frac = float32((p.slider.Value - p.slider.Min) / (p.slider.Max - p.slider.Min))
	}
	x := at.X + pad + frac*(w-2*pad) - size.Width/2
	x = max(0, min(x, p.layer.Size().Width-size.Width))
	y := at.Y - size.Height - theme.Padding()
	p.bubble.Move(fyne.NewPos(x, y))
	if !p.bubble.Visible() {
		p.bubble.Show()
	}
	p.r.win.Canvas().Refresh(p.bubble)
}

// endScrub завершает перемещение ползунка: подсказка скрыта.
func (p *panel) endScrub() {
	p.scrubbing = false
	if p.bubble.Visible() {
		p.bubble.Hide()
		p.r.win.Canvas().Refresh(p.layer)
	}
}

// --- ввод номера страницы ---

// editPage показывает панель и превращает номер страницы в поле ввода.
func (p *panel) editPage() {
	if p.editing || p.total < 1 {
		return
	}
	p.show()
	p.editing = true
	p.pageLbl.Hide()
	p.entry.SetText(itoa(p.r.cur + 1))
	p.entry.Show()
	c := p.r.win.Canvas()
	c.Focus(p.entry)
	p.entry.TypedShortcut(&fyne.ShortcutSelectAll{})
	c.Refresh(p.layer)
}

// commitEdit переходит на введённую страницу (с прижатием к 1…N);
// пустое или нечисловое значение — отмена.
func (p *panel) commitEdit(text string) {
	if !p.editing {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(text))
	p.cancelEdit()
	if err != nil {
		return
	}
	p.r.goTo(max(1, min(n, p.total)) - 1)
}

// cancelEdit возвращает надпись вместо поля ввода.
func (p *panel) cancelEdit() {
	if !p.editing {
		return
	}
	p.editing = false
	p.entry.Hide()
	p.pageLbl.Show()
	p.r.win.Canvas().Unfocus()
	p.r.win.Canvas().Refresh(p.layer)
}

func pageText(page, total int) string {
	return fmt.Sprintf("%d / %d", page+1, total)
}

func itoa(n int) string { return strconv.Itoa(n) }

// tapLabel — надпись, реагирующая на тап (номер страницы).
type tapLabel struct {
	widget.Label
	onTap func()
}

func newTapLabel(onTap func()) *tapLabel {
	l := &tapLabel{onTap: onTap}
	l.ExtendBaseWidget(l)
	return l
}

func (l *tapLabel) Tapped(*fyne.PointEvent) { l.onTap() }

// pageEntry — поле ввода номера страницы: только цифры, Enter — переход,
// Esc и «Назад» (Android) — отмена, потеря фокуса — отмена. На Android
// открывается цифровая клавиатура.
type pageEntry struct {
	widget.Entry
	onSubmit func(string)
	onCancel func()
}

func newPageEntry(onSubmit func(string), onCancel func()) *pageEntry {
	e := &pageEntry{onSubmit: onSubmit, onCancel: onCancel}
	e.ExtendBaseWidget(e)
	return e
}

func (e *pageEntry) TypedRune(r rune) {
	if r >= '0' && r <= '9' {
		e.Entry.TypedRune(r)
	}
}

func (e *pageEntry) TypedKey(k *fyne.KeyEvent) {
	switch k.Name {
	case fyne.KeyEscape, mobile.KeyBack:
		e.onCancel()
	case fyne.KeyReturn, fyne.KeyEnter:
		e.onSubmit(e.Text)
	default:
		e.Entry.TypedKey(k)
	}
}

func (e *pageEntry) FocusLost() {
	e.Entry.FocusLost()
	e.onCancel()
}

// MinSize — не уже четырёх цифр: номер большой галереи не обрезается.
func (e *pageEntry) MinSize() fyne.Size {
	s := e.Entry.MinSize()
	w := fyne.MeasureText("0000", theme.TextSize(), fyne.TextStyle{}).Width + 4*theme.InnerPadding()
	return fyne.NewSize(max(s.Width, w), s.Height)
}

// Keyboard — цифровая клавиатура на Android (mobile.Keyboardable).
func (e *pageEntry) Keyboard() mobile.KeyboardType { return mobile.NumberKeyboard }

// bar — полоса панели: полупрозрачный фон и поглощение нажатий, чтобы
// тап по панели не листал страницу под ней.
type bar struct {
	widget.BaseWidget
	content fyne.CanvasObject
}

func newBar(content fyne.CanvasObject) *bar {
	b := &bar{content: content}
	b.ExtendBaseWidget(b)
	return b
}

func (b *bar) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	return widget.NewSimpleRenderer(container.NewStack(bg, container.NewPadded(b.content)))
}

func (b *bar) Tapped(*fyne.PointEvent)       {}
func (b *bar) DoubleTapped(*fyne.PointEvent) {}
func (b *bar) Dragged(*fyne.DragEvent)       {}
func (b *bar) DragEnd()                      {}
