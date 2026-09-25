// Package problems — список ошибок библиотеки и их статус просмотра.
// Список строится по результату последнего сканирования (одна запись на
// файл), сохраняются только ключи известных записей и их статус. Пакет не
// зависит от виджетов.
package problems

import (
	"encoding/json"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// KeyState — настройка с известными ошибками: JSON-объект «ключ → просмотрена».
const KeyState = "errors.state"

// Item — ошибочный файл папки библиотеки.
type Item struct {
	RelPath string
	Size    int64
	ModTime time.Time
	Reason  string
	Seen    bool
}

// key — идентичность записи: файл с другим размером или временем
// изменения — новая запись.
func (it Item) key() string {
	return it.RelPath + "\x00" + strconv.FormatInt(it.Size, 10) + "\x00" + strconv.FormatInt(it.ModTime.UnixNano(), 10)
}

// Tracker хранит текущий список ошибок. Методы безопасны для вызова из
// разных горутин.
type Tracker struct {
	settings storage.Settings

	mu    sync.Mutex
	items []Item
	known map[string]bool // ключ → просмотрена; ровно ключи items после Sync
}

// New создаёт список и читает статусы просмотра из настроек.
func New(s storage.Settings) *Tracker {
	t := &Tracker{settings: s, known: map[string]bool{}}
	if raw := s.String(KeyState, ""); raw != "" {
		if err := json.Unmarshal([]byte(raw), &t.known); err != nil {
			log.Printf("ошибки: настройка %s повреждена: %v", KeyState, err)
			t.known = map[string]bool{}
		}
	}
	return t
}

// Sync заменяет список ошибками последнего сканирования и возвращает число
// новых записей — неизвестных ни по прежнему списку, ни по сохранённому
// состоянию (то есть и после перезапуска старые ошибки новыми не считаются).
// Записи файлов, которых больше нет в списке, забываются.
func (t *Tracker) Sync(errs []library.ScanError) (added int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	items := make([]Item, 0, len(errs))
	known := make(map[string]bool, len(errs))
	for _, e := range errs {
		it := Item{RelPath: e.RelPath, Size: e.Size, ModTime: e.ModTime, Reason: e.Err.Error()}
		k := it.key()
		seen, ok := t.known[k]
		if !ok {
			added++
		}
		it.Seen = seen
		known[k] = seen
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].ModTime.Equal(items[j].ModTime) {
			return items[i].ModTime.After(items[j].ModTime)
		}
		return model.NaturalLess(items[i].RelPath, items[j].RelPath)
	})
	t.items = items
	t.setKnown(known)
	return added
}

// Items возвращает копию списка: новые по времени изменения сверху.
func (t *Tracker) Items() []Item {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Item(nil), t.items...)
}

// Unseen — число непросмотренных записей.
func (t *Tracker) Unseen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for _, it := range t.items {
		if !it.Seen {
			n++
		}
	}
	return n
}

// MarkAllSeen отмечает все записи просмотренными.
func (t *Tracker) MarkAllSeen() {
	t.mu.Lock()
	defer t.mu.Unlock()
	known := make(map[string]bool, len(t.items))
	for i := range t.items {
		t.items[i].Seen = true
		known[t.items[i].key()] = true
	}
	t.setKnown(known)
}

// setKnown заменяет состояние и сохраняет его, если оно изменилось.
// Вызывается под mu.
func (t *Tracker) setKnown(known map[string]bool) {
	if equalMaps(known, t.known) {
		return
	}
	t.known = known
	data, err := json.Marshal(known) // ключи сортируются — файл стабилен
	if err != nil {
		log.Printf("ошибки: %v", err)
		return
	}
	t.settings.SetString(KeyState, string(data))
}

func equalMaps(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
