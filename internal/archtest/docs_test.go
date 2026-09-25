package archtest

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Документация на двух языках: пары в корне (README.md ↔ README.ru.md) и
// одинаковые наборы файлов в docs/en и docs/ru. Проверки — без сети.

// rootPairs — двуязычные документы в корне: английский → русский.
var rootPairs = map[string]string{
	"README.md":       "README.ru.md",
	"CONTRIBUTING.md": "CONTRIBUTING.ru.md",
}

// linkCheckedOnly — документы с одним языком, в которых проверяются только ссылки.
var linkCheckedOnly = []string{"THIRD_PARTY_NOTICES.md"}

// switcherLines — в скольких первых строках искать переключатель языка.
const switcherLines = 5

// repoRoot — корень репозитория: ближайшая вверх папка с go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("не найден корень репозитория (go.mod)")
		}
		dir = parent
	}
}

// mdNames — имена *.md в папке (без подпапок).
func mdNames(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("папка документации: %v", err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names[e.Name()] = true
		}
	}
	return names
}

// bilingualDocs — пары документов (путь относительно корня, с «/»): документ → его пара.
func bilingualDocs(t *testing.T, root string) map[string]string {
	t.Helper()
	pairs := map[string]string{}
	for en, ru := range rootPairs {
		pairs[en], pairs[ru] = ru, en
	}
	en := mdNames(t, filepath.Join(root, "docs", "en"))
	ru := mdNames(t, filepath.Join(root, "docs", "ru"))
	for name := range en {
		pairs["docs/en/"+name] = "docs/ru/" + name
	}
	for name := range ru {
		pairs["docs/ru/"+name] = "docs/en/" + name
	}
	return pairs
}

func TestDocsHaveTranslations(t *testing.T) {
	root := repoRoot(t)
	pairs := bilingualDocs(t, root)
	docs := make([]string, 0, len(pairs))
	for doc := range pairs {
		docs = append(docs, doc)
	}
	sort.Strings(docs)
	for _, doc := range docs {
		pair := pairs[doc]
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(doc))); err != nil {
			t.Errorf("нет %s (пара для %s)", doc, pair)
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(pair))); err != nil {
			t.Errorf("для %s нет перевода %s", doc, pair)
		}
	}
}

func TestDocsHaveLanguageSwitcher(t *testing.T) {
	root := repoRoot(t)
	for doc, pair := range bilingualDocs(t, root) {
		lines, err := readLines(filepath.Join(root, filepath.FromSlash(doc)))
		if err != nil {
			continue // отсутствие файла сообщает TestDocsHaveTranslations
		}
		// ссылка на пару относительно самого документа
		rel, _ := filepath.Rel(filepath.Dir(filepath.FromSlash(doc)), filepath.FromSlash(pair))
		want := filepath.ToSlash(rel)
		found := false
		for i := 0; i < len(lines) && i < switcherLines; i++ {
			for _, l := range findLinks(lines[i]) {
				if strings.TrimPrefix(l, "./") == want {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("%s: в первых %d строках нет переключателя языка — ссылки на %s", doc, switcherLines, want)
		}
	}
}

func TestDocsLinksExist(t *testing.T) {
	root := repoRoot(t)
	docs := linkCheckedOnly
	for doc := range bilingualDocs(t, root) {
		docs = append(docs, doc)
	}
	sort.Strings(docs)
	for _, doc := range docs {
		path := filepath.Join(root, filepath.FromSlash(doc))
		lines, err := readLines(path)
		if err != nil {
			continue
		}
		for _, bad := range brokenLinks(lines, filepath.Dir(path), root) {
			t.Errorf("%s:%s", doc, bad)
		}
	}
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// brokenLinks — «строка: цель» для относительных ссылок, которые ведут в никуда.
// dir — папка документа; ссылки с «/» в начале считаются от корня репозитория.
func brokenLinks(lines []string, dir, root string) []string {
	var bad []string
	for n, target := range linksOutsideCode(lines) {
		for _, l := range target {
			p, ok := localPath(l)
			if !ok {
				continue
			}
			base := dir
			if strings.HasPrefix(p, "/") {
				base = root
			}
			if _, err := os.Stat(filepath.Join(base, filepath.FromSlash(p))); err != nil {
				bad = append(bad, fmt.Sprintf("%d: ссылка на несуществующий %s", n, l))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

var (
	reMdLink  = regexp.MustCompile(`\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"]*")?\s*\)`)
	reImgSrc  = regexp.MustCompile(`<img\b[^>]*\bsrc\s*=\s*"([^"]+)"`)
	reInline  = regexp.MustCompile("`[^`]*`")
	reScheme  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
	reFenceOn = regexp.MustCompile("^\\s*(```|~~~)")
)

// linksOutsideCode — ссылки по номерам строк (с 1), кроме блоков кода,
// строк с отступом в 4 пробела и `inline code`.
func linksOutsideCode(lines []string) map[int][]string {
	out := map[int][]string{}
	fence := ""
	for i, line := range lines {
		if m := reFenceOn.FindStringSubmatch(line); m != nil {
			switch {
			case fence == "":
				fence = m[1]
			case m[1] == fence:
				fence = ""
			}
			continue
		}
		if fence != "" || strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if links := findLinks(line); len(links) > 0 {
			out[i+1] = links
		}
	}
	return out
}

// findLinks — цели Markdown-ссылок, изображений и <img src> в строке (без inline code).
func findLinks(line string) []string {
	line = reInline.ReplaceAllString(line, "")
	var links []string
	for _, m := range reMdLink.FindAllStringSubmatch(line, -1) {
		links = append(links, m[1])
	}
	for _, m := range reImgSrc.FindAllStringSubmatch(line, -1) {
		links = append(links, m[1])
	}
	return links
}

// localPath — путь к файлу для относительной ссылки; false для внешних
// ссылок (со схемой) и якорей внутри страницы.
func localPath(link string) (string, bool) {
	if reScheme.MatchString(link) || strings.HasPrefix(link, "#") || strings.HasPrefix(link, "//") {
		return "", false
	}
	if i := strings.IndexAny(link, "#?"); i >= 0 {
		link = link[:i]
	}
	if link == "" {
		return "", false
	}
	if p, err := url.PathUnescape(link); err == nil {
		link = p
	}
	return link, true
}
