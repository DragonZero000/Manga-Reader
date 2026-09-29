package details

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
)

// fakeTags — правка тегов в памяти с той же проверкой, что у app.Services.
type fakeTags struct {
	src      *library.Source
	avail    bool
	mu       sync.Mutex
	custom   map[model.Key][]model.Tag
	hidden   map[model.Key][]model.Tag
	ops      []app.TagOp
	fail     error    // ошибка сохранения для следующих правок
	suggest  []string // имена библиотеки по убыванию числа произведений
	prefixes []string
}

func newFakeTags(src *library.Source) *fakeTags {
	return &fakeTags{src: src, avail: true, custom: map[model.Key][]model.Tag{}, hidden: map[model.Key][]model.Tag{}}
}

func (f *fakeTags) Available() bool { return f.avail }

func (f *fakeTags) gallery(k model.Key) model.Gallery {
	g, _ := f.src.Get(k)
	g.Custom = append([]model.Tag(nil), f.custom[k]...)
	g.Hidden = append([]model.Tag(nil), f.hidden[k]...)
	return g
}

func remove(ts []model.Tag, t model.Tag) []model.Tag {
	var out []model.Tag
	for _, x := range ts {
		if x != t {
			out = append(out, x)
		}
	}
	return out
}

func (f *fakeTags) Edit(k model.Key, op app.TagOp, cb func(model.Gallery, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, op)
	if f.fail != nil {
		cb(model.Gallery{}, f.fail)
		return
	}
	g := f.gallery(k)
	t := model.NewTag(op.Tag.Type, op.Tag.Name)
	switch op.Kind {
	case app.TagAdd:
		if err := app.ValidateTagName(t.Name); err != nil {
			cb(model.Gallery{}, err)
			return
		}
		if g.HasTag(t) || g.IsCustom(t) {
			cb(model.Gallery{}, app.ErrTagExists)
			return
		}
		f.custom[k] = append(f.custom[k], t)
	case app.TagRemove:
		f.custom[k] = remove(f.custom[k], t)
	case app.TagHide:
		f.hidden[k] = append(f.hidden[k], t)
	case app.TagUnhide:
		f.hidden[k] = remove(f.hidden[k], t)
	case app.TagReset:
		f.custom[k], f.hidden[k] = nil, nil
	}
	cb(f.gallery(k), nil)
}

func (f *fakeTags) Suggest(tagType, prefix string, limit int, cb func([]string)) {
	f.mu.Lock()
	f.prefixes = append(f.prefixes, prefix)
	var out []string
	for _, n := range f.suggest {
		if strings.HasPrefix(n, prefix) && len(out) < limit {
			out = append(out, n)
		}
	}
	f.mu.Unlock()
	go cb(out)
}

type tagsFixture struct {
	*fixture
	ed       *fakeTags
	toasts   []string
	confirms []string
	answer   bool
}

func setupTags(t *testing.T) *tagsFixture {
	t.Helper()
	f := &tagsFixture{fixture: setup(t, map[string][]byte{"example.zip": exampleData(t)})}
	f.ed = newFakeTags(f.src)
	f.d.SetTagEditor(f.ed, func(s string) { f.toasts = append(f.toasts, s) })
	f.d.confirm = func(title, text, confirm string, cb func(bool)) {
		f.confirms = append(f.confirms, text)
		cb(f.answer)
	}
	f.d.suggestDelay = 0
	g, _ := f.src.Get(exampleKey)
	f.d.Open(g)
	pump(f.q)
	return f
}

var exampleKey = model.LocalKey("example.zip")

// chip — чип тега name; nil — не показан.
func (f *tagsFixture) chip(name string) *tagChip {
	for _, c := range f.d.ts.chips {
		if c.name == name {
			return c
		}
	}
	return nil
}

// groups — группы в виде «Подпись: имя, имя+, имя-» (+ свой, - скрытый).
func (f *tagsFixture) groups() []string {
	return showGroups(TagGroups(f.d.g.TagViews(), f.d.ts.editing))
}

func showGroups(groups []Group) []string {
	var out []string
	for _, gr := range groups {
		var names []string
		for _, t := range gr.Tags {
			n := t.Name
			switch t.Origin {
			case model.OriginCustom:
				n += "+"
			case model.OriginHidden:
				n += "-"
			}
			names = append(names, n)
		}
		out = append(out, gr.Label+": "+strings.Join(names, ", "))
	}
	return out
}

// shownChips — чипы на странице по порядку с отметками происхождения.
func (f *tagsFixture) shownChips() []string {
	var out []string
	for _, c := range f.d.ts.chips {
		n := c.name
		switch c.origin {
		case model.OriginCustom:
			n += "+"
		case model.OriginHidden:
			n += "-"
		}
		out = append(out, n)
	}
	return out
}

func (f *tagsFixture) enterEdit(t *testing.T) {
	t.Helper()
	test.Tap(f.d.ts.editBtn)
	pump(f.q)
	if !f.d.ts.editing {
		t.Fatal("режим редактирования не включён")
	}
}

func (f *tagsFixture) typeTag(t *testing.T, typ, text string) {
	t.Helper()
	add := f.d.ts.addBtns[typ]
	if add == nil {
		t.Fatalf("нет «+» у группы %s", typ)
	}
	test.Tap(add)
	pump(f.q)
	in := f.d.ts.input
	if in == nil || in.typ != typ {
		t.Fatalf("поле ввода не открыто: %+v", in)
	}
	test.Type(in.entry, text)
}

func (f *tagsFixture) submit() {
	f.d.ts.input.entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	pump(f.q)
}

func TestTagsEnterEditMode(t *testing.T) {
	f := setupTags(t)
	if f.d.ts.editBtn.Text != "Изменить теги" || f.d.ts.editBtn.Disabled() {
		t.Fatalf("кнопка режима: %q, неактивна %v", f.d.ts.editBtn.Text, f.d.ts.editBtn.Disabled())
	}
	for _, c := range f.d.ts.chips {
		if c.action != nil {
			t.Fatalf("✕ у %q вне режима", c.name)
		}
	}
	f.enterEdit(t)
	if f.d.ts.editBtn.Text != "Готово" {
		t.Fatalf("кнопка режима: %q", f.d.ts.editBtn.Text)
	}
	for _, c := range f.d.ts.chips {
		if c.action == nil {
			t.Errorf("нет ✕ у %q", c.name)
		}
	}
	for _, typ := range []string{"artist", "parody", "character", "language", "category", "tag"} {
		if f.d.ts.addBtns[typ] == nil {
			t.Errorf("нет «+» у группы %s", typ)
		}
	}
	if sel := f.d.ts.otherSel; sel == nil || sel.PlaceHolder != "Добавить в другую группу" ||
		!reflect.DeepEqual(sel.Options, []string{"Группа"}) {
		t.Fatalf("«Добавить в другую группу»: %+v", sel)
	}
	if b := f.d.ts.resetBtn; b == nil || b.Text != "Сбросить к оригиналу…" || !b.Disabled() {
		t.Fatalf("«Сбросить к оригиналу…»: %+v", b)
	}

	// нажатие на имя тега в режиме не ищет
	test.Tap(f.chip("artist 1"))
	if len(f.searches) != 0 {
		t.Fatalf("поиск в режиме редактирования: %v", f.searches)
	}

	// закрытие страницы выходит из режима
	f.d.Close()
	g, _ := f.src.Get(exampleKey)
	f.d.Open(g)
	if f.d.ts.editing || f.d.ts.editBtn.Text != "Изменить теги" {
		t.Fatal("режим редактирования сохранился после закрытия страницы")
	}
}

func TestTagsAddToGroup(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	f.typeTag(t, "character", "Alice")
	f.submit()
	if f.d.ts.input != nil {
		t.Fatal("поле ввода не закрылось после добавления")
	}
	want := []string{
		"Автор: artist 1",
		"Пародия: parody 1",
		"Персонаж: character 1, alice+",
		"Язык: japanese",
		"Категория: doujinshi",
		"Теги: tag 1, tag 2, tag 3",
	}
	if got := f.groups(); !reflect.DeepEqual(got, want) {
		t.Fatalf("группы:\n got %q\nwant %q", got, want)
	}
	if c := f.chip("alice"); c == nil || c.origin != model.OriginCustom {
		t.Fatalf("чип своего тега: %+v", c)
	}
	if !f.d.ts.editing || f.d.ts.resetBtn.Disabled() {
		t.Fatal("после добавления: режим и активная кнопка сброса")
	}

	// вне режима свой тег ищет «тип:"имя"»
	test.Tap(f.d.ts.editBtn)
	pump(f.q)
	test.Tap(f.chip("alice"))
	if !reflect.DeepEqual(f.searches, []string{`character:"alice"`}) {
		t.Fatalf("поиск по своему тегу: %q", f.searches)
	}
	if !contains(texts(f.d.body), "alice") {
		t.Fatal("своего тега нет в текстах страницы")
	}
}

func TestTagsAddNewGroup(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	f.d.ts.otherSel.SetSelected("Группа")
	pump(f.q)
	in := f.d.ts.input
	if in == nil || in.typ != "group" {
		t.Fatalf("поле ввода новой группы: %+v", in)
	}
	// пустая группа уже на своём месте в порядке групп
	if got := showGroups(TagGroups(f.d.g.TagViews(), true, "group")); got[1] != "Группа: " {
		t.Fatalf("группы с полем ввода: %q", got)
	}
	test.Type(in.entry, "circle x")
	test.Tap(in.add)
	pump(f.q)
	if got := f.groups(); len(got) != 7 || got[1] != "Группа: circle x+" {
		t.Fatalf("новая группа: %q", got)
	}
}

func TestTagsDuplicateAndInvalid(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	before := f.groups()

	f.typeTag(t, "tag", "Tag 1")
	f.submit()
	if !reflect.DeepEqual(f.toasts, []string{"Такой тег уже есть"}) {
		t.Fatalf("уведомления: %q", f.toasts)
	}
	if got := f.groups(); !reflect.DeepEqual(got, before) {
		t.Fatalf("теги изменились: %q", got)
	}
	if f.d.ts.input == nil {
		t.Fatal("поле ввода закрылось после дубликата")
	}

	// скрытый — тоже дубликат
	test.Tap(f.chip("tag 3").action)
	pump(f.q)
	if f.d.ts.input == nil {
		t.Fatal("поле ввода закрылось после скрытия тега")
	}
	f.d.ts.input.entry.SetText("")
	test.Type(f.d.ts.input.entry, "tag 3")
	f.submit()
	if len(f.toasts) != 2 || f.chip("tag 3").origin != model.OriginHidden {
		t.Fatalf("дубликат скрытого: %q, %v", f.toasts, f.chip("tag 3").origin)
	}

	// то же имя другого типа добавляется
	f.d.ts.input.entry.SetText("")
	test.Type(f.d.ts.input.entry, "japanese")
	f.submit()
	if c := f.chip("japanese"); c == nil {
		t.Fatal("нет japanese")
	}
	if got := f.groups(); got[len(got)-1] != "Теги: tag 1, tag 2, tag 3-, japanese+" {
		t.Fatalf("тег другого типа: %q", got)
	}

	// недопустимые имена: ошибка под полем, правки нет
	ops := len(f.ed.ops)
	for _, name := range []string{"   ", `a"b`} {
		f.typeTag(t, "tag", name)
		in := f.d.ts.input
		f.submit()
		if !in.err.Visible() || !strings.Contains(in.err.Text, "Имя тега") {
			t.Errorf("%q: ошибка под полем %q (видна %v)", name, in.err.Text, in.err.Visible())
		}
		in.entry.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
		pump(f.q)
		if f.d.ts.input != nil {
			t.Fatal("Esc не закрыл поле")
		}
	}
	if len(f.ed.ops) != ops {
		t.Fatalf("недопустимое имя отправлено: %+v", f.ed.ops[ops:])
	}
}

func TestTagsHideAndRestore(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	test.Tap(f.chip("tag 3").action)
	pump(f.q)
	if c := f.chip("tag 3"); c == nil || c.origin != model.OriginHidden {
		t.Fatalf("после ✕: %+v", c)
	}
	test.Tap(f.chip("tag 3").action) // ↺
	pump(f.q)
	if c := f.chip("tag 3"); c == nil || c.origin != model.OriginMeta {
		t.Fatalf("после ↺: %+v", c)
	}

	// единственный тег группы скрыт — вне режима группы нет
	test.Tap(f.chip("japanese").action)
	pump(f.q)
	if got := f.groups(); got[3] != "Язык: japanese-" {
		t.Fatalf("в режиме: %q", got)
	}
	test.Tap(f.d.ts.editBtn)
	pump(f.q)
	for _, g := range f.groups() {
		if strings.HasPrefix(g, "Язык") {
			t.Fatalf("группа «Язык» показана: %q", f.groups())
		}
	}
	if f.chip("japanese") != nil || contains(texts(f.d.body), "Язык") {
		t.Fatal("скрытый тег или его группа показаны вне режима")
	}
}

func TestTagsRemoveCustomAndReset(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	f.typeTag(t, "character", "alice")
	f.submit()
	f.typeTag(t, "tag", "mine")
	f.submit()
	test.Tap(f.chip("tag 2").action)
	pump(f.q)

	test.Tap(f.chip("mine").action)
	pump(f.q)
	if f.chip("mine") != nil {
		t.Fatal("свой тег не удалён")
	}

	// отмена сброса ничего не меняет
	f.answer = false
	test.Tap(f.d.ts.resetBtn)
	pump(f.q)
	if len(f.confirms) != 1 || !strings.Contains(f.confirms[0], "Своих тегов: 1, скрытых: 1") {
		t.Fatalf("диалог: %q", f.confirms)
	}
	if f.chip("alice") == nil || f.chip("tag 2").origin != model.OriginHidden {
		t.Fatal("отмена сброса изменила теги")
	}

	f.answer = true
	test.Tap(f.d.ts.resetBtn)
	pump(f.q)
	if got, want := f.shownChips(), []string{"artist 1", "parody 1", "character 1", "japanese", "doujinshi", "tag 1", "tag 2", "tag 3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("после сброса: %q", got)
	}
	if !f.d.ts.resetBtn.Disabled() {
		t.Fatal("кнопка сброса активна после сброса")
	}
}

func TestTagsSaveFailedAndUnavailable(t *testing.T) {
	f := setupTags(t)
	f.enterEdit(t)
	f.ed.fail = errors.New("disk I/O error")
	test.Tap(f.chip("tag 1").action)
	pump(f.q)
	if len(f.toasts) != 1 || f.toasts[0] != "Не удалось сохранить теги: disk I/O error" {
		t.Fatalf("уведомление: %q", f.toasts)
	}
	if f.chip("tag 1").origin != model.OriginMeta {
		t.Fatal("тег изменился при ошибке сохранения")
	}

	f.d.Close()
	f.ed.avail = false
	g, _ := f.src.Get(exampleKey)
	f.d.Open(g)
	if !f.d.ts.editBtn.Disabled() {
		t.Fatal("«Изменить теги» активна без пользовательских данных")
	}
	if f.chip("tag 1") == nil {
		t.Fatal("теги из meta.json не показаны")
	}
}

func TestTagsSuggestions(t *testing.T) {
	f := setupTags(t)
	f.ed.suggest = []string{"alice", "character 1", "alina"}
	f.enterEdit(t)
	f.typeTag(t, "character", "Ali")
	in := f.d.ts.input
	eventually(t, f.q, "подсказки", func() bool { return len(in.sugg.Objects) == 2 })
	if got := texts(in.sugg); !reflect.DeepEqual(got, []string{"alice", "alina"}) {
		t.Fatalf("подсказки: %q", got)
	}
	if f.ed.prefixes[len(f.ed.prefixes)-1] != "ali" {
		t.Fatalf("префикс: %q", f.ed.prefixes)
	}
	// имеющиеся у произведения имена не предлагаются
	in.entry.SetText("")
	test.Type(in.entry, "char")
	eventually(t, f.q, "без подсказок", func() bool { return len(in.sugg.Objects) == 0 })

	in.entry.SetText("")
	test.Type(in.entry, "ali")
	eventually(t, f.q, "подсказки", func() bool { return len(in.sugg.Objects) == 2 })
	test.Tap(findButton(in.sugg, "alina"))
	pump(f.q)
	if c := f.chip("alina"); c == nil || c.origin != model.OriginCustom {
		t.Fatalf("выбор подсказки не добавил тег: %q", f.shownChips())
	}
}

// Страница берёт теги галереи из источника: сетка могла передать старую копию.
func TestOpenUsesCurrentGallery(t *testing.T) {
	f := setupTags(t)
	stale, _ := f.src.Get(exampleKey)
	stale.Custom = []model.Tag{model.NewTag("tag", "stale")}
	f.d.Open(stale)
	if f.chip("stale") != nil {
		t.Fatal("показана устаревшая копия галереи")
	}
}
