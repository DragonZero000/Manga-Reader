package app

import (
	"log"
	"sync"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/userdata"
)

// Resume — страница для «Продолжить» по сохранённой позиции p и текущему
// списку страниц: ok = false — продолжать нечего; approx — страницы с
// сохранённым именем нет, взят сохранённый номер (позиция может быть
// неточной).
func Resume(p userdata.Progress, pages []model.Page) (page int, ok, approx bool) {
	n := len(pages)
	if n == 0 {
		return 0, false, false
	}
	idx := -1
	for i, pg := range pages {
		if pg.Name == p.Page {
			idx = i
			break
		}
	}
	switch {
	case p.Finished:
		// дочитано: продолжить можно только с новых страниц после последней прочитанной
		if idx >= 0 && idx+1 < n {
			return idx + 1, true, false
		}
		return 0, false, false
	case idx > 0:
		return idx, true, false
	case idx == 0:
		return 0, false, false
	}
	page = max(0, min(p.Index, n-1))
	if page == 0 {
		return 0, false, false
	}
	return page, true, true
}

// ProgressService — позиции чтения текущей папки библиотеки: кэш в памяти
// (читается из UI-потока) и запись в user.db одной фоновой горутиной по
// очереди. Методы безопасны для nil (пользовательские данные недоступны):
// чтение — «нет прогресса», запись — ничего.
type ProgressService struct {
	store *userdata.Store
	root  func() string // ключ текущей папки
	now   func() time.Time

	mu        sync.Mutex
	cache     map[string]userdata.Progress // путь → позиция
	pending   []progressOp
	onChanged func()
	wake      chan struct{}
}

type progressOp struct {
	root, rel, fp string
	p             userdata.Progress
	clear         bool
	flushed       chan struct{} // не nil — отметка для Flush
}

// newProgressService создаёт службу и запускает горутину записи.
func newProgressService(store *userdata.Store, root func() string) *ProgressService {
	p := &ProgressService{store: store, root: root, now: time.Now,
		cache: map[string]userdata.Progress{}, wake: make(chan struct{}, 1)}
	go p.writer()
	return p
}

// SetOnChanged задаёт колбэк после перезагрузки кэша (вызывается из фоновой
// горутины).
func (p *ProgressService) SetOnChanged(f func()) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.onChanged = f
	p.mu.Unlock()
}

// Reload перечитывает позиции текущей папки из базы. Обращается к базе — не
// вызывать из UI-потока.
func (p *ProgressService) Reload() {
	if p == nil {
		return
	}
	all, err := p.store.AllProgress(p.root())
	if err != nil {
		log.Printf("user data: reading progress: %v", err)
		return
	}
	p.mu.Lock()
	// записи, ещё стоящие в очереди, новее базы
	for _, op := range p.pending {
		if op.flushed != nil || op.root != p.root() {
			continue
		}
		if op.clear {
			delete(all, op.rel)
		} else {
			all[op.rel] = op.p
		}
	}
	p.cache = all
	f := p.onChanged
	p.mu.Unlock()
	if f != nil {
		f()
	}
}

// Get — сохранённая позиция произведения g.
func (p *ProgressService) Get(g model.Gallery) (userdata.Progress, bool) {
	if p == nil || g.Key.Source != model.SourceLocal {
		return userdata.Progress{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	pr, ok := p.cache[g.Key.ID]
	return pr, ok
}

// Has — есть ли у произведения сохранённый прогресс (в том числе «дочитано»).
func (p *ProgressService) Has(g model.Gallery) bool {
	_, ok := p.Get(g)
	return ok
}

// Resume — страница для «Продолжить» (см. функцию Resume).
func (p *ProgressService) Resume(g model.Gallery) (page int, ok, approx bool) {
	pr, has := p.Get(g)
	if !has {
		return 0, false, false
	}
	return Resume(pr, g.Pages)
}

// Save запоминает, что пользователь остановился на странице page.
func (p *ProgressService) Save(g model.Gallery, page int) {
	if page < 0 || page >= len(g.Pages) {
		return
	}
	p.put(g, userdata.Progress{Page: g.Pages[page].Name, Index: page, Total: len(g.Pages)})
}

// Finish отмечает произведение дочитанным на последней странице.
func (p *ProgressService) Finish(g model.Gallery) {
	n := len(g.Pages)
	if n == 0 {
		return
	}
	p.put(g, userdata.Progress{Page: g.Pages[n-1].Name, Index: n - 1, Total: n, Finished: true})
}

func (p *ProgressService) put(g model.Gallery, pr userdata.Progress) {
	if p == nil || g.Key.Source != model.SourceLocal {
		return
	}
	pr.Updated = p.now()
	p.mu.Lock()
	p.cache[g.Key.ID] = pr
	p.mu.Unlock()
	p.enqueue(progressOp{root: p.root(), rel: g.Key.ID, fp: g.Fingerprint, p: pr})
}

// Reset удаляет прогресс произведения.
func (p *ProgressService) Reset(g model.Gallery) {
	if p == nil || g.Key.Source != model.SourceLocal {
		return
	}
	p.mu.Lock()
	delete(p.cache, g.Key.ID)
	p.mu.Unlock()
	p.enqueue(progressOp{root: p.root(), rel: g.Key.ID, clear: true})
}

// Flush ждёт записи очереди не дольше timeout; false — не успела.
func (p *ProgressService) Flush(timeout time.Duration) bool {
	if p == nil {
		return true
	}
	done := make(chan struct{})
	p.enqueue(progressOp{flushed: done})
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (p *ProgressService) enqueue(op progressOp) {
	p.mu.Lock()
	p.pending = append(p.pending, op)
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// writer записывает операции по одной в порядке постановки: отметка
// «дочитано» не перезаписывается более ранней позицией.
func (p *ProgressService) writer() {
	for range p.wake {
		for {
			p.mu.Lock()
			if len(p.pending) == 0 {
				p.mu.Unlock()
				break
			}
			op := p.pending[0]
			p.pending = p.pending[1:]
			p.mu.Unlock()
			p.write(op)
		}
	}
}

func (p *ProgressService) write(op progressOp) {
	if op.flushed != nil {
		close(op.flushed)
		return
	}
	var err error
	if op.clear {
		var uid int64
		var ok bool
		if uid, ok, err = p.store.Lookup(op.root, op.rel); err == nil && ok {
			err = p.store.ClearProgress(uid)
		}
	} else {
		var uid int64
		if uid, err = p.store.Ensure(op.root, op.rel, op.fp); err == nil {
			err = p.store.SetProgress(uid, op.p)
		}
	}
	if err != nil {
		log.Printf("user data: progress of %s: %v", op.rel, err)
	}
}
