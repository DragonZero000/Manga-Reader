package browser

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
)

// ExtensionID — id встроенного расширения MangaReader.
const ExtensionID = "bridge@mangareader.app"

// extensionWidget — id кнопки расширения в шапке Firefox (id расширения,
// в котором всё, кроме букв и цифр, заменено на «_», + «-browser-action»).
const extensionWidget = "bridge_mangareader_app-browser-action"

//go:embed ext
var extFiles embed.FS

// ExtensionPath — файл расширения в профиле: Firefox устанавливает его
// из папки extensions профиля (xpinstall.signatures.required=false, ESR).
func ExtensionPath(profileDir string) string {
	return filepath.Join(profileDir, "extensions", ExtensionID+".xpi")
}

// Extension собирает xpi встроенного расширения с портом и токеном моста
// (config.js). Архив детерминирован: одинаковые port и token дают
// одинаковые байты.
func Extension(port int, token string) ([]byte, error) {
	var names []string
	err := fs.WalkDir(extFiles, "ext", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	for _, p := range names {
		data, err := extFiles.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if err := add(p[len("ext/"):], data); err != nil {
			return nil, err
		}
	}
	config := fmt.Sprintf("// Создаётся MangaReader при запуске браузера.\nconst BRIDGE = {port: %d, token: %q};\n", port, token)
	if err := add("config.js", []byte(config)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
