package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"mangareader/internal/model"
	"mangareader/internal/userdata"
)

// Ошибки правки тегов; текст для пользователя — в UI.
var (
	// ErrTagExists — у произведения уже есть тег того же типа и имени
	// (оригинальный, в том числе скрытый, или свой).
	ErrTagExists = errors.New("the work already has this tag")
	// ErrTagInvalid — недопустимое имя тега: пустое, длиннее MaxTagName
	// символов или с кавычкой.
	ErrTagInvalid = errors.New("invalid tag name")
	// ErrUserDataUnavailable — пользовательские данные недоступны.
	ErrUserDataUnavailable = errors.New("user data is unavailable")
)

// MaxTagName — наибольшая длина имени своего тега в символах.
const MaxTagName = 100

// TagOpKind — вид правки тегов.
type TagOpKind uint8

const (
	TagAdd    TagOpKind = iota + 1 // добавить свой тег
	TagRemove                      // удалить свой тег
	TagHide                        // скрыть оригинальный тег
	TagUnhide                      // вернуть скрытый тег
	TagReset                       // удалить все свои и вернуть все скрытые
)

// TagOp — правка тегов произведения; Tag не нужен для TagReset.
type TagOp struct {
	Kind TagOpKind
	Tag  model.Tag
}

// ValidateTagName проверяет имя своего тега (после нормализации, как у
// оригинальных тегов): ErrTagInvalid — пустое, длиннее MaxTagName или с «"».
func ValidateTagName(name string) error {
	n := model.NewTag("", name).Name
	if n == "" || utf8.RuneCountInString(n) > MaxTagName || strings.Contains(n, `"`) {
		return ErrTagInvalid
	}
	return nil
}

// TagsAvailable сообщает, можно ли править теги (пользовательские данные
// открыты).
func (s *Services) TagsAvailable() bool { return s.UserData != nil }

// EditTags применяет правку тегов произведения k и возвращает галерею с
// новым наложением; индекс поиска обновлён. Обращается к базе — не
// вызывать из UI-потока.
func (s *Services) EditTags(k model.Key, op TagOp) (model.Gallery, error) {
	if s.UserData == nil {
		return model.Gallery{}, ErrUserDataUnavailable
	}
	if k.Source != model.SourceLocal {
		return model.Gallery{}, fmt.Errorf("gallery %s is not a local file", k)
	}
	g, ok := s.Library.Get(k)
	if !ok {
		return model.Gallery{}, fmt.Errorf("gallery %s not found", k)
	}
	t := model.NewTag(op.Tag.Type, op.Tag.Name)
	if op.Kind == TagAdd {
		if t.Type == "" {
			return model.Gallery{}, ErrTagInvalid
		}
		if err := ValidateTagName(t.Name); err != nil {
			return model.Gallery{}, err
		}
		if g.HasTag(t) || g.IsCustom(t) { // среди оригинальных и скрытые
			return model.Gallery{}, ErrTagExists
		}
	}
	uid, err := s.UserData.Ensure(s.UserDataKey(), k.ID, g.Fingerprint)
	if err != nil {
		return model.Gallery{}, err
	}
	switch op.Kind {
	case TagAdd:
		err = s.UserData.AddCustom(uid, t)
		if errors.Is(err, userdata.ErrDuplicate) {
			err = ErrTagExists
		}
	case TagRemove:
		err = s.UserData.RemoveCustom(uid, t)
	case TagHide:
		err = s.UserData.Hide(uid, t)
	case TagUnhide:
		err = s.UserData.Unhide(uid, t)
	case TagReset:
		err = s.UserData.Reset(uid)
	default:
		err = fmt.Errorf("unknown tag operation %d", op.Kind)
	}
	if err != nil {
		return model.Gallery{}, err
	}
	return s.Library.Refresh(k)
}

// userDataOverlay — наложение тегов из пользовательских данных текущей
// папки библиотеки (library.Overlay).
type userDataOverlay struct{ obs *userDataObserver }

func (a userDataOverlay) Apply(gs []model.Gallery) error {
	store, root := a.obs.store, a.obs.key()
	if len(gs) == 1 { // правка одной галереи — без чтения всей папки
		ov, err := overlayOf(store, root, gs[0].Key.ID)
		if err != nil {
			return err
		}
		gs[0].Custom, gs[0].Hidden = ov.Custom, ov.Hidden
		return nil
	}
	all, err := store.Overlays(root)
	if err != nil {
		return err
	}
	for i := range gs {
		ov := all[gs[i].Key.ID]
		gs[i].Custom, gs[i].Hidden = ov.Custom, ov.Hidden
	}
	return nil
}

func overlayOf(store *userdata.Store, root, rel string) (userdata.Overlay, error) {
	uid, ok, err := store.Lookup(root, rel)
	if err != nil || !ok {
		return userdata.Overlay{}, err
	}
	return store.Overlay(uid)
}

// epochNone — эпоха индекса, построенного без пользовательских данных.
const epochNone = "none"

// loadCatalog показывает библиотеку из каталога до окна и до первого
// сканирования (с наложением пользовательских данных) и проверяет, что
// индекс построен с этими пользовательскими данными. Вызывать после
// attachUserData.
func (s *Services) loadCatalog() {
	if s.Catalog == nil {
		return
	}
	s.Cached = s.Library.LoadCatalog()
	s.checkOverlayEpoch()
}

// checkOverlayEpoch: если пользовательские данные заменены (восстановлены из
// копии, пересозданы, недоступны), индекс хранит чужое наложение — в фоне
// библиотека переиндексируется, затем запоминается новая эпоха.
func (s *Services) checkOverlayEpoch() {
	want := epochNone
	if s.UserData != nil {
		e, err := s.UserData.Epoch()
		if err != nil {
			log.Printf("user data: epoch: %v", err)
			return
		}
		want = e
	}
	have, err := s.Catalog.OverlayEpoch()
	if err != nil {
		log.Printf("catalog: overlay epoch: %v", err)
		return
	}
	if have == want {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.stopBg = cancel
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		if err := s.Library.Reindex(ctx); err != nil {
			log.Printf("user data: reindexing the library: %v", err)
			return
		}
		if err := s.Catalog.SetOverlayEpoch(want); err != nil {
			log.Printf("catalog: overlay epoch: %v", err)
			return
		}
		log.Printf("user data changed: library reindexed with the user's tags")
	}()
}
