package ui

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// progressDelay — задержка сохранения позиции после смены страницы: при
// быстром листании в базу пишется только последняя страница.
const progressDelay = time.Second

// pageMark — позиция, ещё не переданная службе прогресса.
type pageMark struct {
	g    model.Gallery
	page int
}

// progressState — отложенное сохранение позиции чтения.
type progressState struct {
	mu      sync.Mutex
	pending *pageMark
	seq     int
}

// setupProgress связывает читалку, страницу произведения и меню с позицией
// чтения. Вызывать до addOnStopped(svc.Close): позиция записывается раньше,
// чем закрывается база.
func (s *Shell) setupProgress(a fyne.App) {
	prog := s.svc.Progress
	s.Details.SetProgress(func(g model.Gallery) (int, bool) {
		page, ok, _ := prog.Resume(g)
		return page, ok
	}, s.continueReading)
	s.Reader.SetOnPage(s.readerPage)
	s.Reader.SetOnFinished(func(g model.Gallery) {
		s.flushPage() // позиция последней страницы — раньше отметки
		prog.Finish(g)
		s.Details.RefreshProgress()
	})
	prog.SetOnChanged(func() { s.do(s.Details.RefreshProgress) })
	s.actions.HasProgress = prog.Has
	s.actions.ResetProgress = s.resetProgress

	s.addOnStopped(s.flushPage)
	// Android может завершить приложение в фоне — позицию сохраняем сразу
	a.Lifecycle().SetOnExitedForeground(s.flushPage)
}

// readerPage — читалка сообщила страницу: во время чтения сохраняется с
// задержкой, при закрытии читалки — сразу.
func (s *Shell) readerPage(g model.Gallery, page int) {
	s.progress.mu.Lock()
	s.progress.pending = &pageMark{g: g, page: page}
	s.progress.seq++
	seq := s.progress.seq
	s.progress.mu.Unlock()
	if !s.Reader.Visible() {
		s.flushPage()
		s.Details.RefreshProgress()
		return
	}
	time.AfterFunc(progressDelay, func() {
		s.progress.mu.Lock()
		current := seq == s.progress.seq
		s.progress.mu.Unlock()
		if current {
			s.flushPage()
		}
	})
}

// flushPage передаёт отложенную позицию службе прогресса.
func (s *Shell) flushPage() {
	s.progress.mu.Lock()
	m := s.progress.pending
	s.progress.pending = nil
	s.progress.mu.Unlock()
	if m != nil {
		s.svc.Progress.Save(m.g, m.page)
	}
}

// continueReading — «Продолжить»: читалка на сохранённой странице; если
// страница найдена не по имени, а по номеру — подсказка.
func (s *Shell) continueReading(g model.Gallery, page int) {
	if p, ok, approx := s.svc.Progress.Resume(g); ok {
		page = p
		if approx {
			defer s.Toast.Show(i18n.T("reader.position_approx"))
		}
	}
	s.Reader.OpenAt(g, page)
}

// resetProgress — пункт меню «Сбросить прогресс».
func (s *Shell) resetProgress(g model.Gallery) {
	s.svc.Progress.Reset(g)
	s.Toast.Show(i18n.T("actions.progress_reset"))
	if s.Details.Visible() && s.Details.Gallery().Key == g.Key {
		s.Details.RefreshProgress()
	}
}
