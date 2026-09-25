package storage

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"fyne.io/fyne/v2"
)

// Settings — настройки приложения «ключ — строка».
type Settings interface {
	String(key, fallback string) string
	SetString(key, value string)
}

// FileSettings — настройки в JSON-файле (ПК: settings.json в папке приложения).
// Повреждённый или отсутствующий файл даёт значения по умолчанию.
type FileSettings struct {
	path string
	mu   sync.Mutex
	vals map[string]string
}

// NewFileSettings читает настройки из path.
func NewFileSettings(path string) *FileSettings {
	s := &FileSettings{path: path, vals: map[string]string{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &s.vals); err != nil {
			log.Printf("настройки %s повреждены, используются значения по умолчанию: %v", path, err)
			s.vals = map[string]string{}
		}
	}
	return s
}

func (s *FileSettings) String(key, fallback string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.vals[key]; ok {
		return v
	}
	return fallback
}

// SetString сохраняет значение и сразу записывает файл (атомарно).
func (s *FileSettings) SetString(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[key] = value
	if err := s.save(); err != nil {
		log.Printf("настройки %s: %v", s.path, err)
	}
}

func (s *FileSettings) save() error {
	data, err := json.MarshalIndent(s.vals, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

// PrefSettings — настройки в хранилище настроек Fyne (Android).
type PrefSettings struct{ p fyne.Preferences }

func NewPrefSettings(p fyne.Preferences) PrefSettings { return PrefSettings{p} }

func (s PrefSettings) String(key, fallback string) string {
	return s.p.StringWithFallback(key, fallback)
}

func (s PrefSettings) SetString(key, value string) { s.p.SetString(key, value) }

// MemSettings — настройки в памяти (для тестов).
type MemSettings struct {
	mu   sync.Mutex
	vals map[string]string
}

func NewMemSettings() *MemSettings { return &MemSettings{vals: map[string]string{}} }

func (s *MemSettings) String(key, fallback string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok := s.vals[key]; ok {
		return v
	}
	return fallback
}

func (s *MemSettings) SetString(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[key] = value
}
