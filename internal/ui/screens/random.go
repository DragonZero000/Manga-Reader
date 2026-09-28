package screens

import (
	_ "embed"
	"hash/fnv"
	"math/rand/v2"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/model"
)

//go:embed dice.svg
var diceSVG []byte

// diceIcon — значок кнопки «Случайное» (кубик) в цвете темы.
var diceIcon = theme.NewThemedResource(fyne.NewStaticResource("dice.svg", diceSVG))

// randomPicker выбирает случайную галерею из набора. В режиме
// app.RandomModeNoRepeat галереи выдаются кругами: в круге каждая один раз,
// новый круг не начинается с последней выданной; круг начинается заново,
// если набор изменился. Используется только в UI-потоке.
type randomPicker struct {
	mode string
	rnd  *rand.Rand

	// отпечаток набора, для которого заполнен мешок
	n    int
	hash uint64
	// bag — ключи, ещё не выданные в текущем круге (выдаются с конца)
	bag     []model.Key
	last    model.Key
	hasLast bool
}

func newRandomPicker(mode string) *randomPicker {
	return &randomPicker{mode: mode, rnd: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))}
}

// SetMode меняет режим и начинает круг заново.
func (p *randomPicker) SetMode(mode string) {
	p.mode = mode
	p.bag, p.n, p.hash, p.hasLast = nil, 0, 0, false
}

// Pick выбирает галерею из items; false — набор пуст.
func (p *randomPicker) Pick(items []model.Gallery) (model.Gallery, bool) {
	if len(items) == 0 {
		return model.Gallery{}, false
	}
	if p.mode != app.RandomModeNoRepeat {
		return p.give(items[p.rnd.IntN(len(items))]), true
	}
	if h := fingerprint(items); len(items) != p.n || h != p.hash {
		p.n, p.hash, p.bag = len(items), h, nil
	}
	byKey := make(map[model.Key]model.Gallery, len(items))
	for _, g := range items {
		byKey[g.Key] = g
	}
	for {
		if len(p.bag) == 0 {
			p.fill(items)
		}
		k := p.bag[len(p.bag)-1]
		p.bag = p.bag[:len(p.bag)-1]
		if g, ok := byKey[k]; ok { // ключа может не быть при совпадении отпечатков
			return p.give(g), true
		}
	}
}

// fill начинает новый круг: перемешанные ключи набора; первым выдаётся не
// последний выданный (если галерей больше одной).
func (p *randomPicker) fill(items []model.Gallery) {
	p.bag = make([]model.Key, len(items))
	for i, g := range items {
		p.bag[i] = g.Key
	}
	p.rnd.Shuffle(len(p.bag), func(i, j int) { p.bag[i], p.bag[j] = p.bag[j], p.bag[i] })
	top := len(p.bag) - 1
	if top > 0 && p.hasLast && p.bag[top] == p.last {
		j := p.rnd.IntN(top)
		p.bag[top], p.bag[j] = p.bag[j], p.bag[top]
	}
}

func (p *randomPicker) give(g model.Gallery) model.Gallery {
	p.last, p.hasLast = g.Key, true
	return g
}

// fingerprint — хеш ключей набора по порядку (FNV-64).
func fingerprint(items []model.Gallery) uint64 {
	h := fnv.New64a()
	for _, g := range items {
		h.Write([]byte(g.Key.Source))
		h.Write([]byte{0})
		h.Write([]byte(g.Key.ID))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// randomButton — кнопка 🎲: открывает случайную галерею из items().
// Неактивна, пока набор пуст (см. update). На экран кладётся btn, а не сам
// randomButton: со встроенным *widget.Button у обёртки был бы свой рендерер,
// который Enable/Disable не обновляют (кнопка работала, но выглядела
// неактивной).
type randomButton struct {
	btn    *widget.Button
	picker *randomPicker
}

func newRandomButton(mode string, items func() []model.Gallery, open func(model.Gallery)) *randomButton {
	b := &randomButton{picker: newRandomPicker(mode)}
	b.btn = widget.NewButtonWithIcon("", diceIcon, func() {
		if g, ok := b.picker.Pick(items()); ok {
			open(g)
		}
	})
	b.btn.Disable()
	return b
}

// update делает кнопку активной, если в наборе n > 0 галерей.
func (b *randomButton) update(n int) {
	if n > 0 {
		b.btn.Enable()
	} else {
		b.btn.Disable()
	}
}
