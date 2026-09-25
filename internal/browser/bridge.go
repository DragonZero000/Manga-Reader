package browser

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// maxBridgeBody ограничивает размер запроса расширения.
const maxBridgeBody = 64 << 10

// tokenHeader — заголовок с токеном моста.
const tokenHeader = "X-MangaReader-Token"

// Bridge — HTTP-сервер на 127.0.0.1, через который встроенное расширение
// сообщает приложению о загрузках и нажатии «MangaReader». Запросы без
// токена текущего запуска отклоняются.
type Bridge struct {
	ln          net.Listener
	srv         *http.Server
	token       string
	downloadDir string
	onDownload  func(relPath string)
	onShow      func()
}

// NewBridge запускает мост. onDownload получает путь скачанного файла
// относительно downloadDir (с прямыми слешами) после записи адреса
// страницы; onShow — нажатие «MangaReader». Колбэки вызываются из горутин
// сервера.
func NewBridge(downloadDir string, onDownload func(relPath string), onShow func()) (*Bridge, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		ln.Close()
		return nil, err
	}
	b := &Bridge{ln: ln, token: hex.EncodeToString(raw), downloadDir: downloadDir, onDownload: onDownload, onShow: onShow}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /download", b.download)
	mux.HandleFunc("POST /show", b.show)
	b.srv = &http.Server{Handler: b.auth(mux)}
	go b.srv.Serve(ln)
	return b, nil
}

// Port — порт моста.
func (b *Bridge) Port() int { return b.ln.Addr().(*net.TCPAddr).Port }

// Token — токен текущего запуска (передаётся расширению).
func (b *Bridge) Token() string { return b.token }

// Close останавливает мост.
func (b *Bridge) Close() error { return b.srv.Close() }

func (b *Bridge) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(tokenHeader)), []byte(b.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBridgeBody)
		next.ServeHTTP(w, r)
	})
}

// download: {"file": полный путь, "page": адрес страницы} — адрес
// записывается к файлу (storage.WriteSourceURL).
func (b *Bridge) download(w http.ResponseWriter, r *http.Request) {
	var req struct {
		File string `json:"file"`
		Page string `json:"page"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rel, ok := insideDir(b.downloadDir, req.File)
	page := model.WebURL(req.Page)
	if !ok || page == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := storage.WriteSourceURL(req.File, page); err != nil {
		log.Printf("браузер: адрес страницы для %s: %v", rel, err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	if b.onDownload != nil {
		b.onDownload(rel)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (b *Bridge) show(w http.ResponseWriter, r *http.Request) {
	io.Copy(io.Discard, r.Body)
	if b.onShow != nil {
		b.onShow()
	}
	w.WriteHeader(http.StatusNoContent)
}

// insideDir возвращает путь file относительно dir (с прямыми слешами), если
// file — абсолютный путь внутри dir.
func insideDir(dir, file string) (string, bool) {
	if dir == "" || !filepath.IsAbs(file) {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(file))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}
