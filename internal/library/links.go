package library

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// Links — адреса страниц, с которых скачаны файлы, по пути файла
// относительно папки библиотеки (Android: встроенный браузер сообщает адрес
// при загрузке; у SAF нет альтернативных потоков файлов, как на Windows).
// Хранится в JSON-файле личной папки приложения. Методы безопасны для
// вызова из разных горутин.
type Links struct {
	path string

	mu    sync.Mutex
	links map[string]string
}

// LoadLinks читает ссылки из path; отсутствующий или повреждённый файл —
// пустой набор.
func LoadLinks(path string) *Links {
	l := &Links{path: path, links: map[string]string{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &l.links); err != nil {
			log.Printf("ссылки %s повреждены: %v", path, err)
			l.links = map[string]string{}
		}
	}
	return l
}

// Get — адрес страницы для файла rel («» — неизвестен).
func (l *Links) Get(rel string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.links[rel]
}

// Set запоминает адрес страницы для rel (только абсолютный http(s)).
func (l *Links) Set(rel, url string) error {
	url = model.WebURL(url)
	if url == "" || rel == "" {
		return fmt.Errorf("ссылка %q для %q не сохранена", url, rel)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.links[rel] == url {
		return nil
	}
	l.links[rel] = url
	return l.save()
}

// Prune удаляет записи файлов, которых нет в exists.
func (l *Links) Prune(exists func(rel string) bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for rel := range l.links {
		if !exists(rel) {
			delete(l.links, rel)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return l.save()
}

// save записывает файл атомарно (временный файл + переименование). Под mu.
func (l *Links) save() error {
	data, err := json.MarshalIndent(l.links, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(l.path), ".links-*.tmp")
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
	return os.Rename(tmp.Name(), l.path)
}

// WithLinks — хранилище st, которое сообщает сканеру адрес страницы файла
// из links (интерфейс sourcer).
func WithLinks(st storage.Storage, links *Links) storage.Storage {
	return linkedStorage{st, links}
}

type linkedStorage struct {
	storage.Storage
	links *Links
}

func (s linkedStorage) SourceURL(rel string) string { return s.links.Get(rel) }
