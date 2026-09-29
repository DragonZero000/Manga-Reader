package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func overlayGallery() Gallery {
	return Gallery{
		Tags: []Tag{
			NewTag("artist", "artist 1"),
			NewTag("tag", "tag 1"),
			NewTag("tag", "tag 2"),
		},
		Custom: []Tag{NewTag("character", "alice"), NewTag("tag", "my fav")},
		Hidden: []Tag{NewTag("tag", "tag 1"), NewTag("tag", "gone")},
	}
}

func TestEffectiveTagsOrder(t *testing.T) {
	got := overlayGallery().EffectiveTags()
	want := []Tag{
		NewTag("artist", "artist 1"),
		NewTag("tag", "tag 2"),
		NewTag("character", "alice"),
		NewTag("tag", "my fav"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EffectiveTags = %v, want %v", got, want)
	}
}

func TestEffectiveTagsNoOverlay(t *testing.T) {
	g := Gallery{Tags: []Tag{NewTag("tag", "a")}}
	if got := g.EffectiveTags(); !reflect.DeepEqual(got, g.Tags) {
		t.Fatalf("EffectiveTags = %v", got)
	}
}

func TestTagViews(t *testing.T) {
	got := overlayGallery().TagViews()
	want := []TagView{
		{NewTag("artist", "artist 1"), OriginMeta},
		{NewTag("tag", "tag 1"), OriginHidden},
		{NewTag("tag", "tag 2"), OriginMeta},
		{NewTag("character", "alice"), OriginCustom},
		{NewTag("tag", "my fav"), OriginCustom},
	}
	// tag:gone скрыт, но его нет среди оригинальных — не показывается.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TagViews = %v, want %v", got, want)
	}
}

func TestOverlayNotInJSON(t *testing.T) {
	b, err := json.Marshal(overlayGallery())
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, bad := range []string{"alice", "my fav", "gone", "Custom", "Hidden"} {
		if strings.Contains(s, bad) {
			t.Fatalf("JSON содержит наложение %q: %s", bad, s)
		}
	}
}
