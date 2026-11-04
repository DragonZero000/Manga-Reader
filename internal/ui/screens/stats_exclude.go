package screens

import (
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/stats"
)

// maxChips — сколько временных исключений показывается чипами; больше —
// свёрнутая строка «Исключено: N».
const maxChips = 3

// statsConfirm — диалог подтверждения; подменяется в тестах.
var statsConfirm = Confirm

// toggleOrder переключает порядок блока id (топ или самые большие).
func (s *Stats) toggleOrder(id stats.BlockID) {
	if s.ascending[id] {
		delete(s.ascending, id)
	} else {
		s.ascending[id] = true
	}
	app.SetStatsAscending(s.settings, s.ascending)
	s.recompute()
}

// setLargestBy задаёт величину списка самых больших.
func (s *Stats) setLargestBy(m stats.Mode) {
	if s.largestBy == m {
		return
	}
	s.largestBy = m
	app.SetStatsLargestBy(s.settings, m)
	s.recompute()
}

// excludeMenu — меню строки тега: «Скрыть» (временно) и «Скрыть навсегда».
func (s *Stats) excludeMenu(id stats.BlockID, tag model.Tag) *fyne.Menu {
	hide := fyne.NewMenuItem(i18n.T("stats.exclude"), func() { s.excludeTemp(id, tag.String()) })
	hide.Icon = theme.VisibilityOffIcon()
	forever := fyne.NewMenuItem(i18n.T("stats.exclude_forever"), func() { s.askPermanent(id, tag.String()) })
	forever.Icon = theme.DeleteIcon()
	return fyne.NewMenu("", hide, forever)
}

func addKey(m map[stats.BlockID]map[string]bool, id stats.BlockID, key string) {
	if m[id] == nil {
		m[id] = map[string]bool{}
	}
	m[id][key] = true
}

func removeKey(m map[stats.BlockID]map[string]bool, id stats.BlockID, key string) {
	delete(m[id], key)
	if len(m[id]) == 0 {
		delete(m, id)
	}
}

// excludeTemp временно (до перезапуска) убирает тег из блока id.
func (s *Stats) excludeTemp(id stats.BlockID, key string) {
	addKey(s.temp, id, key)
	s.rebuildChips()
	s.recompute()
}

// unexcludeTemp снимает временное исключение.
func (s *Stats) unexcludeTemp(id stats.BlockID, key string) {
	removeKey(s.temp, id, key)
	s.rebuildChips()
	s.recompute()
}

// askPermanent спрашивает и переносит тег в постоянные исключения блока id.
func (s *Stats) askPermanent(id stats.BlockID, key string) {
	text := i18n.T("stats.forever.text", "Tag", key, "Block", blockTitle(id))
	statsConfirm(i18n.T("stats.forever.title"), text, i18n.T("stats.forever.confirm"), func(ok bool) {
		if ok {
			s.makePermanent(id, key)
		}
	}, s.win)
}

// makePermanent делает исключение постоянным: сохраняет его в настройках и
// убирает из временных.
func (s *Stats) makePermanent(id stats.BlockID, key string) {
	removeKey(s.temp, id, key)
	addKey(s.excluded, id, key)
	app.SetStatsExcluded(s.settings, s.excluded)
	s.rebuildChips()
	s.recompute()
}

// unexcludePermanent снимает постоянное исключение («Настроить»).
func (s *Stats) unexcludePermanent(id stats.BlockID, key string) {
	removeKey(s.excluded, id, key)
	app.SetStatsExcluded(s.settings, s.excluded)
	s.dirty = true
}

// excludedOrder — блоки исключений в порядке показа.
func excludedOrder() []stats.BlockID {
	return append(stats.AllBlocks(), stats.ExcludeCustom, stats.ExcludeHidden)
}

// excludedList — исключения набора m: блоки в порядке показа, теги по
// алфавиту.
func excludedList(m map[stats.BlockID]map[string]bool) (ids []stats.BlockID, keys []string) {
	for _, id := range excludedOrder() {
		ks := make([]string, 0, len(m[id]))
		for k := range m[id] {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			ids, keys = append(ids, id), append(keys, k)
		}
	}
	return ids, keys
}

// rebuildChips пересоздаёт панель временных исключений: чипы «тип:имя —
// Блок» с ✕ (вернуть) и «Навсегда»; больше maxChips — строка «Исключено: N»,
// которая раскрывает и сворачивает чипы.
func (s *Stats) rebuildChips() {
	ids, keys := excludedList(s.temp)
	s.chipBtns = s.chipBtns[:0]
	var objs []fyne.CanvasObject
	if len(keys) > maxChips {
		icon := theme.MenuExpandIcon()
		if s.chipsOpen {
			icon = theme.MenuDropDownIcon()
		}
		head := widget.NewButtonWithIcon(i18n.T("stats.excluded_count", "N", len(keys)), icon, func() {
			s.chipsOpen = !s.chipsOpen
			s.rebuildChips()
		})
		head.Importance = widget.LowImportance
		head.Alignment = widget.ButtonAlignLeading
		objs = append(objs, head)
	}
	if len(keys) <= maxChips || s.chipsOpen {
		for i, k := range keys {
			id, k := ids[i], k
			x := widget.NewButtonWithIcon(tagLabel(model.ParseTag(k), id), theme.CancelIcon(), func() { s.unexcludeTemp(id, k) })
			x.Importance = widget.LowImportance
			x.Alignment = widget.ButtonAlignLeading
			forever := widget.NewButton(i18n.T("stats.chip.forever"), func() { s.askPermanent(id, k) })
			forever.Importance = widget.LowImportance
			s.chipBtns = append(s.chipBtns, x)
			objs = append(objs, container.NewBorder(nil, nil, nil, forever, x))
		}
	}
	s.chips.Objects = objs
	s.chips.Refresh()
}

// --- для тестов ---

// Excluded — число исключённых тегов: временных и постоянных.
func (s *Stats) Excluded() int {
	n := 0
	for _, m := range []map[stats.BlockID]map[string]bool{s.excluded, s.temp} {
		for _, set := range m {
			n += len(set)
		}
	}
	return n
}

// Permanent — постоянные исключения строками «тип:имя — Блок».
func (s *Stats) Permanent() []string {
	ids, keys := excludedList(s.excluded)
	out := make([]string, len(keys))
	for i := range keys {
		out[i] = tagLabel(model.ParseTag(keys[i]), ids[i])
	}
	return out
}

// ChipTexts — подписи показанных чипов временных исключений.
func (s *Stats) ChipTexts() []string {
	var out []string
	for _, b := range s.chipBtns {
		out = append(out, b.Text)
	}
	return out
}

// TapChip нажимает ✕ у i-го показанного чипа.
func (s *Stats) TapChip(i int) { s.chipBtns[i].OnTapped() }

// ChipsHeader — текст строки «Исключено: N» («» — её нет); TapChipsHeader
// нажимает её.
func (s *Stats) ChipsHeader() string {
	if len(s.chips.Objects) > 0 {
		if b, ok := s.chips.Objects[0].(*widget.Button); ok {
			return b.Text
		}
	}
	return ""
}

func (s *Stats) TapChipsHeader() { s.chips.Objects[0].(*widget.Button).OnTapped() }

// TapChipForever нажимает «Навсегда» у i-го показанного чипа.
func (s *Stats) TapChipForever(i int) {
	n := len(s.chips.Objects) - len(s.chipBtns)
	s.chips.Objects[n+i].(*fyne.Container).Objects[1].(*widget.Button).OnTapped()
}
