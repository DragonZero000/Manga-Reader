package userdata

import (
	"database/sql"
	"time"
)

// Progress — позиция чтения произведения.
type Progress struct {
	// Page — имя файла страницы в архиве (model.Page.Name).
	Page string
	// Index — номер страницы с 0 на момент сохранения; Total — число страниц.
	Index, Total int
	// Finished — произведение дочитано (Page — последняя прочитанная страница).
	Finished bool
	Updated  time.Time
}

// AllProgress — позиции чтения произведений папки root (путь → позиция);
// записи-сироты не входят.
func (s *Store) AllProgress(root string) (map[string]Progress, error) {
	rows, err := s.db.Query(`SELECT w.rel, p.page, p.page_index, p.total, p.finished, p.updated_at
		FROM progress p JOIN works w ON w.uid = p.uid
		WHERE w.root = ? AND w.orphaned_at IS NULL`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Progress{}
	for rows.Next() {
		var (
			rel     string
			p       Progress
			updated int64
		)
		if err := rows.Scan(&rel, &p.Page, &p.Index, &p.Total, &p.Finished, &updated); err != nil {
			return nil, err
		}
		p.Updated = time.Unix(0, updated)
		out[rel] = p
	}
	return out, rows.Err()
}

// SetProgress сохраняет позицию чтения произведения uid.
func (s *Store) SetProgress(uid int64, p Progress) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO progress(uid, page, page_index, total, finished, updated_at)
			VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(uid) DO UPDATE SET page = excluded.page, page_index = excluded.page_index,
				total = excluded.total, finished = excluded.finished, updated_at = excluded.updated_at`,
			uid, p.Page, p.Index, p.Total, p.Finished, p.Updated.UnixNano())
		return err
	})
}

// ClearProgress удаляет позицию чтения произведения uid.
func (s *Store) ClearProgress(uid int64) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM progress WHERE uid = ?`, uid)
		return err
	})
}
