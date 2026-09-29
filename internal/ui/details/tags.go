package details

import (
	"errors"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/ui/screens"
)

const (
	// maxSuggestions — наибольшее число подсказок под полем ввода тега.
	maxSuggestions = 10
	// suggestDelay — пауза после ввода перед запросом подсказок.
	suggestDelay = 150 * time.Millisecond
	// customAlpha — непрозрачность жёлтого фона своего тега.
	customAlpha = 0x5a
)

// TagEditor — правка тегов произведения. Методы вызываются из UI-потока,
// работа выполняется в фоне, cb вызывается из любой горутины.
type TagEditor interface {
	// Available — пользовательские данные доступны.
	Available() bool
	// Edit применяет правку и сообщает галерею с новыми тегами.
	Edit(k model.Key, op app.TagOp, cb func(model.Gallery, error))
	// Suggest — имена тегов типа tagType с префиксом prefix (по убыванию
	// числа произведений), не больше limit.
	Suggest(tagType, prefix string, limit int, cb func([]string))
}

// SetTagEditor подключает правку тегов; notify показывает уведомление
// (ошибка сохранения, «такой тег уже есть»). Без него кнопка «Изменить
// теги» неактивна.
func (d *Details) SetTagEditor(ed TagEditor, notify func(string)) {
	d.tags, d.notify = ed, notify
}

// tagsState — состояние раздела тегов открытой страницы.
type tagsState struct {
	editing bool
	input   *tagInput // открытое поле ввода; nil — нет

	// элементы последней сборки (для тестов)
	editBtn  *widget.Button
	addBtns  map[string]*widget.Button
	otherSel *widget.Select
	resetBtn *widget.Button
	chips    []*tagChip
}

// resetTags выходит из режима редактирования (открытие и закрытие страницы).
func (d *Details) resetTags() {
	d.closeInput()
	d.ts.editing = false
}

// buildTags собирает раздел тегов: кнопка режима, группы, в режиме
// редактирования — «Добавить в другую группу» и «Сбросить к оригиналу…».
func (d *Details) buildTags() {
	st := &d.ts
	var extra []string
	if st.input != nil {
		extra = []string{st.input.typ}
	}
	views := d.g.TagViews()
	groups := TagGroups(views, st.editing, extra...)
	st.chips, st.addBtns = nil, map[string]*widget.Button{}

	if st.editing {
		st.editBtn = widget.NewButtonWithIcon(i18n.T("tags.done"), theme.ConfirmIcon(), d.stopEditing)
	} else {
		st.editBtn = widget.NewButtonWithIcon(i18n.T("tags.edit"), theme.DocumentCreateIcon(), d.startEditing)
		if d.tags == nil || !d.tags.Available() {
			st.editBtn.Disable()
		}
	}
	objs := []fyne.CanvasObject{container.NewHBox(st.editBtn)}
	if len(groups) > 0 {
		form := container.New(layout.NewFormLayout())
		for _, gr := range groups {
			form.Add(widget.NewLabelWithStyle(gr.Label, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
			form.Add(d.groupRow(gr))
		}
		objs = append(objs, form)
	}

	st.otherSel, st.resetBtn = nil, nil
	if st.editing {
		missing := missingTypes(groups)
		labels := make([]string, len(missing))
		byLabel := map[string]string{}
		for i, typ := range missing {
			labels[i] = groupLabel(typ)
			byLabel[labels[i]] = typ
		}
		st.otherSel = widget.NewSelect(labels, func(label string) {
			if typ, ok := byLabel[label]; ok {
				d.openInput(typ)
			}
		})
		st.otherSel.PlaceHolder = i18n.T("tags.add_group")
		if len(labels) == 0 {
			st.otherSel.Disable()
		}
		custom, hidden := overlayCounts(views)
		st.resetBtn = widget.NewButtonWithIcon(i18n.T("tags.reset"), theme.ContentUndoIcon(), func() {
			d.confirmReset(custom, hidden)
		})
		if custom+hidden == 0 {
			st.resetBtn.Disable()
		}
		objs = append(objs, container.New(layout.NewRowWrapLayout(), st.otherSel, st.resetBtn))
	}
	d.tagsBox.Objects = objs
	d.tagsBox.Refresh()
}

// overlayCounts — число своих и показанных скрытых тегов.
func overlayCounts(views []model.TagView) (custom, hidden int) {
	for _, v := range views {
		switch v.Origin {
		case model.OriginCustom:
			custom++
		case model.OriginHidden:
			hidden++
		}
	}
	return custom, hidden
}

// groupRow — чипы тегов группы с переносом по строкам; в режиме
// редактирования — «+» в конце или открытое поле ввода под чипами.
func (d *Details) groupRow(gr Group) fyne.CanvasObject {
	st := &d.ts
	chips := container.New(layout.NewRowWrapLayout())
	for _, t := range gr.Tags {
		chips.Add(d.newChip(gr.Type, t))
	}
	inputHere := st.input != nil && st.input.typ == gr.Type
	if st.editing && !inputHere {
		add := widget.NewButtonWithIcon("", theme.ContentAddIcon(), func() { d.openInput(gr.Type) })
		add.Importance = widget.LowImportance
		st.addBtns[gr.Type] = add
		chips.Add(add)
	}
	if !inputHere {
		return chips
	}
	if len(chips.Objects) == 0 {
		return st.input.box
	}
	return container.NewVBox(chips, st.input.box)
}

func (d *Details) newChip(typ string, t GroupTag) *tagChip {
	tag := model.NewTag(typ, t.Name)
	c := newTagChip(t.Name, t.Origin)
	if !d.ts.editing {
		q := TagQuery(typ, t.Name)
		c.onTap = func() { d.onSearch(q) }
	} else {
		op, icon := app.TagHide, theme.CancelIcon()
		switch t.Origin {
		case model.OriginCustom:
			op = app.TagRemove
		case model.OriginHidden:
			op, icon = app.TagUnhide, theme.ContentUndoIcon()
		}
		c.action = widget.NewButtonWithIcon("", icon, func() { d.editTags(app.TagOp{Kind: op, Tag: tag}) })
		c.action.Importance = widget.LowImportance
	}
	d.ts.chips = append(d.ts.chips, c)
	return c
}

func (d *Details) startEditing() {
	if !d.visible || d.tags == nil || !d.tags.Available() {
		return
	}
	d.ts.editing = true
	d.rebuildTags()
}

func (d *Details) stopEditing() {
	d.resetTags()
	d.rebuildTags()
}

// rebuildTags пересобирает раздел тегов, сохраняя прокрутку страницы.
func (d *Details) rebuildTags() {
	off := d.scroll.Offset
	d.buildTags()
	d.updateSpacer()
	d.body.Refresh()
	d.scroll.Offset = off
	d.scroll.Refresh()
	// RowWrapLayout узнаёт свою высоту после первой раскладки (см. build)
	token := d.token
	d.do(func() {
		if token == d.token {
			d.body.Refresh()
		}
	})
}

// editTags применяет правку в фоне; результат — только для той же страницы.
func (d *Details) editTags(op app.TagOp) {
	if !d.visible || d.tags == nil {
		return
	}
	token, k := d.token, d.g.Key
	in := d.ts.input
	d.tags.Edit(k, op, func(g model.Gallery, err error) {
		d.do(func() {
			if token != d.token {
				return
			}
			if err != nil {
				d.showTagError(err)
				return
			}
			d.g = g
			if op.Kind == app.TagAdd && d.ts.input == in {
				d.closeInput()
			}
			d.rebuildTags()
		})
	})
}

func (d *Details) showTagError(err error) {
	if d.notify == nil {
		return
	}
	switch {
	case errors.Is(err, app.ErrTagExists), errors.Is(err, app.ErrTagInvalid), errors.Is(err, app.ErrUserDataUnavailable):
		d.notify(screens.ErrorText(err))
	default:
		d.notify(i18n.T("tags.save_failed", "Error", screens.ErrorText(err)))
	}
}

// confirmReset спрашивает подтверждение сброса к оригиналу.
func (d *Details) confirmReset(custom, hidden int) {
	d.confirm(i18n.T("tags.reset.title"), i18n.T("tags.reset.text", "Custom", custom, "Hidden", hidden),
		i18n.T("tags.reset.button"), func(ok bool) {
			if ok {
				d.editTags(app.TagOp{Kind: app.TagReset})
			}
		})
}

// --- поле ввода ---

// tagInput — поле ввода нового тега в строке группы: имя, «Добавить»,
// «Отмена», ошибка и подсказки под ним.
type tagInput struct {
	typ   string
	entry *tagEntry
	add   *widget.Button
	err   *widget.Label
	sugg  *fyne.Container
	box   fyne.CanvasObject
	gen   int // поколение подсказок: устаревшие ответы отбрасываются
	timer *time.Timer
}

// openInput открывает поле ввода тега типа typ (закрывая прежнее).
func (d *Details) openInput(typ string) {
	if !d.ts.editing {
		return
	}
	d.closeInput()
	in := &tagInput{typ: typ}
	in.entry = newTagEntry(d.closeInputAndRebuild)
	in.entry.SetPlaceHolder(i18n.T("tags.name_placeholder"))
	in.entry.OnSubmitted = func(s string) { d.submitTag(in, s) }
	in.entry.OnChanged = func(s string) {
		d.setInputError(in, s, false)
		d.scheduleSuggest(in, s)
	}
	in.add = widget.NewButton(i18n.T("tags.add"), func() { d.submitTag(in, in.entry.Text) })
	in.add.Importance = widget.HighImportance
	cancel := widget.NewButton(i18n.T("dialog.cancel"), d.closeInputAndRebuild)
	in.err = widget.NewLabel("")
	in.err.Importance = widget.DangerImportance
	in.err.Wrapping = fyne.TextWrapWord
	in.err.Hide()
	in.sugg = container.NewVBox()
	in.box = container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(in.add, cancel), in.entry),
		in.err, in.sugg)
	d.ts.input = in
	d.rebuildTags()

	token := d.token
	d.do(func() {
		if token != d.token || d.ts.input != in {
			return
		}
		d.win.Canvas().Focus(in.entry)
		d.scrollToInput(in)
	})
}

// closeInput закрывает поле ввода без пересборки раздела.
func (d *Details) closeInput() {
	if in := d.ts.input; in != nil {
		in.gen++
		if in.timer != nil {
			in.timer.Stop()
		}
		d.win.Canvas().Unfocus()
	}
	d.ts.input = nil
}

func (d *Details) closeInputAndRebuild() {
	if d.ts.input == nil {
		return
	}
	d.closeInput()
	d.rebuildTags()
}

// submitTag добавляет свой тег с именем name из поля in.
func (d *Details) submitTag(in *tagInput, name string) {
	if d.ts.input != in {
		return
	}
	if !d.setInputError(in, name, true) {
		return
	}
	d.editTags(app.TagOp{Kind: app.TagAdd, Tag: model.NewTag(in.typ, name)})
}

// setInputError показывает под полем ошибку имени; пустое имя — ошибка
// только при подтверждении. true — имя допустимо.
func (d *Details) setInputError(in *tagInput, name string, submitting bool) bool {
	err := app.ValidateTagName(name)
	show := err != nil && (submitting || model.NewTag("", name).Name != "")
	if show {
		in.err.SetText(screens.ErrorText(err))
		in.err.Show()
	} else if in.err.Visible() {
		in.err.Hide()
	}
	return err == nil
}

// scrollToInput прокручивает страницу так, чтобы поле было в верхней трети
// экрана: на телефоне клавиатура не закрывает поле и подсказки.
func (d *Details) scrollToInput(in *tagInput) {
	drv := fyne.CurrentApp().Driver()
	y := drv.AbsolutePositionForObject(in.entry).Y - drv.AbsolutePositionForObject(d.scroll.Content).Y
	d.scroll.ScrollToOffset(fyne.NewPos(0, max(0, y-d.scroll.Size().Height/6)))
}

// updateSpacer: при открытом поле внизу страницы есть место, чтобы поле
// можно было прокрутить вверх даже в конце страницы.
func (d *Details) updateSpacer() {
	h := float32(0)
	if d.ts.input != nil {
		h = d.win.Canvas().Size().Height * 2 / 3
	}
	d.spacer.SetMinSize(fyne.NewSize(0, h))
}

// --- подсказки ---

// scheduleSuggest запрашивает подсказки после паузы в вводе.
func (d *Details) scheduleSuggest(in *tagInput, text string) {
	in.gen++
	gen := in.gen
	if in.timer != nil {
		in.timer.Stop()
	}
	prefix := model.NewTag("", text).Name
	if prefix == "" || d.tags == nil {
		d.showSuggestions(in, nil)
		return
	}
	in.timer = time.AfterFunc(d.suggestDelay, func() {
		d.do(func() {
			if d.ts.input != in || in.gen != gen {
				return
			}
			have := d.groupNames(in.typ)
			d.tags.Suggest(in.typ, prefix, maxSuggestions+len(have), func(names []string) {
				d.do(func() {
					if d.ts.input != in || in.gen != gen {
						return
					}
					var out []string
					for _, n := range names {
						if !have[n] && len(out) < maxSuggestions {
							out = append(out, n)
						}
					}
					d.showSuggestions(in, out)
				})
			})
		})
	})
}

// groupNames — имена тегов типа typ, которые уже есть у произведения
// (в том числе скрытые).
func (d *Details) groupNames(typ string) map[string]bool {
	have := map[string]bool{}
	for _, v := range d.g.TagViews() {
		if v.Type == typ {
			have[v.Name] = true
		}
	}
	return have
}

func (d *Details) showSuggestions(in *tagInput, names []string) {
	if len(names) == 0 && len(in.sugg.Objects) == 0 {
		return
	}
	objs := make([]fyne.CanvasObject, len(names))
	for i, n := range names {
		b := widget.NewButton(n, func() { d.submitTag(in, n) })
		b.Alignment = widget.ButtonAlignLeading
		b.Importance = widget.LowImportance
		objs[i] = b
	}
	in.sugg.Objects = objs
	in.sugg.Refresh()
	d.body.Refresh()
}

// --- поле с Esc ---

// tagEntry — поле ввода, в котором Esc отменяет ввод.
type tagEntry struct {
	widget.Entry
	onEsc func()
}

func newTagEntry(onEsc func()) *tagEntry {
	e := &tagEntry{onEsc: onEsc}
	e.ExtendBaseWidget(e)
	return e
}

func (e *tagEntry) TypedKey(ev *fyne.KeyEvent) {
	if ev.Name == fyne.KeyEscape {
		e.onEsc()
		return
	}
	e.Entry.TypedKey(ev)
}

// --- чип ---

// tagChip — тег на странице произведения: скруглённый фон и имя; свой тег
// выделен жёлтым фоном и рамкой, скрытый — красным зачёркнутым текстом.
// В режиме редактирования справа кнопка ✕ или ↺.
type tagChip struct {
	widget.BaseWidget
	name   string
	origin model.TagOrigin
	onTap  func()         // nil — нажатие на имя ничего не делает
	action *widget.Button // ✕ / ↺; nil — нет
}

var (
	_ fyne.Tappable      = (*tagChip)(nil)
	_ desktop.Cursorable = (*tagChip)(nil)
)

func newTagChip(name string, origin model.TagOrigin) *tagChip {
	c := &tagChip{name: name, origin: origin}
	c.ExtendBaseWidget(c)
	return c
}

func (c *tagChip) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *tagChip) Cursor() desktop.Cursor {
	if c.onTap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

func (c *tagChip) CreateRenderer() fyne.WidgetRenderer {
	c.ExtendBaseWidget(c)
	r := &chipRenderer{c: c, bg: canvas.NewRectangle(color.Transparent), text: canvas.NewText(c.name, color.Black)}
	r.text.TextStyle.Strikethrough = c.origin == model.OriginHidden
	r.objects = []fyne.CanvasObject{r.bg, r.text}
	if c.action != nil {
		r.objects = append(r.objects, c.action)
	}
	r.applyTheme()
	return r
}

type chipRenderer struct {
	c       *tagChip
	bg      *canvas.Rectangle
	text    *canvas.Text
	objects []fyne.CanvasObject
}

func (r *chipRenderer) applyTheme() {
	th := r.c.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	r.bg.CornerRadius = th.Size(theme.SizeNameInputRadius) * 2
	r.bg.FillColor = th.Color(theme.ColorNameInputBackground, v)
	r.bg.StrokeWidth = 0
	r.text.Color = th.Color(theme.ColorNameForeground, v)
	r.text.TextSize = th.Size(theme.SizeNameText)
	switch r.c.origin {
	case model.OriginCustom:
		// цвет — на фоне: жёлтый текст на светлой теме нечитаем
		warn := th.Color(theme.ColorNameWarning, v)
		cr, cg, cb, _ := warn.RGBA()
		r.bg.FillColor = color.NRGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), customAlpha}
		r.bg.StrokeColor = warn
		r.bg.StrokeWidth = th.Size(theme.SizeNameInputBorder)
	case model.OriginHidden:
		r.text.Color = th.Color(theme.ColorNameError, v)
	}
}

func (r *chipRenderer) pad() float32 { return r.c.Theme().Size(theme.SizeNameInnerPadding) }

func (r *chipRenderer) actionSize() fyne.Size {
	if r.c.action == nil {
		return fyne.Size{}
	}
	return r.c.action.MinSize()
}

func (r *chipRenderer) MinSize() fyne.Size {
	t := r.text.MinSize()
	a := r.actionSize()
	w := t.Width + 2*r.pad()
	if a.Width > 0 {
		w = t.Width + r.pad() + a.Width
	}
	return fyne.NewSize(w, max(t.Height+2*r.pad(), a.Height))
}

func (r *chipRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.bg.Move(fyne.Position{})
	t := r.text.MinSize()
	r.text.Resize(t)
	r.text.Move(fyne.NewPos(r.pad(), (size.Height-t.Height)/2))
	if a := r.c.action; a != nil {
		as := r.actionSize()
		a.Resize(fyne.NewSize(as.Width, size.Height))
		a.Move(fyne.NewPos(size.Width-as.Width, 0))
	}
}

func (r *chipRenderer) Refresh() {
	r.text.Text = r.c.name
	r.applyTheme()
	r.bg.Refresh()
	r.text.Refresh()
}

func (r *chipRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *chipRenderer) Destroy()                     {}
