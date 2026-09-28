package ui

import (
	"errors"

	"fyne.io/fyne/v2"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/storage"
	"mangareader/internal/ui/screens"
)

// newActions — действия меню галереи для сеток и страницы произведения.
// «Открыть в браузере» задаёт setOpenURL (зависит от браузера платформы).
func (s *Shell) newActions() *screens.GalleryActions {
	act := &screens.GalleryActions{
		Open:      s.Details.Open,
		Read:      s.Reader.Open,
		CopyTitle: s.copyTitle,
	}
	if storage.CanReveal {
		act.ShowInFolder = s.showInFolder
	}
	if storage.CanDelete {
		act.Delete = s.Delete
	}
	return act
}

// setOpenURL задаёт открытие ссылки на произведение — одно и то же для
// кнопки страницы произведения и пункта меню карточки.
func (s *Shell) setOpenURL(open func(string)) {
	s.Details.SetOpenURL(open)
	s.actions.OpenURL = func(g model.Gallery) { open(g.SourceURL) }
}

func (s *Shell) copyTitle(g model.Gallery) {
	fyne.CurrentApp().Clipboard().SetContent(g.Title)
	s.Toast.Show(i18n.T("actions.title_copied"))
}

// showInFolder открывает Проводник с выделенным архивом (в фоне: оболочка
// Windows может отвечать не сразу).
func (s *Shell) showInFolder(g model.Gallery) {
	root := s.svc.Library.Root()
	go func() {
		if err := storage.Reveal(root, g.Key.ID); err != nil {
			s.Toast.Show(i18n.T("actions.show_in_folder_failed", "Error", screens.ErrorText(err)))
		}
	}()
}

// inUse — произведение k открыто в читалке: его файл не удаляется.
// Страница произведения открытым произведением не считается.
func (s *Shell) inUse(k model.Key) bool {
	return s.Reader.Visible() && s.Reader.Gallery().Key == k
}

// Delete удаляет произведение g после подтверждения: проверяет, что оно не
// открыто в читалке и что к папке есть доступ на запись, узнаёт способ
// удаления (корзина или безвозвратно), спрашивает пользователя, удаляет
// файл в фоне. После удаления карточка сразу исчезает из библиотеки и
// поиска, страница произведения закрывается, библиотека пересканируется.
// Вызывать из UI-потока.
func (s *Shell) Delete(g model.Gallery) {
	if s.inUse(g.Key) {
		s.Toast.Show(i18n.T("actions.delete.in_use"))
		return
	}
	if s.needsWriteAccess() {
		s.askWriteAccess()
		return
	}
	go func() {
		trash, err := s.svc.Library.CanTrash(g.Key)
		s.do(func() {
			if err != nil {
				s.Toast.Show(deleteErrorText(err))
				return
			}
			s.confirmDelete(g, trash)
		})
	}()
}

// confirmDelete спрашивает подтверждение с текстом по способу удаления.
func (s *Shell) confirmDelete(g model.Gallery, trash bool) {
	var question, button string
	switch {
	case trash:
		question, button = i18n.T("actions.delete.to_trash"), i18n.T("actions.delete.to_trash_button")
	case storage.HasTrash:
		question, button = i18n.T("actions.delete.no_trash"), i18n.T("actions.delete.button")
	default:
		question, button = i18n.T("actions.delete.permanent"), i18n.T("actions.delete.button")
	}
	text := g.Title + "\n" + g.Key.ID + "\n\n" + question
	s.confirm(i18n.T("actions.delete.title"), text, button, func(ok bool) {
		if ok {
			s.deleteFile(g, !trash)
		}
	})
}

func (s *Shell) deleteFile(g model.Gallery, permanent bool) {
	go func() {
		err := s.svc.Library.Delete(g.Key, permanent)
		s.do(func() {
			if err != nil {
				s.Toast.Show(deleteErrorText(err))
				return
			}
			s.removed(g.Key)
		})
	}()
}

// removed убирает удалённое произведение с экранов сразу, не дожидаясь
// сканирования; каталог, ошибки и ссылки приводит в порядок сканирование.
func (s *Shell) removed(k model.Key) {
	if s.Details.Visible() && s.Details.Gallery().Key == k {
		s.Details.Close()
	}
	s.library.Remove(k)
	s.search.Remove(k)
	s.library.RequestScan()
}

// deleteErrorText — текст toast при ошибке удаления; занятый файл — без
// общего пояснения.
func deleteErrorText(err error) string {
	if errors.Is(err, storage.ErrBusy) {
		return screens.ErrorText(err)
	}
	return i18n.T("actions.delete.failed", "Error", screens.ErrorText(err))
}

// askWriteAccess объясняет, что папку нужно выбрать заново с доступом на
// запись (Android: папка выбрана до появления браузера и удаления), и
// открывает системный выбор папки.
func (s *Shell) askWriteAccess() {
	s.confirm(i18n.T("shell.write_access.title"), i18n.T("shell.write_access.text"), i18n.T("folder.choose"),
		func(ok bool) {
			if ok {
				s.ChooseFolder()
			}
		})
}
