//go:build sqlite_fts5

package catalog

import "log"

// Covers — миниатюры обложек на диске (реализует thumbs.Store). Обложки
// изменённых и удалённых файлов удаляет сохранение результатов сканирования.
type Covers struct{ c *Catalog }

// Covers возвращает хранилище миниатюр каталога.
func (c *Catalog) Covers() Covers { return Covers{c} }

func (s Covers) Load(key string, w, h int) ([]byte, bool) {
	var data []byte
	err := s.c.db.QueryRow(`SELECT jpeg FROM covers WHERE key = ? AND w = ? AND h = ?`, key, w, h).Scan(&data)
	return data, err == nil && len(data) > 0
}

func (s Covers) Save(key, rel string, w, h int, jpeg []byte) {
	s.c.writeMu.Lock()
	defer s.c.writeMu.Unlock()
	if _, err := s.c.db.Exec(`INSERT INTO covers(key, rel, w, h, jpeg) VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET rel = excluded.rel, w = excluded.w, h = excluded.h, jpeg = excluded.jpeg`,
		key, rel, w, h, jpeg); err != nil {
		log.Printf("каталог: обложка %s: %v", rel, err)
	}
}
