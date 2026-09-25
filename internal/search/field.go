package search

// Field — параметр произведения, по которому можно искать или сортировать.
type Field int

const (
	FieldTitle     Field = iota + 1 // основное и альтернативное название
	FieldTag                        // тег с типом
	FieldID                         // внешний ID
	FieldPages                      // количество страниц
	FieldUploaded                   // дата загрузки на сайт
	FieldFavorites                  // избранное на сайте
	FieldScanlator                  // сканлейтор
	FieldAdded                      // время изменения файла
	FieldSize                       // размер файла
)

// Op — операция фильтра.
type Op int

const (
	OpContains Op = iota + 1 // текст содержит подстроку
	OpHas                    // есть тег
	OpNotHas                 // нет тега
	OpEq                     // равно
	OpLt                     // меньше
	OpGt                     // больше
	OpBetween                // между (включительно)
)

// ValueKind — тип значения поля.
type ValueKind int

const (
	KindText ValueKind = iota + 1
	KindTag
	KindNumber
	KindDate
)

// FieldInfo — описание поля. Из этой таблицы строятся UI фильтров и парсер запросов.
type FieldInfo struct {
	Field    Field
	Name     string // имя в строке запроса: pages, artist:... и т.д.
	Label    string // подпись в UI
	Kind     ValueKind
	Ops      []Op
	Sortable bool
}

var numRange = []Op{OpLt, OpGt, OpBetween}

var fields = []FieldInfo{
	{FieldTitle, "title", "Название", KindText, []Op{OpContains}, true},
	{FieldTag, "tag", "Тег", KindTag, []Op{OpHas, OpNotHas}, false},
	{FieldID, "id", "ID", KindNumber, []Op{OpEq}, true},
	{FieldPages, "pages", "Страниц", KindNumber, []Op{OpEq, OpLt, OpGt, OpBetween}, true},
	{FieldUploaded, "uploaded", "Загружено на сайт", KindDate, numRange, true},
	{FieldFavorites, "favorites", "Избранное на сайте", KindNumber, numRange, true},
	{FieldScanlator, "scanlator", "Сканлейтор", KindText, []Op{OpEq, OpContains}, false},
	{FieldAdded, "added", "Добавлено", KindDate, numRange, true},
	{FieldSize, "size", "Размер файла", KindNumber, numRange, true},
}

// Fields возвращает описание всех полей поиска.
func Fields() []FieldInfo {
	out := make([]FieldInfo, len(fields))
	copy(out, fields)
	return out
}

// Info возвращает описание поля.
func (f Field) Info() (FieldInfo, bool) {
	for _, fi := range fields {
		if fi.Field == f {
			return fi, true
		}
	}
	return FieldInfo{}, false
}

// FieldByName ищет поле по имени из строки запроса.
func FieldByName(name string) (Field, bool) {
	for _, fi := range fields {
		if fi.Name == name {
			return fi.Field, true
		}
	}
	return 0, false
}

func (f Field) String() string {
	if fi, ok := f.Info(); ok {
		return fi.Name
	}
	return "unknown"
}

// Allows сообщает, допустима ли операция для поля.
func (fi FieldInfo) Allows(op Op) bool {
	for _, o := range fi.Ops {
		if o == op {
			return true
		}
	}
	return false
}

var opNames = map[Op]string{
	OpContains: "содержит",
	OpHas:      "есть",
	OpNotHas:   "нет",
	OpEq:       "равно",
	OpLt:       "меньше",
	OpGt:       "больше",
	OpBetween:  "между",
}

func (o Op) String() string {
	if s, ok := opNames[o]; ok {
		return s
	}
	return "unknown"
}
