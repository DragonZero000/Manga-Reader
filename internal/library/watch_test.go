package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Короткие интервалы для тестов.
const (
	testQuiet   = 150 * time.Millisecond
	testMaxWait = 600 * time.Millisecond
	// settle — заведомо больше тишины: серия успела закончиться
	settle = 5 * testQuiet
)

// startWatch запускает наблюдатель за dir и возвращает счётчик вызовов.
func startWatch(t *testing.T, dir string) *atomic.Int32 {
	t.Helper()
	w, err := newWatcher(dir, testQuiet, testMaxWait)
	if err != nil {
		t.Fatal(err)
	}
	var n atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx, func() { n.Add(1) })
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return &n
}

// waitCalls ждёт, пока счётчик станет не меньше want, затем проверяет,
// что лишних вызовов нет.
func waitCalls(t *testing.T, n *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for n.Load() < want && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(settle)
	if got := n.Load(); got != want {
		t.Fatalf("вызовов %d, ожидалось %d", got, want)
	}
}

func TestWatchCreateAndRemove(t *testing.T) {
	dir := t.TempDir()
	n := startWatch(t, dir)

	p := writeFile(t, dir, "new.zip", []byte("x"))
	waitCalls(t, n, 1)

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	waitCalls(t, n, 2)
}

func TestWatchNewSubdir(t *testing.T) {
	dir := t.TempDir()
	n := startWatch(t, dir)

	if err := os.Mkdir(filepath.Join(dir, "series"), 0o755); err != nil {
		t.Fatal(err)
	}
	waitCalls(t, n, 1)

	// файл в новой папке — событие приходит, потому что папка добавлена
	writeFile(t, dir, "series/vol1.zip", []byte("x"))
	waitCalls(t, n, 2)
}

func TestWatchExistingSubdir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a/b/old.zip", []byte("x"))
	n := startWatch(t, dir)
	writeFile(t, dir, "a/b/new.zip", []byte("x"))
	waitCalls(t, n, 1)
}

func TestWatchBurstIsOneCall(t *testing.T) {
	dir := t.TempDir()
	n := startWatch(t, dir)
	for i := 0; i < 50; i++ {
		writeFile(t, dir, fmt.Sprintf("g-%d.zip", i), []byte("x"))
	}
	waitCalls(t, n, 1)
}

func TestWatchMaxWait(t *testing.T) {
	dir := t.TempDir()
	n := startWatch(t, dir)
	// события чаще, чем тишина, дольше maxWait: вызов не откладывается бесконечно
	stop := time.Now().Add(2 * testMaxWait)
	for i := 0; time.Now().Before(stop); i++ {
		writeFile(t, dir, "busy.zip", []byte(fmt.Sprint(i)))
		time.Sleep(testQuiet / 3)
	}
	if n.Load() == 0 {
		t.Fatal("за время непрерывных событий не было ни одного вызова")
	}
}

func TestWatchIgnoresPartWritesAndHidden(t *testing.T) {
	dir := t.TempDir()
	n := startWatch(t, dir)

	part := filepath.Join(dir, "g-3.zip.part")
	f, err := os.Create(part)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		f.Write([]byte("данные загрузки"))
		f.Sync()
		time.Sleep(20 * time.Millisecond)
	}
	f.Close()
	writeFile(t, dir, ".hidden.zip", []byte("x"))
	waitCalls(t, n, 0)

	// конец загрузки: .part переименован в итоговый файл
	if err := os.Rename(part, filepath.Join(dir, "g-3.zip")); err != nil {
		t.Fatal(err)
	}
	waitCalls(t, n, 1)
}

func TestNewWatcherMissingDir(t *testing.T) {
	if _, err := NewWatcher(filepath.Join(t.TempDir(), "нет")); err == nil {
		t.Fatal("ожидалась ошибка для несуществующей папки")
	}
}
