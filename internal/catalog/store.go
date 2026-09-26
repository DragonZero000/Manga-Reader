//go:build sqlite_fts5

package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"mangareader/internal/library"
	"mangareader/internal/model"
)

// ScanStore — хранилище результатов сканирования (library.ScanStore).
type ScanStore struct{ c *Catalog }

var _ library.ScanStore = ScanStore{}

// ScanStore возвращает хранилище результатов сканирования каталога.
func (c *Catalog) ScanStore() ScanStore { return ScanStore{c} }

func (s ScanStore) Load() (map[string]library.StoredEntry, error) {
	rows, err := s.c.db.Query(`SELECT rel, size, mtime, gallery, err_kind, err_text FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]library.StoredEntry{}
	for rows.Next() {
		var (
			rel           string
			size, mtime   int64
			gallery       []byte
			kind, errText sql.NullString
		)
		if err := rows.Scan(&rel, &size, &mtime, &gallery, &kind, &errText); err != nil {
			return nil, err
		}
		e := library.StoredEntry{Size: size, ModTime: time.Unix(0, mtime)}
		if errText.Valid {
			e.Err = library.RestoreError(kind.String, errText.String)
		} else if err := json.Unmarshal(gallery, &e.Gallery); err != nil {
			log.Printf("каталог: %s: запись повреждена, файл будет разобран заново: %v", rel, err)
			continue
		}
		out[rel] = e
	}
	return out, rows.Err()
}

// Save сохраняет изменения сканирования одной транзакцией: файлы, копии
// ссылок; обложки изменённых и удалённых файлов удаляются.
func (s ScanStore) Save(d library.ScanDelta) error {
	return s.c.write(func(tx *sql.Tx) error {
		for rel, e := range d.Put {
			var gallery []byte
			var kind, errText any
			if e.Err != nil {
				kind, errText = library.ErrorKind(e.Err), e.Err.Error()
			} else {
				var err error
				if gallery, err = json.Marshal(e.Gallery); err != nil {
					return fmt.Errorf("%s: %w", rel, err)
				}
			}
			if _, err := tx.Exec(`INSERT INTO files(rel, size, mtime, gallery, err_kind, err_text) VALUES(?, ?, ?, ?, ?, ?)
				ON CONFLICT(rel) DO UPDATE SET size = excluded.size, mtime = excluded.mtime, gallery = excluded.gallery,
				err_kind = excluded.err_kind, err_text = excluded.err_text`,
				rel, e.Size, e.ModTime.UnixNano(), gallery, kind, errText); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM covers WHERE rel = ?`, rel); err != nil {
				return err
			}
		}
		for _, rel := range d.Delete {
			for _, q := range []string{`DELETE FROM files WHERE rel = ?`, `DELETE FROM covers WHERE rel = ?`, `DELETE FROM links WHERE rel = ?`} {
				if _, err := tx.Exec(q, rel); err != nil {
					return err
				}
			}
		}
		for rel, url := range d.Links {
			if _, err := tx.Exec(`INSERT INTO links(rel, url) VALUES(?, ?)
				ON CONFLICT(rel) DO UPDATE SET url = excluded.url`, rel, url); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s ScanStore) StoredLink(rel string) string {
	var url string
	if err := s.c.db.QueryRow(`SELECT url FROM links WHERE rel = ?`, rel).Scan(&url); err != nil {
		return ""
	}
	return url
}

// checkIndex перестраивает индекс поиска из сохранённых галерей, если они
// разошлись (например, приложение завершилось между записью файлов и
// индекса).
func (c *Catalog) checkIndex() error {
	var docs, galleries int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM docs`).Scan(&docs); err != nil {
		return err
	}
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM files WHERE gallery IS NOT NULL`).Scan(&galleries); err != nil {
		return err
	}
	var orphans int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM docs d WHERE NOT EXISTS
		(SELECT 1 FROM files f WHERE f.rel = d.rel AND f.gallery IS NOT NULL)`).Scan(&orphans); err != nil {
		return err
	}
	if docs == galleries && orphans == 0 {
		return nil
	}
	log.Printf("каталог: индекс поиска (%d) разошёлся с галереями (%d) — перестраиваю", docs, galleries)
	stored, err := c.ScanStore().Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	return c.write(func(tx *sql.Tx) error {
		for _, t := range []string{"docs", "docs_fts", "tags"} {
			if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
				return err
			}
		}
		for _, e := range stored {
			if e.Err == nil && e.Gallery.Key != (model.Key{}) {
				if err := upsertDoc(ctx, tx, e.Gallery); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
