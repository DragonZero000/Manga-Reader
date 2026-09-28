package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// Тексты интерфейса берутся из файлов переводов (internal/i18n), поэтому в
// не-тестовом коде UI и точки входа не должно быть строковых литералов с
// кириллицей; комментарии не проверяются.
func TestNoCyrillicLiteralsInUI(t *testing.T) {
	for _, root := range []string{"../ui", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && hasCyrillic(lit.Value) {
					t.Errorf("%s: строка %s — текст интерфейса должен браться из i18n", fset.Position(lit.Pos()), lit.Value)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
