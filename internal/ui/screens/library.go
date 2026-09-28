// Package screens содержит экраны приложения.
package screens

import (
	"context"
	"errors"
	"log"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/problems"
	"mangareader/internal/storage"
)

// autoRefreshInterval — не чаще одного автосканирования за этот интервал.
const autoRefreshInterval = 2 * time.Second

// Повторы сканирования, пока файлы заняты другим процессом (антивирус):
// через busyRetryDelay, после busyRetries попыток занятые файлы
// показываются как ошибки.
const (
	busyRetryDelay = 2 * time.Second
	busyRetries    = 5
)

// Library — экран библиотеки: сетка галерей из папки библиотеки.
// Все поля меняются только в UI-потоке.
type Library struct {
	src      *library.Source
	problems *problems.Tracker
	notify   func(string)
	// OnScanned вызывается в UI-потоке после каждого успешного сканирования.
	OnScanned func()
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	scanning bool
	pending  bool // во время скана пришёл запрос — сканировать ещё раз
	scanned  bool // сканирование уже показало результат — каталог устарел
	lastAuto time.Time

	busyTries int           // подряд сканов с занятыми файлами
	busyDelay time.Duration // задержка повтора (меняется в тестах)

	count    *widget.Label
	refresh  *widget.Button
	random   *randomButton
	tools    *fyne.Container // кнопки панели справа
	activity *widget.Activity
	grid     *galleryGrid
	empty    fyne.CanvasObject
	emptyDir *widget.Label
	choose   fyne.CanvasObject // «Выберите папку» (Android), nil на ПК
	content  fyne.CanvasObject
}

// NewLibrary создаёт экран; metrics — размер карточек, общий с поиском;
// actions — действия карточки (нажатие — actions.Open), chooseFolder
// открывает выбор папки (Android; на ПК — nil).
func NewLibrary(svc *app.Services, metrics *GridMetrics, notify func(string), actions *GalleryActions, chooseFolder func()) *Library {
	l := &Library{src: svc.Library, problems: svc.Problems, notify: notify, do: fyne.Do, busyDelay: busyRetryDelay}

	l.count = widget.NewLabel("")
	l.activity = widget.NewActivity()
	l.activity.Hide()
	l.refresh = widget.NewButtonWithIcon(i18n.T("library.refresh"), theme.ViewRefreshIcon(), l.Refresh)
	l.grid = newGalleryGrid(svc.Thumbs, metrics, actions)
	l.random = newRandomButton(app.RandomMode(svc.Settings), l.grid.Items, func(g model.Gallery) { actions.Open(g) })
	l.tools = container.NewHBox(l.random.btn, l.activity, l.refresh)
	toolbar := container.NewBorder(nil, nil, l.count, l.tools)

	l.empty, l.emptyDir = newEmptyState()
	l.grid.Widget().Hide()
	views := container.NewStack(l.empty, l.grid.Widget())
	if chooseFolder != nil {
		l.choose = newChooseState(chooseFolder)
		l.choose.Hide()
		views.Add(l.choose)
	}
	l.content = container.NewBorder(toolbar, nil, nil, nil, views)
	l.updateEmptyDir()
	l.updateCount()
	return l
}

func (l *Library) Content() fyne.CanvasObject { return l.content }

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (l *Library) SetDispatcher(do func(func())) { l.do = do }

// AddTool добавляет кнопку на панель библиотеки (после 🎲, перед «Обновить»).
func (l *Library) AddTool(o fyne.CanvasObject) {
	l.tools.Objects = append([]fyne.CanvasObject{l.tools.Objects[0], o}, l.tools.Objects[1:]...)
	l.tools.Refresh()
}

// SetRandomMode задаёт режим кнопки 🎲 и начинает круг заново.
func (l *Library) SetRandomMode(mode string) { l.random.picker.SetMode(mode) }

// GridColumns — число колонок сетки (для тестов).
func (l *Library) GridColumns() int { return l.grid.grid.ColumnCount() }

// RandomMode — режим кнопки 🎲 (для тестов).
func (l *Library) RandomMode() string { return l.random.picker.mode }

// Refresh запускает сканирование в фоне. Вызывать из UI-потока.
// Повторный вызов во время сканирования игнорируется.
func (l *Library) Refresh() {
	if l.scanning {
		return
	}
	l.scanning = true
	l.refresh.Disable()
	l.activity.Show()
	l.activity.Start()

	go func() {
		res, err := l.src.Scan(context.Background())
		l.do(func() { l.applyScan(res, err) })
	}()
}

// RequestScan — сканирование по изменению папки. Если сканирование уже
// идёт, после него выполняется ещё одно: изменение не теряется, а
// одновременно идёт не больше одного скана. Вызывать из UI-потока.
func (l *Library) RequestScan() {
	if l.scanning {
		l.pending = true
		return
	}
	l.Refresh()
}

// Remove убирает галерею k из сетки сразу после удаления файла и обновляет
// число галерей и кнопку 🎲; каталог приводит в порядок сканирование.
// Вызывать из UI-потока.
func (l *Library) Remove(k model.Key) {
	if !l.grid.Remove(k) {
		return
	}
	l.updateCount()
	if len(l.grid.Items()) == 0 {
		l.show(l.empty)
	}
}

// Galleries — галереи в сетке (для тестов).
func (l *Library) Galleries() []model.Gallery { return l.grid.Items() }

// RandomButton — кнопка 🎲 (для тестов).
func (l *Library) RandomButton() *widget.Button { return l.random.btn }

// Count — текст числа галерей (для тестов).
func (l *Library) Count() string { return l.count.Text }

// Scanning — идёт сканирование (для тестов).
func (l *Library) Scanning() bool { return l.scanning }

// SetBusyRetryDelay задаёт задержку повтора при занятых файлах (для тестов).
func (l *Library) SetBusyRetryDelay(d time.Duration) { l.busyDelay = d }

// AutoRefresh — сканирование по событиям жизненного цикла (запуск, возврат
// на передний план) с ограничением частоты. Вызывать из UI-потока.
func (l *Library) AutoRefresh() {
	if time.Since(l.lastAuto) < autoRefreshInterval {
		return
	}
	l.lastAuto = time.Now()
	l.Refresh()
}

// ShowCached показывает библиотеку из каталога до окончания первого
// сканирования (галереи и ошибки прошлых запусков). Пустой результат ничего
// не меняет: пустое состояние покажет сканирование; если сканирование уже
// показало свой результат, он свежее. Вызывать из UI-потока.
func (l *Library) ShowCached(res library.ScanResult) {
	if l.scanned || (len(res.Galleries) == 0 && len(res.Errors) == 0) {
		return
	}
	l.grid.SetItems(res.Galleries)
	l.updateCount()
	if len(res.Galleries) > 0 {
		l.show(l.grid.Widget())
	}
	l.problems.Sync(res.Errors) // ошибки прошлых запусков уже известны — без toast
}

func (l *Library) applyScan(res library.ScanResult, err error) {
	l.scanning = false
	l.activity.Stop()
	l.activity.Hide()
	l.refresh.Enable()
	defer func() {
		if l.pending {
			l.pending = false
			l.Refresh()
		}
	}()

	if errors.Is(err, library.ErrScanInProgress) {
		return
	}
	if err != nil {
		log.Printf("library: %v", err)
		if l.choose != nil && errors.Is(err, storage.ErrUnavailable) {
			// Android: папка не выбрана, доступ отозван или папка удалена
			l.grid.SetItems(nil)
			l.updateCount()
			l.show(l.choose)
			return
		}
		l.notify(i18n.T("library.read_failed", "Error", ErrorText(err)))
		return
	}
	l.scanned = true
	l.retryBusy(len(res.Busy))
	l.grid.SetItems(res.Galleries)
	l.updateCount()
	l.updateEmptyDir()
	if len(res.Galleries) == 0 {
		l.show(l.empty)
	} else {
		l.show(l.grid.Widget())
	}
	// toast только о записях, которых ещё не было в списке ошибок
	if n := l.problems.Sync(res.Errors); n > 0 {
		l.notify(i18n.T("library.new_errors", "Count", n))
	}
	if l.OnScanned != nil {
		l.OnScanned()
	}
}

// retryBusy планирует повтор, пока есть занятые файлы; после busyRetries
// попыток подряд занятые файлы показываются как ошибки (новых повторов нет —
// освобождение файла заметит наблюдение за папкой или ручное обновление).
func (l *Library) retryBusy(busy int) {
	if busy == 0 {
		if l.busyTries > 0 {
			l.busyTries = 0
			l.src.SetBusyAsError(false)
		}
		return
	}
	l.busyTries++
	if l.busyTries > busyRetries {
		return
	}
	if l.busyTries == busyRetries {
		l.src.SetBusyAsError(true)
	}
	time.AfterFunc(l.busyDelay, func() { l.do(l.RequestScan) })
}

// show оставляет видимым одно из состояний: пусто, сетка, выбор папки.
func (l *Library) show(o fyne.CanvasObject) {
	for _, v := range []fyne.CanvasObject{l.empty, l.grid.Widget(), l.choose} {
		if v == nil {
			continue
		}
		if v == o {
			v.Show()
			v.Refresh() // Container.Show сам не перерисовывает
		} else {
			v.Hide()
		}
	}
}

func (l *Library) updateEmptyDir() {
	dir := l.src.Root()
	if dir == "" {
		dir = i18n.T("library.folder_not_selected")
	}
	l.emptyDir.SetText(dir)
}

// updateCount показывает число галерей; 🎲 активна, если они есть.
func (l *Library) updateCount() {
	n := len(l.grid.Items())
	l.count.SetText(i18n.N("library.count", n))
	l.random.update(n)
}

// newEmptyState — пустое состояние с папкой библиотеки.
func newEmptyState() (fyne.CanvasObject, *widget.Label) {
	title := widget.NewLabelWithStyle(i18n.T("library.empty.title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	hint := widget.NewLabel(i18n.T("library.empty.hint"))
	hint.Alignment = fyne.TextAlignCenter
	hint.Wrapping = fyne.TextWrapWord

	path := widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Monospace: true})
	path.Wrapping = fyne.TextWrapBreak

	icon := widget.NewIcon(theme.FolderOpenIcon())

	// VBox со спейсерами вместо Center: переносимым надписям нужна вся ширина.
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(64), icon)),
		title, hint, path,
		layout.NewSpacer(),
	), path
}

// newChooseState — «Выберите папку с мангой» с кнопкой выбора (Android).
func newChooseState(choose func()) fyne.CanvasObject {
	title := widget.NewLabelWithStyle(i18n.T("library.choose.title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel(i18n.T("library.choose.hint"))
	hint.Alignment = fyne.TextAlignCenter
	hint.Wrapping = fyne.TextWrapWord
	btn := widget.NewButtonWithIcon(i18n.T("folder.choose"), theme.FolderOpenIcon(), choose)
	btn.Importance = widget.HighImportance
	icon := widget.NewIcon(theme.FolderIcon())
	return container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(64), icon)),
		title, hint, container.NewCenter(btn),
		layout.NewSpacer(),
	)
}
