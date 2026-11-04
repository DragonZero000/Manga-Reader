package screens

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/stats"
)

// boundsErrorText — текст ошибки границ на языке интерфейса.
func boundsErrorText(err error) string {
	var be *stats.BoundsError
	if errors.As(err, &be) {
		return i18n.T("stats.bounds.err."+be.Code, "A", be.Value)
	}
	return err.Error()
}

// statsConfig — диалог «Настроить»: галочки блоков и границы корзин.
// Изменения сохраняются сразу; статистика пересчитывается при закрытии.
type statsConfig struct {
	s      *Stats
	checks map[stats.BlockID]*widget.Check
	// fields — сворачиваемый список полей Сводки под её галочкой.
	fields      *widget.Accordion
	fieldChecks map[string]*widget.Check
	permanent   *fyne.Container
	// permItem — сворачиваемый раздел постоянных исключений (заголовок с числом).
	permItem *widget.AccordionItem
	permAcc  *widget.Accordion
	pages    *boundsField
	size     *boundsField
	dlg      dialog.Dialog
}

// newSummaryFields — «Поля сводки»: галочка на каждое поле, свёрнуто.
func (c *statsConfig) newSummaryFields() fyne.CanvasObject {
	s := c.s
	c.fieldChecks = map[string]*widget.Check{}
	box := container.NewVBox()
	for _, f := range stats.SummaryFields() {
		f := f
		ck := widget.NewCheck(summaryLabel(f), func(on bool) {
			if on == !s.sumHidden[f] {
				return
			}
			if on {
				delete(s.sumHidden, f)
			} else {
				s.sumHidden[f] = true
			}
			app.SetStatsSummaryHidden(s.settings, s.sumHidden)
			s.dirty = true
		})
		ck.SetChecked(!s.sumHidden[f])
		c.fieldChecks[f] = ck
		box.Add(ck)
	}
	// отступ слева: поля Сводки отличаются от галочек блоков ниже
	indent := canvas.NewRectangle(nil)
	indent.SetMinSize(fyne.NewSize(statIndent, 0))
	c.fields = widget.NewAccordion(widget.NewAccordionItem(i18n.T("stats.config.summary_fields"),
		container.NewBorder(nil, nil, indent, nil, box)))
	return c.fields
}

// showSummaryFields показывает «Поля сводки», только пока Сводка включена.
func (c *statsConfig) showSummaryFields(on bool) {
	if on {
		c.fields.Show()
	} else {
		c.fields.Hide()
	}
}

// rebuildPermanent пересоздаёт список постоянных исключений: «тип:имя —
// Блок» и ✕ (снять без подтверждения).
func (c *statsConfig) rebuildPermanent() {
	ids, keys := excludedList(c.s.excluded)
	var objs []fyne.CanvasObject
	if len(keys) == 0 {
		empty := widget.NewLabel(i18n.T("stats.config.permanent_empty"))
		empty.Importance = widget.LowImportance
		objs = append(objs, empty)
	}
	for i, k := range keys {
		id, k := ids[i], k
		label := widget.NewLabel(tagLabel(model.ParseTag(k), id))
		label.Truncation = fyne.TextTruncateEllipsis
		x := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
			c.s.unexcludePermanent(id, k)
			c.rebuildPermanent()
		})
		x.Importance = widget.LowImportance
		objs = append(objs, container.NewBorder(nil, nil, nil, x, label))
	}
	c.permanent.Objects = objs
	c.permanent.Refresh()
	if c.permItem != nil {
		c.permItem.Title = i18n.T("stats.config.permanent_count", "N", len(keys))
		c.permAcc.Refresh()
	}
}

// boundsField — поле границ корзин с ошибкой под ним и кнопкой «По умолчанию».
type boundsField struct {
	entry *widget.Entry
	err   *widget.Label
	reset *widget.Button
	box   *fyne.Container
}

func (c *statsConfig) newBoundsField(title, key string, def []float64) *boundsField {
	f := &boundsField{entry: widget.NewEntry(), err: widget.NewLabel("")}
	f.err.Importance = widget.DangerImportance
	f.err.Wrapping = fyne.TextWrapWord
	f.err.Hide()
	pages, size := app.StatsBounds(c.s.settings)
	cur := pages
	if key == app.KeyStatsBoundsSize {
		cur = size
	}
	f.entry.SetText(stats.FormatBounds(cur))
	f.entry.OnChanged = func(text string) {
		if err := app.SetStatsBounds(c.s.settings, key, text); err != nil {
			f.err.SetText(boundsErrorText(err))
			f.err.Show()
			return // прежние границы остаются
		}
		f.err.Hide()
		c.s.dirty = true
	}
	f.reset = widget.NewButton(i18n.T("stats.config.default"), func() {
		f.entry.SetText(stats.FormatBounds(def))
	})
	label := widget.NewLabel(title)
	label.Wrapping = fyne.TextWrapWord
	f.box = container.NewVBox(label, container.NewBorder(nil, nil, nil, f.reset, f.entry), f.err)
	return f
}

// showConfig открывает диалог настройки блоков и границ.
func (s *Stats) showConfig() {
	c := &statsConfig{s: s, checks: map[stats.BlockID]*widget.Check{}}
	body := container.NewVBox(widget.NewLabelWithStyle(i18n.T("stats.config.blocks"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	for _, id := range stats.AllBlocks() {
		id := id
		ck := widget.NewCheck(blockTitle(id), func(on bool) {
			if on == !s.disabled[id] {
				return
			}
			if on {
				delete(s.disabled, id)
			} else {
				s.disabled[id] = true
			}
			app.SetStatsDisabled(s.settings, s.disabled)
			s.dirty = true
		})
		ck.SetChecked(!s.disabled[id])
		c.checks[id] = ck
		body.Add(ck)
		if id == stats.BlockSummary {
			body.Add(c.newSummaryFields())
			prev := ck.OnChanged
			ck.OnChanged = func(on bool) {
				prev(on)
				c.showSummaryFields(on)
			}
			c.showSummaryFields(ck.Checked)
		}
	}
	body.Add(widget.NewSeparator())
	// постоянные исключения — свёрнутый раздел: их может быть много
	c.permanent = container.NewVBox()
	c.permItem = widget.NewAccordionItem("", c.permanent)
	c.permAcc = widget.NewAccordion(c.permItem)
	c.rebuildPermanent()
	body.Add(c.permAcc)
	c.pages = c.newBoundsField(i18n.T("stats.config.pages_bounds"), app.KeyStatsBoundsPages, stats.DefaultPageBounds)
	c.size = c.newBoundsField(i18n.T("stats.config.size_bounds"), app.KeyStatsBoundsSize, stats.DefaultSizeBounds)
	body.Add(widget.NewSeparator())
	body.Add(c.pages.box)
	body.Add(c.size.box)

	c.dlg = dialog.NewCustom(i18n.T("stats.config.title"), i18n.T("stats.config.done"), container.NewVScroll(body), s.win)
	c.dlg.SetOnClosed(func() {
		if s.dirty {
			s.recompute()
		}
	})
	size := s.win.Canvas().Size()
	c.dlg.Resize(fyne.NewSize(min(size.Width*0.9, 480), min(size.Height*0.9, 620)))
	s.lastConfig = c
	c.dlg.Show()
}

// --- для тестов ---

// ConfigCheck — галочка блока в открытом диалоге «Настроить».
func (s *Stats) ConfigCheck(id stats.BlockID) *widget.Check { return s.lastConfig.checks[id] }

// ConfigPages и ConfigSize — поля границ открытого диалога.
func (s *Stats) ConfigPages() *widget.Entry { return s.lastConfig.pages.entry }
func (s *Stats) ConfigSize() *widget.Entry  { return s.lastConfig.size.entry }

// ConfigPagesError и ConfigSizeError — текст ошибки под полем («» — нет).
func (s *Stats) ConfigPagesError() string { return fieldError(s.lastConfig.pages) }
func (s *Stats) ConfigSizeError() string  { return fieldError(s.lastConfig.size) }

func fieldError(f *boundsField) string {
	if !f.err.Visible() {
		return ""
	}
	return f.err.Text
}

// ConfigDefaultPages нажимает «По умолчанию» у границ страниц.
func (s *Stats) ConfigDefaultPages() { s.lastConfig.pages.reset.OnTapped() }

// CloseConfig закрывает диалог «Настроить».
func (s *Stats) CloseConfig() { s.lastConfig.dlg.Hide() }

// ConfigSummaryField — галочка поля Сводки; SummaryFieldsVisible — виден ли
// список «Поля сводки».
func (s *Stats) ConfigSummaryField(f string) *widget.Check { return s.lastConfig.fieldChecks[f] }
func (s *Stats) SummaryFieldsVisible() bool                { return s.lastConfig.fields.Visible() }

// ConfigPermanent — строки постоянных исключений в «Настроить»;
// RemovePermanent нажимает ✕ у i-й.
func (s *Stats) ConfigPermanent() []string {
	var out []string
	for _, o := range s.lastConfig.permanent.Objects {
		if row, ok := o.(*fyne.Container); ok {
			out = append(out, row.Objects[0].(*widget.Label).Text)
		}
	}
	return out
}

func (s *Stats) RemovePermanent(i int) {
	s.lastConfig.permanent.Objects[i].(*fyne.Container).Objects[1].(*widget.Button).OnTapped()
}

// PermanentTitle — заголовок раздела постоянных исключений; PermanentOpen —
// раскрыт ли он.
func (s *Stats) PermanentTitle() string { return s.lastConfig.permItem.Title }
func (s *Stats) PermanentOpen() bool    { return s.lastConfig.permItem.Open }
