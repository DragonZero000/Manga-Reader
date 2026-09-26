//go:build !sqlite_fts5

package catalog

// Каталогу нужен SQLite с полнотекстовым поиском FTS5: собирайте с тегом
// sqlite_fts5 (make test, make run, make build-* добавляют его сами):
//
//	go test -tags sqlite_fts5 ./...
var _ = каталогу_нужен_тег_сборки_sqlite_fts5
