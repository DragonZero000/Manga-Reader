package reader

import (
	"fmt"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
)

var modeLabels = map[string]string{ModePaged: "Страницы", ModeStrip: "Лента"}

// panel — панель управления поверх страницы: название, номер страницы,
// ползунок, переключатель режима и «Закрыть».
type panel struct {
	r        *Reader
	layer    *fyne.Container
	title    *widget.Label
	pageLbl  *widget.Label
	slider   *widget.Slider
	mode     *widget.RadioGroup
	suppress bool // программное изменение — не реагировать
}

func newPanel(r *Reader) *panel {
	p := &panel{r: r, title: widget.NewLabel(""), pageLbl: widget.NewLabel("")}
	p.title.Truncation = fyne.TextTruncateEllipsis
	p.title.TextStyle.Bold = true

	closeBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), r.Close)
	top := container.NewBorder(nil, nil, nil, closeBtn, p.title)

	p.slider = widget.NewSlider(1, 2)
	p.slider.Step = 1
	p.slider.OnChanged = func(v float64) {
		if !p.suppress {
			p.pageLbl.SetText(pageText(int(v)-1, len(p.r.g.Pages)))
		}
	}
	p.slider.OnChangeEnded = func(v float64) {
		if !p.suppress {
			r.goTo(int(v) - 1)
			r.win.Canvas().Unfocus()
		}
	}
	p.mode = widget.NewRadioGroup([]string{modeLabels[ModePaged], modeLabels[ModeStrip]}, func(s string) {
		if p.suppress {
			return
		}
		for mode, label := range modeLabels {
			if label == s {
				r.setMode(mode)
			}
		}
		r.win.Canvas().Unfocus()
	})
	p.mode.Horizontal = true
	p.mode.Required = true

	bottom := container.NewVBox(
		container.NewBorder(nil, nil, p.pageLbl, nil, p.slider),
		container.NewCenter(p.mode),
	)
	p.layer = container.NewBorder(newBar(top), newBar(bottom), nil, nil)
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
	p.pageLbl.SetText(pageText(page, total))
	if total <= 1 {
		p.slider.Hide()
		return
	}
	p.slider.Show()
	p.slider.Min, p.slider.Max = 1, float64(total)
	p.slider.SetValue(float64(page + 1))
}

func (p *panel) setMode(mode string) {
	p.suppress = true
	p.mode.SetSelected(modeLabels[mode])
	p.suppress = false
}

func (p *panel) toggle() {
	if p.layer.Visible() {
		p.hide()
		return
	}
	p.layer.Show()
	p.r.repaint(p.layer)
}

func (p *panel) hide() {
	p.layer.Hide()
	p.r.win.Canvas().Refresh(p.layer)
}

func pageText(page, total int) string {
	return fmt.Sprintf("%d / %d", page+1, total)
}

func itoa(n int) string { return strconv.Itoa(n) }

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
