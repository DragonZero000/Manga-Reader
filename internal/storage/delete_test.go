package storage

import (
	"context"
	"testing"
)

func TestCleanRel(t *testing.T) {
	for _, rel := range []string{"a.zip", "series/vol1.zip", "vol 1, part 2.zip"} {
		if got, err := cleanRel(rel); err != nil || got != rel {
			t.Errorf("%q: %q %v", rel, got, err)
		}
	}
	for _, rel := range []string{"", ".", "..", "../a.zip", "a/../../b.zip", "/a.zip", "a//b.zip", "./a.zip", "a/"} {
		if _, err := cleanRel(rel); err == nil {
			t.Errorf("%q: ожидалась ошибка", rel)
		}
	}
}

type fakeDeleter struct{ Storage }

func (fakeDeleter) CanTrash(string) (bool, error) { return false, nil }
func (fakeDeleter) Delete(string, bool) error     { return nil }

type wrapped struct{ Storage }

func (w wrapped) Unwrap() Storage { return w.Storage }

type plain struct{}

func (plain) Name() string                                  { return "" }
func (plain) Walk(context.Context, func(Entry) error) error { return nil }
func (plain) Open(string) (File, error)                     { return nil, ErrUnsupported }

func TestDeleterOf(t *testing.T) {
	if _, ok := DeleterOf(plain{}); ok {
		t.Error("хранилище без удаления")
	}
	if _, ok := DeleterOf(nil); ok {
		t.Error("nil")
	}
	if _, ok := DeleterOf(wrapped{wrapped{fakeDeleter{plain{}}}}); !ok {
		t.Error("удаление в обёртке не найдено")
	}
}
