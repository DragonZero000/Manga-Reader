package screens

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/model"
	"mangareader/internal/thumbs"
)

// blockingOpen — первое открытие ждёт release; порядок открытий записывается.
type blockingOpen struct {
	data    []byte
	release chan struct{}
	mu      sync.Mutex
	opened  []string
}

func (o *blockingOpen) open(k model.Key, _ string) (io.ReadCloser, error) {
	o.mu.Lock()
	o.opened = append(o.opened, k.ID)
	first := len(o.opened) == 1
	o.mu.Unlock()
	if first {
		<-o.release
	}
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

func (o *blockingOpen) list() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.opened)
}

func galleryOf(id string) model.Gallery {
	return model.Gallery{Key: model.LocalKey(id), Title: id, Pages: []model.Page{{Name: "1.png"}},
		File: model.FileInfo{ModTime: time.Unix(1000, 0)}}
}

// Карточку переиспользовали до начала декодирования прежней обложки —
// прежний запрос отменён и не декодируется; в карточке — новая обложка.
func TestGridCardReuseCancels(t *testing.T) {
	a := test.NewTempApp(t)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	o := &blockingOpen{data: buf.Bytes(), release: make(chan struct{})}
	th := thumbs.New(o.open, 1<<20, 1)
	g := newGalleryGrid(th, func(model.Gallery) {})
	q := make(chan func(), 64)
	g.do = func(f func()) { q <- f }
	w := a.NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(fyne.NewSize(400, 400))
	g.items = []model.Gallery{galleryOf("busy.zip"), galleryOf("gone.zip"), galleryOf("shown.zip")}

	busy := newGalleryCard(g.coverSize)
	g.updateCard(0, busy) // занимает единственный декодер
	deadline := time.Now().Add(3 * time.Second)
	for len(o.list()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	card := newGalleryCard(g.coverSize)
	g.updateCard(1, card) // ждёт в очереди
	g.updateCard(2, card) // та же карточка — уже для другой галереи
	close(o.release)

	for time.Now().Before(deadline) {
		select {
		case f := <-q:
			f()
		default:
			time.Sleep(5 * time.Millisecond)
		}
		if card.cover.Visible() && card.cover.Image != nil {
			break
		}
	}
	if !card.cover.Visible() || card.cover.Image == nil {
		t.Fatal("обложка новой галереи не показана")
	}
	if card.thumbKey != thumbs.CacheKey(g.items[2]) {
		t.Fatalf("карточка ждёт %q", card.thumbKey)
	}
	if got := o.list(); slices.Contains(got, "gone.zip") {
		t.Fatalf("отменённая обложка декодировалась: %v", got)
	}
	if _, ok := card.cover.Image.(*image.RGBA); !ok || card.cover.ScaleMode != canvas.ImageScaleFastest {
		t.Fatalf("обложка %T, ScaleMode %v", card.cover.Image, card.cover.ScaleMode)
	}
}
