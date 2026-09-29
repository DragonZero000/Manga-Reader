package userdata

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"mangareader/internal/model"
)

// Свои и скрытые теги произведения (user_tags). Теги хранятся
// нормализованными (model.NewTag); проверка дубликата с оригинальными
// тегами — забота вызывающего: база не знает meta.json.

// Виды записей user_tags.
const (
	kindCustom = 1
	kindHidden = 2
)

// ErrDuplicate — такой свой тег у произведения уже есть.
var ErrDuplicate = errors.New("tag already exists")

// Overlay — наложение пользователя на теги произведения.
type Overlay struct {
	Custom []model.Tag // свои, в порядке добавления
	Hidden []model.Tag // скрытые оригинальные
}

// Empty сообщает, что наложения нет.
func (o Overlay) Empty() bool { return len(o.Custom) == 0 && len(o.Hidden) == 0 }

// newEpoch возвращает случайную эпоху базы (16 байт hex).
func newEpoch() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Epoch возвращает эпоху базы: случайное значение, заданное при создании.
// Новая (пересозданная) база получает новую эпоху.
func (s *Store) Epoch() (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = 'epoch'`).Scan(&v)
	return v, err
}

// Overlays возвращает наложения произведений папки root по rel. Сироты
// не возвращаются.
func (s *Store) Overlays(root string) (map[string]Overlay, error) {
	rows, err := s.db.Query(`SELECT w.rel, t.kind, t.type, t.name
		FROM user_tags t JOIN works w ON w.uid = t.uid
		WHERE w.root = ? AND w.orphaned_at IS NULL
		ORDER BY t.uid, t.kind, t.seq`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Overlay{}
	for rows.Next() {
		var rel string
		var kind int
		var t model.Tag
		if err := rows.Scan(&rel, &kind, &t.Type, &t.Name); err != nil {
			return nil, err
		}
		o := out[rel]
		o.add(kind, t)
		out[rel] = o
	}
	return out, rows.Err()
}

// Overlay возвращает наложение записи uid.
func (s *Store) Overlay(uid int64) (Overlay, error) {
	rows, err := s.db.Query(`SELECT kind, type, name FROM user_tags
		WHERE uid = ? ORDER BY kind, seq`, uid)
	if err != nil {
		return Overlay{}, err
	}
	defer rows.Close()
	var o Overlay
	for rows.Next() {
		var kind int
		var t model.Tag
		if err := rows.Scan(&kind, &t.Type, &t.Name); err != nil {
			return Overlay{}, err
		}
		o.add(kind, t)
	}
	return o, rows.Err()
}

func (o *Overlay) add(kind int, t model.Tag) {
	switch kind {
	case kindCustom:
		o.Custom = append(o.Custom, t)
	case kindHidden:
		o.Hidden = append(o.Hidden, t)
	}
}

// AddCustom добавляет свой тег в конец; ErrDuplicate — такой уже есть.
func (s *Store) AddCustom(uid int64, t model.Tag) error {
	return s.insert(uid, kindCustom, t, true)
}

// RemoveCustom удаляет свой тег (если его нет — ничего не делает).
func (s *Store) RemoveCustom(uid int64, t model.Tag) error {
	return s.remove(uid, kindCustom, t)
}

// Hide скрывает оригинальный тег (повторное скрытие — без изменений).
func (s *Store) Hide(uid int64, t model.Tag) error {
	return s.insert(uid, kindHidden, t, false)
}

// Unhide возвращает скрытый тег.
func (s *Store) Unhide(uid int64, t model.Tag) error {
	return s.remove(uid, kindHidden, t)
}

// Reset удаляет все свои теги и возвращает все скрытые.
func (s *Store) Reset(uid int64) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM user_tags WHERE uid = ?`, uid)
		return err
	})
}

func (s *Store) insert(uid int64, kind int, t model.Tag, dupErr bool) error {
	t = model.NewTag(t.Type, t.Name)
	return s.write(func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM user_tags
			WHERE uid = ? AND kind = ? AND type = ? AND name = ?`, uid, kind, t.Type, t.Name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			if dupErr {
				return ErrDuplicate
			}
			return nil
		}
		_, err := tx.Exec(`INSERT INTO user_tags(uid, kind, type, name, seq)
			VALUES(?, ?, ?, ?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM user_tags WHERE uid = ? AND kind = ?))`,
			uid, kind, t.Type, t.Name, uid, kind)
		return err
	})
}

func (s *Store) remove(uid int64, kind int, t model.Tag) error {
	t = model.NewTag(t.Type, t.Name)
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`DELETE FROM user_tags WHERE uid = ? AND kind = ? AND type = ? AND name = ?`,
			uid, kind, t.Type, t.Name)
		return err
	})
}
