package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPageJSON(t *testing.T) {
	// «作品» и «漫画» в Shift-JIS: не UTF-8, encoding/json слил бы их в одно имя
	a := string([]byte{0x8D, 0xEC, 0x95, 0x69}) + "/001.jpg"
	b := string([]byte{0x96, 0x9F, 0x89, 0xE6}) + "/001.jpg"
	for _, name := range []string{"example/1.jpg", "тест/1.jpg", `ch1\001.jpg`, a, b} {
		data, err := json.Marshal(Page{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		var back Page
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatal(err)
		}
		if back.Name != name {
			t.Errorf("%q → %s → %q", name, data, back.Name)
		}
	}

	// имя в UTF-8 — прежний формат: старые записи каталога читаются как есть
	data, _ := json.Marshal(Page{Name: "example/1.jpg"})
	if string(data) != `{"Name":"example/1.jpg"}` {
		t.Errorf("формат изменился: %s", data)
	}
	var old Page
	if err := json.Unmarshal([]byte(`{"Name":"example/1.jpg"}`), &old); err != nil || old.Name != "example/1.jpg" {
		t.Errorf("старая запись: %q, %v", old.Name, err)
	}

	// в составе галереи (так галерея хранится в каталоге)
	g := Gallery{Key: LocalKey("sjis.zip"), Pages: []Page{{a}, {b}}}
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	var gb Gallery
	if err := json.Unmarshal(data, &gb); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gb.Pages, g.Pages) {
		t.Errorf("страницы галереи %q, ожидались %q", gb.Pages, g.Pages)
	}
}
