package ui

import (
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("условие не выполнено за 2 с")
}

// uiMu сериализует «UI-поток» в тестах: тестовый драйвер Fyne выполняет
// fyne.Do прямо в вызывающей горутине (например, в горутине таймера).
var uiMu sync.Mutex

func serial(f func()) { uiMu.Lock(); defer uiMu.Unlock(); f() }

func isVisible(o fyne.CanvasObject) bool {
	var v bool
	serial(func() { v = o.Visible() })
	return v
}

func labelText(tt *Toast) string {
	var s string
	serial(func() { s = tt.label.Text })
	return s
}

func TestToastKeepsContent(t *testing.T) {
	a := test.NewTempApp(t)
	w := a.NewWindow("t")
	main := widget.NewLabel("экран")
	toast := NewToast(0)
	toast.do = serial
	w.SetContent(container.NewStack(main, toast.Layer()))

	go toast.ShowFor("привет", 50*time.Millisecond) // вызов из фоновой горутины
	waitFor(t, func() bool { return isVisible(toast.Layer()) })

	if txt := labelText(toast); txt != "привет" {
		t.Fatalf("текст уведомления = %q", txt)
	}
	if !isVisible(main) {
		t.Fatal("основной экран должен оставаться видимым")
	}

	waitFor(t, func() bool { return !isVisible(toast.Layer()) })
	if w.Content().(*fyne.Container).Objects[0] != main {
		t.Fatal("содержимое окна заменено")
	}
}

func TestToastReplaceRestartsTimer(t *testing.T) {
	test.NewTempApp(t)
	toast := NewToast(0)
	toast.do = serial

	toast.ShowFor("первое", 40*time.Millisecond)
	toast.ShowFor("второе", 300*time.Millisecond)
	time.Sleep(100 * time.Millisecond) // таймер первого истёк
	if !isVisible(toast.Layer()) {
		t.Fatal("таймер первого уведомления не должен скрывать второе")
	}
	if txt := labelText(toast); txt != "второе" {
		t.Fatalf("текст = %q", txt)
	}
	waitFor(t, func() bool { return !isVisible(toast.Layer()) })
}
