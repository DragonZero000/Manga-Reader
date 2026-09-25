package model

import "testing"

func TestWebURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://a.example/g/1/": "https://a.example/g/1/",
		" HTTP://a.example ":     "HTTP://a.example",
		"a.example/g/1":          "",
		"ftp://a.example/":       "",
		"file:///C:/x.zip":       "",
		"https://":               "",
		"":                       "",
	} {
		if got := WebURL(in); got != want {
			t.Errorf("WebURL(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}
