package browser

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// ErrUnsupported — встроенный браузер есть только на Windows.
var ErrUnsupported = errors.New("встроенный браузер доступен только на Windows")

// Интервалы: проверка новых окон и выхода процесса; ожидание штатного
// закрытия перед принудительным завершением.
const (
	pollInterval = 500 * time.Millisecond
	closeTimeout = 5 * time.Second
	// remoteReady — через сколько после запуска Firefox принимает адреса от
	// второго запуска; раньше адрес теряется (окно уже есть, приём — нет).
	remoteReady = 5 * time.Second
)

// Options — пути и источники настроек браузера.
type Options struct {
	FirefoxDir  string
	ProfileDir  string
	DownloadDir string
	// Prefs возвращает текущие настройки (домашняя страница, запрошенные
	// очистки); вызывается перед каждым запуском.
	Prefs func() (home string, clear []string)
	// ClearApplied вызывается, когда запрошенные очистки записаны в user.js
	// (Firefox выполнит их при этом запуске).
	ClearApplied func()
	// OnState вызывается из фоновой горутины при запуске и закрытии браузера.
	OnState func(running bool)
	// OnDownload — браузер скачал файл и сообщил адрес страницы (путь
	// относительно DownloadDir); OnShow — нажата кнопка «MangaReader».
	// Вызываются из горутин моста.
	OnDownload func(relPath string)
	OnShow     func()
}

// Browser — встроенный браузер: один экземпляр Firefox с профилем приложения.
// Методы безопасны для вызова из разных горутин.
type Browser struct {
	opt Options
	exe string

	mu       sync.Mutex
	owner    uintptr // HWND главного окна; 0 — не задан
	watching bool    // горутина наблюдения запущена
	attached map[uintptr]bool
	before   []string  // подключи Installer в реестре до запуска (nil — неизвестны)
	started  time.Time // когда запущен браузер (этим приложением)
	bridge   *Bridge   // мост к расширению на время сеанса браузера
}

// New создаёт браузер; ничего не запускает.
func New(opt Options) *Browser {
	return &Browser{opt: opt, exe: filepath.Join(opt.FirefoxDir, "firefox.exe"), attached: map[uintptr]bool{}}
}

// SetHandlers задаёт Options.OnDownload и Options.OnShow (до запуска браузера).
func (b *Browser) SetHandlers(onDownload func(relPath string), onShow func()) {
	b.mu.Lock()
	b.opt.OnDownload, b.opt.OnShow = onDownload, onShow
	b.mu.Unlock()
}

// SetOnState задаёт Options.OnState (вызывается из фоновой горутины).
func (b *Browser) SetOnState(f func(running bool)) {
	b.mu.Lock()
	b.opt.OnState = f
	b.mu.Unlock()
}

func (b *Browser) onState(running bool) {
	b.mu.Lock()
	f := b.opt.OnState
	b.mu.Unlock()
	if f != nil {
		f(running)
	}
}

// Exe — путь к firefox.exe.
func (b *Browser) Exe() string { return b.exe }

// Available — firefox.exe на месте.
func (b *Browser) Available() bool {
	_, err := os.Stat(b.exe)
	return err == nil
}

// SetOwner задаёт главное окно (HWND), к которому привязываются окна браузера.
func (b *Browser) SetOwner(hwnd uintptr) {
	b.mu.Lock()
	b.owner = hwnd
	b.mu.Unlock()
}

// Running — открыто хотя бы одно окно браузера или жив его процесс.
func (b *Browser) Running() bool {
	return len(findWindows(b.exe)) > 0 || countProcs(b.exe) > 0
}

// Open открывает url (или домашнюю страницу при url == "") во встроенном
// браузере: запускает его или передаёт адрес уже открытому экземпляру и
// выводит его окно на передний план.
func (b *Browser) Open(url string) error {
	if !supported {
		return ErrUnsupported
	}
	if !b.Available() {
		return fmt.Errorf("Браузер не найден: %s", b.exe)
	}
	if b.Running() {
		b.mu.Lock()
		wait := remoteReady - time.Since(b.started)
		b.mu.Unlock()
		if url != "" && wait > 0 {
			time.Sleep(wait)
		}
		if url != "" {
			// без -no-remote Firefox передаёт адрес запущенному экземпляру
			// с тем же профилем и сразу завершается
			if err := b.command(url).Run(); err != nil {
				return fmt.Errorf("передача адреса в браузер: %w", err)
			}
		}
		for _, h := range findWindows(b.exe) {
			foreground(h)
			break
		}
		b.watch()
		return nil
	}
	if err := b.writeConfig(); err != nil {
		return err
	}
	b.mu.Lock()
	b.before = installerKeys()
	b.started = time.Now()
	b.mu.Unlock()
	cmd := b.command(url)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("запуск браузера: %w", err)
	}
	go cmd.Wait() // процесс-лаунчер: окно принадлежит дочернему процессу
	b.watch()
	return nil
}

func (b *Browser) command(url string) *exec.Cmd {
	args := []string{"-profile", b.opt.ProfileDir}
	if url != "" {
		args = append(args, url)
	}
	cmd := exec.Command(b.exe, args...)
	// отчёты о сбоях иначе пишутся в %APPDATA%\Mozilla
	cmd.Env = append(os.Environ(), "MOZ_CRASHREPORTER_DISABLE=1")
	return cmd
}

// startBridge запускает мост к расширению на сеанс браузера (если ещё не
// запущен). Без моста браузер работает, но адрес страницы загрузки не
// запоминается.
func (b *Browser) startBridge() (port int, token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.bridge == nil {
		onDownload := func(rel string) {
			b.mu.Lock()
			f := b.opt.OnDownload
			b.mu.Unlock()
			if f != nil {
				f(rel)
			}
		}
		onShow := func() {
			b.mu.Lock()
			f, owner := b.opt.OnShow, b.owner
			b.mu.Unlock()
			// браузер уже свернут расширением — показать главное окно
			if owner != 0 {
				foreground(owner)
			}
			if f != nil {
				f()
			}
		}
		br, err := NewBridge(b.opt.DownloadDir, onDownload, onShow)
		if err != nil {
			log.Printf("браузер: мост к расширению: %v", err)
			return 0, ""
		}
		b.bridge = br
	}
	return b.bridge.Port(), b.bridge.Token()
}

func (b *Browser) stopBridge() {
	b.mu.Lock()
	br := b.bridge
	b.bridge = nil
	b.mu.Unlock()
	if br != nil {
		br.Close()
	}
}

// writeConfig записывает user.js, userChrome.css и расширение перед
// запуском и удаляет policies.json прежних версий.
func (b *Browser) writeConfig() error {
	cfg := Config{FirefoxDir: b.opt.FirefoxDir, ProfileDir: b.opt.ProfileDir, DownloadDir: b.opt.DownloadDir}
	if b.opt.Prefs != nil {
		cfg.Home, cfg.Clear = b.opt.Prefs()
	}
	if err := os.Remove(cfg.LegacyPoliciesPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("настройки браузера: %w", err)
	}
	ext, err := Extension(b.startBridge())
	if err != nil {
		return fmt.Errorf("расширение браузера: %w", err)
	}
	files := []struct {
		path string
		data []byte
	}{
		{filepath.Join(cfg.ProfileDir, "user.js"), UserJS(cfg)},
		{filepath.Join(cfg.ProfileDir, "chrome", "userChrome.css"), UserChrome()},
		{ExtensionPath(cfg.ProfileDir), ext},
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return fmt.Errorf("настройки браузера: %w", err)
		}
		if err := os.WriteFile(f.path, f.data, 0o644); err != nil {
			return fmt.Errorf("настройки браузера: %w", err)
		}
	}
	if len(cfg.Clear) > 0 && b.opt.ClearApplied != nil {
		b.opt.ClearApplied()
	}
	return nil
}

// watch запускает (однократно) горутину, которая привязывает новые окна
// браузера к главному окну и после выхода браузера убирает его записи в
// реестре.
func (b *Browser) watch() {
	b.mu.Lock()
	if b.watching {
		b.mu.Unlock()
		return
	}
	b.watching = true
	b.mu.Unlock()
	go b.loop()
}

func (b *Browser) loop() {
	b.onState(true)
	start := time.Now()
	for {
		wins := findWindows(b.exe)
		b.mu.Lock()
		owner := b.owner
		for _, h := range wins {
			if owner != 0 && !b.attached[h] {
				attach(h, owner)
				b.attached[h] = true
				if len(b.attached) == 1 {
					foreground(h)
				}
			}
		}
		b.mu.Unlock()
		// процесс-лаунчер мог ещё не создать окно: ждём, пока жив хоть один процесс
		if len(wins) == 0 && countProcs(b.exe) == 0 && time.Since(start) > 2*time.Second {
			break
		}
		time.Sleep(pollInterval)
	}
	b.mu.Lock()
	b.watching = false
	b.attached = map[uintptr]bool{}
	b.mu.Unlock()
	b.stopBridge()
	b.cleanup()
	b.onState(false)
}

// cleanup убирает записи Firefox в реестре (см. CleanRegistry).
func (b *Browser) cleanup() {
	b.mu.Lock()
	before := b.before
	b.before = nil
	b.mu.Unlock()
	if n, err := CleanRegistry(b.opt.FirefoxDir, before); err != nil {
		log.Printf("браузер: уборка реестра: %v", err)
	} else if n > 0 {
		log.Printf("браузер: удалено записей реестра: %d", n)
	}
}

// Close закрывает браузер: WM_CLOSE всем окнам, затем, если за closeTimeout
// процессы не завершились, — принудительно. Блокирует до завершения.
func (b *Browser) Close() {
	if !supported || !b.Running() {
		b.stopBridge()
		return
	}
	for _, h := range findWindows(b.exe) {
		closeWindow(h)
	}
	deadline := time.Now().Add(closeTimeout)
	for countProcs(b.exe) > 0 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if countProcs(b.exe) > 0 {
		log.Printf("браузер: не закрылся за %v — завершается принудительно", closeTimeout)
		killProcs(b.exe)
	}
	b.stopBridge()
	b.cleanup()
}
