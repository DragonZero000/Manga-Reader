package screens

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"

	"mangareader/internal/app"
	"mangareader/internal/storage"
)

// coverAspect — отношение высоты обложки к ширине.
const coverAspect = 1.414

// Ширина обложки на ПК по настройке «Размер карточек».
var desktopCoverWidth = map[string]float32{
	app.GridSizeSmall:  130,
	app.GridSizeMedium: 160,
	app.GridSizeLarge:  200,
}

// coverWidthMobile — ширина обложки на телефоне, пока ширина сетки неизвестна.
const coverWidthMobile = 128

// fitEpsilon — запас ширины карточки: погрешность float в
// GridWrap.ColumnCount не должна отнять колонку.
const fitEpsilon = 0.5

// fitThreshold — меньшие изменения ширины карточки не применяются: защита от
// цикла «раскладка → Refresh → раскладка». Меньше fitEpsilon, чтобы
// неприменённое изменение не отнимало колонку.
const fitThreshold = 0.25

// isMobile — телефон (настройка колонок) или ПК (размер карточек); подменяется в тестах.
var isMobile = func() bool { return fyne.CurrentDevice().IsMobile() }

// GridMetrics — размер карточек, общий для сеток библиотеки и поиска.
// На телефоне ширина карточки делит ширину сетки в портретной ориентации
// на выбранное число колонок; на ПК — по настройке «Размер карточек».
// Меняется только в UI-потоке.
type GridMetrics struct {
	settings storage.Settings
	mobile   bool
	columns  int       // карточек в ряду (телефон)
	cover    fyne.Size // текущий размер обложки
	avail    float32   // ширина сетки в портретной ориентации (0 — неизвестна)
	grids    []*galleryGrid
}

// NewGridMetrics создаёт метрики по настройкам приложения.
func NewGridMetrics(s storage.Settings) *GridMetrics {
	m := &GridMetrics{settings: s, mobile: isMobile()}
	m.load()
	return m
}

// Reload перечитывает настройку плотности и перестраивает сетки, если размер
// карточек изменился.
func (m *GridMetrics) Reload() {
	if m.load() {
		m.changed()
	}
}

// Cover — текущий размер обложки карточки.
func (m *GridMetrics) Cover() fyne.Size { return m.cover }

// load читает настройку и вычисляет размер обложки; true — размер изменился.
func (m *GridMetrics) load() bool {
	if !m.mobile {
		return m.setWidth(desktopCoverWidth[app.GridSize(m.settings)])
	}
	m.columns = app.GridColumns(m.settings)
	if m.avail == 0 {
		return m.setWidth(coverWidthMobile)
	}
	return m.setWidth(columnWidth(m.avail, m.columns))
}

// fit подгоняет ширину карточки на телефоне под ширину сетки в портретной
// ориентации avail; true — размер изменился. На ПК ничего не делает.
func (m *GridMetrics) fit(avail float32) bool {
	if !m.mobile || avail <= 0 {
		return false
	}
	m.avail = avail
	return m.setWidth(columnWidth(avail, m.columns))
}

// setWidth задаёт ширину обложки; true — размер изменился больше порога.
func (m *GridMetrics) setWidth(w float32) bool {
	if math.Abs(float64(w-m.cover.Width)) <= fitThreshold {
		return false
	}
	m.cover = fyne.NewSize(w, float32(math.Round(float64(w)*coverAspect)))
	return true
}

// changed перестраивает все сетки под новый размер карточек.
func (m *GridMetrics) changed() {
	for _, g := range m.grids {
		g.apply()
	}
}

// columnWidth — ширина карточки, при которой ровно n карточек с отступами
// между ними занимают ширину avail.
func columnWidth(avail float32, n int) float32 {
	pad := theme.Padding()
	return (avail-pad*float32(n-1))/float32(n) - fitEpsilon
}
