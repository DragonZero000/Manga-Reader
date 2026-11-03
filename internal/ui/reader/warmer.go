package reader

import (
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"mangareader/internal/ui/frameclock"
)

// Бюджет заблаговременной загрузки текстур страниц, пикселей в секунду.
const (
	warmRateMobile  = 40e6
	warmRateDesktop = 120e6
	// warmMaxStep — наибольший учитываемый промежуток между тактами.
	warmMaxStep = 100 * time.Millisecond
)

// warmState — что делать с объектом в очереди прогрева сейчас.
type warmState int

const (
	warmNow   warmState = iota // сдвинуть в точку и прогреть
	warmLater                  // пока нельзя (край экрана не занят страницей)
	warmDrop                   // больше не нужен или уже на экране
)

// warmItem — объект изображения, текстуру которого нужно загрузить заранее.
type warmItem struct {
	img *canvas.Image
	// spot — куда сдвинуть объект (в координатах родителя), чтобы он
	// пересекал видимую область одной строкой пикселей под видимой
	// страницей, или почему сейчас нельзя.
	spot func() (fyne.Position, warmState)
	// done вызывается, когда объект вернулся на место (nil — нет).
	done func()

	home, at fyne.Position // место объекта и точка прогрева
	hidden   bool          // объект был скрыт: показан только на такт прогрева
}

// warmer загружает текстуры готовых изображений страниц до их появления на
// экране (design D4 perf-and-layers). Скрытый объект показывается только
// на такт прогрева. Fyne создаёт текстуру объекта при
// первой отрисовке внутри видимой области, а Move текстуру не трогает:
// на такте N объект сдвигается так, что касается экрана одной строкой
// пикселей (кадр N загружает текстуру), на такте N+1 возвращается на место.
// Скорость — бюджет пикселей в секунду по фактическому времени между
// тактами, а не по числу кадров. Объект в очереди уже итогового размера:
// Resize освободил бы текстуру. Только UI-поток.
type warmer struct {
	rate float64
	// now — время кадра (frameclock.Now); startAnim запускает такты (anim.Start).
	// Подменяются в тестах.
	now       func() time.Time
	startAnim func()

	queue     []*warmItem
	moved     []*warmItem // сдвинуты на прошлом такте
	allowance float64
	last      time.Time
	anim      *fyne.Animation
	running   bool
	// warmed — прогретых объектов (для тестов).
	warmed int
}

func newWarmer(mobile bool) *warmer {
	w := &warmer{rate: warmRateDesktop, now: frameclock.Now}
	if mobile {
		w.rate = warmRateMobile
	}
	w.anim = fyne.NewAnimation(time.Second, func(float32) {
		if w.running {
			w.step(w.now())
		}
	})
	w.anim.RepeatCount = fyne.AnimationRepeatForever
	w.anim.Curve = fyne.AnimationLinear
	w.startAnim = w.anim.Start
	return w
}

// add ставит объект в очередь прогрева (повторно — не ставится).
func (w *warmer) add(img *canvas.Image, spot func() (fyne.Position, warmState), done func()) {
	if slices.ContainsFunc(w.queue, func(it *warmItem) bool { return it.img == img }) {
		return
	}
	w.queue = append(w.queue, &warmItem{img: img, spot: spot, done: done})
	if !w.running {
		w.running = true
		w.last = w.now()
		w.startAnim()
	}
}

// remove убирает объект из очереди (объект освобождён или стал видимым).
func (w *warmer) remove(img *canvas.Image) {
	w.queue = slices.DeleteFunc(w.queue, func(it *warmItem) bool { return it.img == img })
}

// clear возвращает сдвинутые объекты на место и очищает очередь.
func (w *warmer) clear() {
	w.restore()
	w.queue = nil
}

// restore возвращает объекты, сдвинутые на прошлом такте (если их не
// переставила раскладка).
func (w *warmer) restore() {
	for _, it := range w.moved {
		if it.img.Position() == it.at {
			it.img.Move(it.home)
		}
		if it.hidden {
			it.img.Hide()
		}
		if it.done != nil {
			it.done()
		}
	}
	w.moved = w.moved[:0]
}

// step — такт: сдвинутые на прошлом такте объекты возвращаются, бюджет
// растёт на rate × dt, объекты из начала очереди сдвигаются для прогрева,
// пока бюджет позволяет. Объект, который пока нельзя прогреть, пропускается.
func (w *warmer) step(now time.Time) {
	w.restore()
	// Время кадра бывает чуть впереди time.Now (вызов вне кадра): бюджет
	// считается от самого позднего времени и назад не идёт.
	if now.After(w.last) {
		w.allowance += w.rate * min(now.Sub(w.last), warmMaxStep).Seconds()
		w.last = now
	}
	for i := 0; i < len(w.queue); {
		it := w.queue[i]
		pos, st := it.spot()
		if st == warmDrop || it.img.Image == nil {
			w.queue = slices.Delete(w.queue, i, i+1)
			continue
		}
		if st == warmLater {
			i++
			continue
		}
		b := it.img.Image.Bounds()
		px := float64(b.Dx() * b.Dy())
		if px > w.allowance {
			break // ждёт бюджета; очередь — по порядку
		}
		w.allowance -= px
		w.queue = slices.Delete(w.queue, i, i+1)
		it.home, it.at = it.img.Position(), pos
		it.hidden = !it.img.Visible()
		it.img.Show()
		it.img.Move(pos)
		w.moved = append(w.moved, it)
		w.warmed++
	}
	if len(w.queue) == 0 && len(w.moved) == 0 {
		w.allowance = 0
		w.running = false
		w.anim.Stop()
	}
}
