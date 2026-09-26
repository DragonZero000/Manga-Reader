//go:build sqlite_fts5

package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
)

// Index — индекс поиска над каталогом (search.Index). Семантика совпадает
// с search.MemIndex (общий набор проверок search/indextest).
type Index struct{ c *Catalog }

var _ search.Index = Index{}

// Index возвращает индекс поиска каталога.
func (c *Catalog) Index() Index { return Index{c} }

func (x Index) Upsert(ctx context.Context, g model.Gallery) error {
	return x.c.write(func(tx *sql.Tx) error { return upsertDoc(ctx, tx, g) })
}

func (x Index) Remove(ctx context.Context, k model.Key) error {
	return x.c.write(func(tx *sql.Tx) error { return removeDoc(ctx, tx, k.String()) })
}

// upsertDoc заменяет поисковые данные галереи (docs, docs_fts, tags).
func upsertDoc(ctx context.Context, tx *sql.Tx, g model.Gallery) error {
	key := g.Key.String()
	if err := removeDoc(ctx, tx, key); err != nil {
		return err
	}
	var hay []string
	add := func(s string) {
		if s = search.Normalize(strings.TrimSpace(s)); s != "" {
			hay = append(hay, s)
		}
	}
	add(g.Title)
	add(g.AltTitle)
	for _, t := range g.Tags {
		t = model.NewTag(t.Type, t.Name)
		if t.Name == "" {
			continue
		}
		add(t.Name)
		add(t.String())
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO tags(key, type, name, name_norm) VALUES(?, ?, ?, ?)`,
			key, t.Type, t.Name, search.Normalize(t.Name)); err != nil {
			return err
		}
	}
	add(g.Scanlator)
	add(g.Key.ID)
	if g.ExternalID > 0 {
		add(strconv.FormatInt(g.ExternalID, 10))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO docs_fts(key, hay) VALUES(?, ?)`, key, strings.Join(hay, "\n")); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO docs(key, rel, title_sort, title_norm, scan_norm, ext_id, pages,
		favorites, size, uploaded, added) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key, g.Key.ID, strings.ToLower(g.Title), search.Normalize(g.Title+"\n"+g.AltTitle),
		search.Normalize(strings.TrimSpace(g.Scanlator)), g.ExternalID, len(g.Pages), g.Favorites,
		g.File.Size, nanos(g.Uploaded), nanos(g.File.ModTime))
	return err
}

func removeDoc(ctx context.Context, tx *sql.Tx, key string) error {
	for _, q := range []string{`DELETE FROM docs WHERE key = ?`, `DELETE FROM docs_fts WHERE key = ?`, `DELETE FROM tags WHERE key = ?`} {
		if _, err := tx.ExecContext(ctx, q, key); err != nil {
			return err
		}
	}
	return nil
}

// nanos — время в UnixNano; нулевое — NULL (значения нет).
func nanos(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixNano()
}

// sortColumns — колонка сортировки для каждого поля (FieldAdded — по умолчанию).
var sortColumns = map[search.Field]string{
	search.FieldTitle:     "d.title_sort",
	search.FieldID:        "d.ext_id",
	search.FieldPages:     "d.pages",
	search.FieldFavorites: "d.favorites",
	search.FieldSize:      "d.size",
	search.FieldUploaded:  "d.uploaded",
	search.FieldAdded:     "d.added",
}

func (x Index) Search(ctx context.Context, q search.Query) ([]model.Key, int, error) {
	if err := q.Validate(); err != nil {
		return nil, 0, err
	}
	where, args, err := buildWhere(q)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := x.c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM docs d WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("поиск: %w", err)
	}
	col, ok := sortColumns[q.Sort.Field]
	if !ok {
		col = sortColumns[search.FieldAdded]
	}
	dir := "ASC"
	if q.Sort.Desc {
		dir = "DESC"
	}
	limit := -1
	if q.Limit > 0 {
		limit = q.Limit
	}
	rows, err := x.c.db.QueryContext(ctx, `SELECT d.key FROM docs d WHERE `+where+
		` ORDER BY `+col+` `+dir+`, d.key COLLATE NATSORT ASC LIMIT ? OFFSET ?`,
		append(args, limit, q.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("поиск: %w", err)
	}
	defer rows.Close()
	var keys []model.Key
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, 0, err
		}
		k, err := model.ParseKey(s)
		if err != nil {
			log.Printf("каталог: %v", err)
			continue
		}
		keys = append(keys, k)
	}
	return keys, total, rows.Err()
}

// buildWhere — условие WHERE для слов и фильтров (всё через «И»).
func buildWhere(q search.Query) (string, []any, error) {
	conds := []string{"1"}
	var args []any
	for _, t := range q.Terms {
		t = search.Normalize(t)
		if t == "" {
			continue
		}
		conds = append(conds, `d.key IN (SELECT key FROM docs_fts WHERE hay LIKE ?`+escapeClause(t)+`)`)
		args = append(args, "%"+escapeLike(t)+"%")
	}
	for _, f := range q.Filters {
		c, a, err := filterCond(f)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, c)
		args = append(args, a...)
	}
	return strings.Join(conds, " AND "), args, nil
}

func filterCond(f search.Filter) (string, []any, error) {
	v := f.Value
	switch f.Field {
	case search.FieldTag:
		cond := `EXISTS (SELECT 1 FROM tags t WHERE t.key = d.key AND t.name_norm = ?`
		args := []any{search.Normalize(v.Tag.Name)}
		if v.Tag.Type != "" {
			cond += ` AND t.type = ?`
			args = append(args, v.Tag.Type)
		}
		cond += `)`
		if f.Op == search.OpNotHas {
			cond = `NOT ` + cond
		}
		return cond, args, nil
	case search.FieldTitle:
		t := search.Normalize(v.Text)
		return `d.title_norm LIKE ?` + escapeClause(t), []any{"%" + escapeLike(t) + "%"}, nil
	case search.FieldScanlator:
		want := search.Normalize(strings.TrimSpace(v.Text))
		if f.Op == search.OpEq {
			return `d.scan_norm <> '' AND d.scan_norm = ?`, []any{want}, nil
		}
		return `d.scan_norm <> '' AND d.scan_norm LIKE ?` + escapeClause(want), []any{"%" + escapeLike(want) + "%"}, nil
	case search.FieldID:
		return numCond("d.ext_id", f)
	case search.FieldPages:
		return numCond("d.pages", f)
	case search.FieldFavorites:
		return numCond("d.favorites", f)
	case search.FieldSize:
		return numCond("d.size", f)
	case search.FieldUploaded:
		return timeCond("d.uploaded", f)
	case search.FieldAdded:
		return timeCond("d.added", f)
	}
	return "", nil, fmt.Errorf("поиск: поле %v не поддерживается каталогом", f.Field)
}

// numCond: нулевое значение (поля нет) не проходит — как cmpNum в MemIndex.
func numCond(col string, f search.Filter) (string, []any, error) {
	v := f.Value
	switch f.Op {
	case search.OpEq:
		return col + ` > 0 AND ` + col + ` = ?`, []any{v.Num}, nil
	case search.OpLt:
		return col + ` > 0 AND ` + col + ` < ?`, []any{v.Num}, nil
	case search.OpGt:
		return col + ` > 0 AND ` + col + ` > ?`, []any{v.Num}, nil
	case search.OpBetween:
		return col + ` > 0 AND ` + col + ` BETWEEN ? AND ?`, []any{v.Num, v.Num2}, nil
	}
	return "0", nil, nil
}

// timeCond: NULL (даты нет) не проходит — как cmpTime в MemIndex.
func timeCond(col string, f search.Filter) (string, []any, error) {
	v := f.Value
	switch f.Op {
	case search.OpLt:
		return col + ` IS NOT NULL AND ` + col + ` < ?`, []any{v.Time.UnixNano()}, nil
	case search.OpGt:
		return col + ` IS NOT NULL AND ` + col + ` > ?`, []any{v.Time.UnixNano()}, nil
	case search.OpBetween:
		return col + ` IS NOT NULL AND ` + col + ` BETWEEN ? AND ?`, []any{v.Time.UnixNano(), v.Time2.UnixNano()}, nil
	}
	return "0", nil, nil
}

// escapeLike экранирует шаблонные символы LIKE: % и _ и сам «\» — обычные.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// escapeClause — ESCAPE нужен только при спецсимволах: без него FTS5
// trigram использует свой индекс для LIKE.
func escapeClause(s string) string {
	if strings.ContainsAny(s, `\%_`) {
		return ` ESCAPE '\'`
	}
	return ""
}

func (x Index) SuggestTags(ctx context.Context, tagType, prefix string, limit int) ([]search.TagCount, error) {
	prefix = search.Normalize(strings.TrimSpace(prefix))
	tagType = strings.ToLower(strings.TrimSpace(tagType))
	cond := `name_norm LIKE ?` + escapeClause(prefix)
	args := []any{escapeLike(prefix) + "%"}
	if tagType != "" {
		cond += ` AND type = ?`
		args = append(args, tagType)
	}
	if limit <= 0 {
		limit = -1
	}
	rows, err := x.c.db.QueryContext(ctx, `SELECT type, name, COUNT(*) AS n FROM tags WHERE `+cond+
		` GROUP BY type, name ORDER BY n DESC, type || ':' || name ASC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("подсказки тегов: %w", err)
	}
	defer rows.Close()
	out := []search.TagCount{}
	for rows.Next() {
		var tc search.TagCount
		if err := rows.Scan(&tc.Tag.Type, &tc.Tag.Name, &tc.Count); err != nil {
			return nil, err
		}
		out = append(out, tc)
	}
	return out, rows.Err()
}
