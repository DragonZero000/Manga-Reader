package library

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Интервалы объединения событий: сканирование — после секунды тишины,
// но не позже 10 секунд от первого события серии.
const (
	watchQuiet   = time.Second
	watchMaxWait = 10 * time.Second
)

// Watcher наблюдает за папкой библиотеки и всеми вложенными папками (ПК).
// fsnotify не умеет рекурсивно, поэтому папки добавляются по одной,
// в том числе созданные после запуска.
type Watcher struct {
	root    string
	fs      *fsnotify.Watcher
	quiet   time.Duration
	maxWait time.Duration
}

// NewWatcher начинает наблюдение за root.
func NewWatcher(root string) (*Watcher, error) {
	return newWatcher(root, watchQuiet, watchMaxWait)
}

func newWatcher(root string, quiet, maxWait time.Duration) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("наблюдение за %s: %w", root, err)
	}
	w := &Watcher{root: root, fs: fw, quiet: quiet, maxWait: maxWait}
	if err := w.addTree(root); err != nil {
		fw.Close()
		return nil, fmt.Errorf("наблюдение за %s: %w", root, err)
	}
	return w, nil
}

// addTree добавляет папку dir и все её вложенные папки, кроме скрытых.
// Ошибка возвращается только для самой dir.
func (w *Watcher) addTree(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return err
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if err := w.fs.Add(p); err != nil {
			if p == dir {
				return err
			}
			log.Printf("наблюдение: %s: %v", p, err)
		}
		return nil
	})
}

// Run передаёт в onChange объединённые серии изменений, пока не отменён ctx.
// onChange вызывается из горутины наблюдателя.
func (w *Watcher) Run(ctx context.Context, onChange func()) {
	defer w.fs.Close()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	var pending bool
	var first time.Time

	trigger := func() {
		now := time.Now()
		if !pending {
			pending, first = true, now
		}
		d := min(w.quiet, w.maxWait-now.Sub(first))
		timer.Reset(max(d, 0))
	}

	for {
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case ev, ok := <-w.fs.Events:
			if !ok {
				return
			}
			if ev.Has(fsnotify.Create) && w.isNewDir(ev.Name) {
				// в новой папке уже могут быть файлы — серия запускается в любом случае
				if err := w.addTree(ev.Name); err != nil {
					log.Printf("наблюдение: %s: %v", ev.Name, err)
				}
			}
			if w.relevant(ev) {
				trigger()
			}
		case err, ok := <-w.fs.Errors:
			if !ok {
				return
			}
			// например, переполнение буфера: полный скан всё восстановит
			log.Printf("наблюдение: %v", err)
			trigger()
		case <-timer.C:
			pending = false
			onChange()
		}
	}
}

func (w *Watcher) isNewDir(name string) bool {
	info, err := os.Stat(name)
	return err == nil && info.IsDir() && !isHidden(w.rel(name))
}

func (w *Watcher) rel(name string) string {
	r, err := filepath.Rel(w.root, name)
	if err != nil {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(r)
}

// relevant — событие может изменить результат сканирования. Не влияют:
// смена атрибутов, скрытые файлы и запись идущей загрузки (*.part);
// переименование и удаление *.part — конец загрузки — влияют.
func (w *Watcher) relevant(ev fsnotify.Event) bool {
	if ev.Op == fsnotify.Chmod || isHidden(w.rel(ev.Name)) {
		return false
	}
	if strings.HasSuffix(strings.ToLower(ev.Name), ".part") {
		return ev.Has(fsnotify.Rename) || ev.Has(fsnotify.Remove)
	}
	return true
}
