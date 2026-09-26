//go:build sqlite_fts5

package catalog

import (
	"database/sql"
	"log"

	"mangareader/internal/library"
	"mangareader/internal/thumbs"
)

var (
	_ library.LinkBackup = (*Catalog)(nil)
	_ thumbs.Store       = Covers{}
)

// Links — копия ссылок скачанных файлов (library.LinkBackup): ею
// восстанавливается отсутствующий или повреждённый links.json.
func (c *Catalog) Links() map[string]string {
	out := map[string]string{}
	rows, err := c.db.Query(`SELECT rel, url FROM links`)
	if err != nil {
		log.Printf("каталог: ссылки: %v", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var rel, url string
		if err := rows.Scan(&rel, &url); err == nil {
			out[rel] = url
		}
	}
	return out
}

// ImportLinks переносит ссылки в каталог (новый или пересозданный каталог
// получает их из links.json).
func (c *Catalog) ImportLinks(links map[string]string) error {
	if len(links) == 0 {
		return nil
	}
	return c.write(func(tx *sql.Tx) error {
		for rel, url := range links {
			if _, err := tx.Exec(`INSERT INTO links(rel, url) VALUES(?, ?)
				ON CONFLICT(rel) DO UPDATE SET url = excluded.url`, rel, url); err != nil {
				return err
			}
		}
		return nil
	})
}
