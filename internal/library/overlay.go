package library

import (
	"context"
	"fmt"
	"log"

	"mangareader/internal/model"
)

// Overlay заполняет наложение пользователя (Gallery.Custom, Gallery.Hidden)
// по пути архива (Key.ID). Галереи без наложения получают пустые списки.
// Вызывается в горутине сканирования или правки (не в UI-потоке).
type Overlay interface {
	Apply(gs []model.Gallery) error
}

// SetOverlay задаёт источник наложения (nil — без наложения). Действует со
// следующего сканирования, LoadCatalog, Refresh или Reindex. Ждёт окончания
// идущего сканирования.
func (s *Source) SetOverlay(o Overlay) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.stMu.Lock()
	s.overlay = o
	s.stMu.Unlock()
}

func (s *Source) getOverlay() Overlay {
	s.stMu.RLock()
	defer s.stMu.RUnlock()
	return s.overlay
}

// applyOverlay применяет наложение к срезам галерей; ошибка — в журнал,
// галереи остаются без наложения.
func applyOverlay(o Overlay, lists ...[]model.Gallery) {
	if o == nil {
		return
	}
	for _, gs := range lists {
		if len(gs) == 0 {
			continue
		}
		if err := o.Apply(gs); err != nil {
			log.Printf("user data: tags not applied: %v", err)
		}
	}
}

// lockEdit занимает блокировку сканирования для правки наложения.
func (s *Source) lockEdit() {
	s.edits.Add(1)
	s.scanMu.Lock()
}

func (s *Source) unlockEdit() {
	s.scanMu.Unlock()
	s.edits.Add(-1)
}

// Refresh перечитывает наложение галереи k, заменяет её в списке и индексе
// и возвращает обновлённую галерею. Ждёт окончания идущего сканирования,
// чтобы его устаревший результат не перезаписал правку, — не вызывать из
// UI-потока.
func (s *Source) Refresh(k model.Key) (model.Gallery, error) {
	s.lockEdit()
	defer s.unlockEdit()
	g, ok := s.Get(k)
	if !ok {
		return model.Gallery{}, fmt.Errorf("gallery %s not found", k)
	}
	g.Custom, g.Hidden = nil, nil
	if o := s.getOverlay(); o != nil {
		gs := []model.Gallery{g}
		if err := o.Apply(gs); err != nil {
			return model.Gallery{}, err
		}
		g = gs[0]
	}

	// список неизменяем: заменяем копией (Delete может изменить его без scanMu)
	s.mu.Lock()
	i, ok := s.byKey[k]
	if ok {
		gs := make([]model.Gallery, len(s.galleries))
		copy(gs, s.galleries)
		gs[i] = g
		s.galleries = gs
	}
	s.mu.Unlock()
	if !ok {
		return model.Gallery{}, fmt.Errorf("gallery %s not found", k)
	}
	if err := s.index.Upsert(context.Background(), g); err != nil {
		return g, fmt.Errorf("index: %s: %w", k, err)
	}
	return g, nil
}

// Reindex заново применяет наложение ко всем галереям и обновляет индекс
// (пользовательские данные заменены или недоступны). Ждёт окончания идущего
// сканирования — не вызывать из UI-потока.
func (s *Source) Reindex(ctx context.Context) error {
	s.lockEdit()
	defer s.unlockEdit()
	s.mu.RLock()
	gs := make([]model.Gallery, len(s.galleries))
	copy(gs, s.galleries)
	s.mu.RUnlock()
	for i := range gs {
		gs[i].Custom, gs[i].Hidden = nil, nil
	}
	if o := s.getOverlay(); o != nil && len(gs) > 0 {
		if err := o.Apply(gs); err != nil {
			return err
		}
	}

	byKey := make(map[model.Key]int, len(gs))
	for i, g := range gs {
		byKey[g.Key] = i
	}
	s.mu.Lock()
	s.galleries, s.byKey = gs, byKey
	s.mu.Unlock()
	for _, g := range gs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.index.Upsert(ctx, g); err != nil {
			return fmt.Errorf("index: %s: %w", g.Key, err)
		}
	}
	return nil
}
