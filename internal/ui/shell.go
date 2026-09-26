// Package ui содержит оболочку приложения: окно, навигацию и уведомления.
package ui

import (
	"context"
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/ui/details"
	"mangareader/internal/ui/reader"
	"mangareader/internal/ui/screens"
)

// Shell — главное окно: вкладки экранов и слой уведомлений поверх них.
type Shell struct {
	svc    *app.Services
	Window fyne.Window
	Tabs   *container.AppTabs
	Toast  *Toast

	Reader  *reader.Reader
	Details *details.Details

	searchTab *container.TabItem
	errorsTab *container.TabItem
	watching  bool     // наблюдение за папкой запущено
	onStopped []func() // действия при выходе

	library  *screens.Library
	search   *screens.Search
	errors   *screens.Errors
	settings *screens.Settings
}

// NewShell создаёт окно и все экраны. Экраны создаются один раз и
// сохраняют состояние при переключении вкладок.
func NewShell(a fyne.App, svc *app.Services) *Shell {
	s := &Shell{Window: a.NewWindow("MangaReader")}
	mobile := fyne.CurrentDevice().IsMobile()

	var bottomInset float32
	if mobile {
		bottomInset = theme.IconInlineSize() * 3 // высота нижних вкладок
	}
	s.Toast = NewToast(bottomInset)

	s.svc = svc
	var choose func()
	if svc.CanChooseFolder {
		choose = s.ChooseFolder
	}
	s.Reader = reader.New(a, s.Window, svc.Library, svc.Settings, s.Toast.Show)
	s.Details = details.New(s.Window, svc.Library, svc.Thumbs, s.Reader.Open, s.SearchFor)
	s.library = screens.NewLibrary(svc, s.Toast.Show, s.Details.Open, choose)
	s.search = screens.NewSearch(svc, s.Details.Open)
	s.errors = screens.NewErrors(svc.Problems)
	// поиск и ошибки отражают содержимое папки после каждого сканирования
	s.library.OnScanned = func() {
		s.search.Rerun()
		s.updateErrors()
		s.svc.PruneLinks()
	}
	s.settings = screens.NewSettings(a, s.Window, svc, s.Toast.Show, choose, s.search.SetMode)
	s.setupBrowser(a)

	s.searchTab = container.NewTabItemWithIcon("Поиск", theme.SearchIcon(), s.search.Content())
	s.errorsTab = container.NewTabItemWithIcon("Ошибки", theme.ErrorIcon(), s.errors.Content())
	s.Tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Библиотека", theme.StorageIcon(), s.library.Content()),
		s.searchTab,
		s.errorsTab,
		container.NewTabItemWithIcon("Настройки", theme.SettingsIcon(), s.settings.Content()),
	)
	s.Tabs.OnSelected = func(item *container.TabItem) {
		switch item {
		case s.searchTab:
			s.search.Rerun()
		case s.errorsTab:
			s.updateErrors()
		}
	}
	if mobile {
		s.Tabs.SetTabLocation(container.TabLocationBottom)
	} else {
		s.Tabs.SetTabLocation(container.TabLocationLeading)
	}

	// Слои поверх вкладок: страница произведения, над ней читалка, сверху
	// уведомления. Вкладки под слоями сохраняют состояние.
	s.Window.SetContent(container.NewStack(s.Tabs, s.Details.Layer(), s.Reader.Layer(), s.Toast.Layer()))
	s.Window.Canvas().SetOnTypedKey(s.typedKey)
	s.Window.Resize(fyne.NewSize(960, 640))

	// Сканирование при запуске и при возврате на передний план
	// (например, после копирования архивов по USB на Android).
	a.Lifecycle().SetOnStarted(func() {
		fyne.Do(func() {
			// Android, первый запуск: папка ещё не выбрана — сразу системный выбор
			if svc.CanChooseFolder && !svc.FolderChosen() {
				s.ChooseFolder()
			}
			s.library.AutoRefresh()
			s.attachBrowser()
		})
		s.startWatcher(a)
	})
	a.Lifecycle().SetOnEnteredForeground(func() { fyne.Do(s.library.AutoRefresh) })

	if svc.LibraryErr != nil {
		s.Toast.ShowFor("Не удалось подготовить папку библиотеки: "+svc.LibraryErr.Error(), DefaultToastDuration*2)
	}
	return s
}

// setupBrowser подключает встроенный браузер (Windows): кнопка «Браузер»
// в библиотеке, «Открыть в браузере» на странице произведения, закрытие
// браузера при выходе. Без встроенного браузера ссылка открывается в
// системном браузере.
func (s *Shell) setupBrowser(a fyne.App) {
	if s.svc.MobileBrowser {
		s.setupMobileBrowser()
		return
	}
	b := s.svc.Browser
	if b == nil {
		s.Details.SetOpenURL(func(u string) {
			if pu, err := url.Parse(u); err == nil {
				if err := a.OpenURL(pu); err != nil {
					s.Toast.Show("Не удалось открыть ссылку: " + err.Error())
				}
			}
		})
		return
	}
	// Open может ждать готовности браузера — не в UI-потоке
	open := func(u string) {
		go func() {
			if err := b.Open(u); err != nil {
				s.Toast.Show(err.Error())
			}
		}()
	}
	s.library.AddTool(widget.NewButtonWithIcon("Браузер", theme.ComputerIcon(), func() { open("") }))
	s.Details.SetOpenURL(open)
	// браузер сообщил адрес страницы скачанного файла: разобрать файл заново
	// (Invalidate ждёт идущего сканирования — вызывается в горутине моста)
	b.SetHandlers(func(rel string) {
		s.svc.Library.Invalidate(rel)
		fyne.Do(s.library.RequestScan)
	}, nil)
	b.SetOnState(func(running bool) { fyne.Do(func() { s.settings.SetBrowserRunning(running) }) })
	s.addOnStopped(b.Close)
}

// setupMobileBrowser подключает встроенный браузер Android: кнопка
// «Браузер» в библиотеке, «Открыть в браузере» — в новой вкладке, загрузки
// браузера — адрес страницы и повторный разбор файла.
func (s *Shell) setupMobileBrowser() {
	open := func(u string) {
		if s.svc.NeedsWriteAccess() {
			// папка выбрана до появления браузера — доступ только на чтение
			screens.Confirm("Нужен доступ на запись",
				"Браузер сохраняет загрузки в папку библиотеки. Выберите её ещё раз и разрешите доступ — "+
					"приложение запомнит доступ на чтение и запись.",
				"Выбрать папку",
				func(ok bool) {
					if ok {
						s.ChooseFolder()
					}
				}, s.Window)
			return
		}
		if err := s.svc.OpenMobileBrowser(u); err != nil {
			s.Toast.Show("Не удалось открыть браузер: " + err.Error())
		}
	}
	s.library.AddTool(widget.NewButtonWithIcon("Браузер", theme.ComputerIcon(), func() { open("") }))
	s.Details.SetOpenURL(open)
	mobilebrowser.SetDownloadHandler(func(rel, page string) {
		// поток загрузки Android: Invalidate ждёт идущего сканирования
		s.svc.OnDownloaded(rel, page)
		fyne.Do(s.library.RequestScan)
	})
}

// attachBrowser передаёт браузеру главное окно, к которому привязываются его
// окна. Вызывать из UI-потока после показа окна.
func (s *Shell) attachBrowser() {
	b := s.svc.Browser
	nw, ok := s.Window.(driver.NativeWindow)
	if b == nil || !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		if wc, ok := ctx.(driver.WindowsWindowContext); ok {
			b.SetOwner(wc.HWND)
		}
	})
}

// addOnStopped добавляет действие при выходе из приложения.
func (s *Shell) addOnStopped(f func()) {
	s.onStopped = append(s.onStopped, f)
	fyne.CurrentApp().Lifecycle().SetOnStopped(func() {
		for _, f := range s.onStopped {
			f()
		}
	})
}

// startWatcher запускает наблюдение за папкой библиотеки (ПК): изменения
// в папке пересканируют библиотеку. Останавливается при выходе.
func (s *Shell) startWatcher(a fyne.App) {
	w := s.svc.Watcher
	if w == nil || s.watching {
		return
	}
	s.watching = true
	ctx, cancel := context.WithCancel(context.Background())
	s.addOnStopped(cancel)
	go w.Run(ctx, func() { fyne.Do(s.library.RequestScan) })
}

// ChooseFolder открывает системный выбор папки библиотеки (Android). После
// выбора разрешение делается постоянным, библиотека пересканируется.
// Отмена оставляет прежнюю папку (или экран «Выберите папку»).
func (s *Shell) ChooseFolder() {
	dialog.ShowFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil {
			s.Toast.Show("Не удалось выбрать папку: " + err.Error())
			return
		}
		if lu == nil {
			s.library.Refresh() // покажет «Выберите папку», если папки нет
			return
		}
		if err := s.svc.SetFolder(lu.String()); err != nil {
			s.Toast.Show(err.Error())
			return
		}
		s.settings.Update()
		s.library.Refresh()
	}, s.Window)
}

// updateErrors обновляет экран ошибок и число на вкладке. Открытая вкладка
// «Ошибки» означает, что пользователь видит все записи, — они отмечаются
// просмотренными (в том числе появившиеся, пока вкладка открыта).
func (s *Shell) updateErrors() {
	p := s.svc.Problems
	if s.Tabs.Selected() == s.errorsTab {
		p.MarkAllSeen()
	}
	s.errors.Refresh()
	icon := theme.ErrorIcon()
	if n := p.Unseen(); n > 0 {
		settings := fyne.CurrentApp().Settings()
		th, v := settings.Theme(), settings.ThemeVariant()
		icon = BadgeIcon(icon, n, th.Color(theme.ColorNameForeground, v), th.Color(theme.ColorNameError, v))
	}
	if s.errorsTab.Icon != icon {
		s.errorsTab.Icon = icon
		s.Tabs.Refresh()
	}
}

// Errors — экран ошибок и его вкладка (для тестов).
func (s *Shell) Errors() (*screens.Errors, *container.TabItem) { return s.errors, s.errorsTab }

// SearchFor открывает вкладку «Поиск» с запросом text: закрывает слои
// (страницу произведения, читалку) и сразу выполняет поиск.
func (s *Shell) SearchFor(text string) {
	s.Reader.Close()
	s.Details.Close()
	s.Tabs.Select(s.searchTab)
	s.search.SetQuery(text)
}

// Search — экран поиска (для тестов).
func (s *Shell) Search() *screens.Search { return s.search }

// Settings — экран настроек (для тестов).
func (s *Shell) Settings() *screens.Settings { return s.settings }

// typedKey — клавиши без фокуса. Esc и «Назад» закрывают верхний слой:
// читалку, затем страницу произведения; прочие клавиши уходят в читалку.
// Когда слоёв нет, «Назад» сворачивает приложение: раз обработчик задан,
// Fyne сам этого не делает.
func (s *Shell) typedKey(ev *fyne.KeyEvent) {
	back := ev.Name == fyne.KeyEscape || ev.Name == mobile.KeyBack
	switch {
	case s.Reader.Visible():
		if back {
			s.Reader.Close()
			return
		}
		s.Reader.TypedKey(ev.Name)
	case s.Details.Visible():
		if back {
			s.Details.Close()
		}
	case ev.Name == mobile.KeyBack:
		if d, ok := fyne.CurrentApp().Driver().(interface{ GoBack() }); ok {
			d.GoBack()
		}
	}
}
