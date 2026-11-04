package app

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/stats"
	"mangareader/internal/storage"
)

func TestStatsSettingsDefaults(t *testing.T) {
	s := storage.NewMemSettings()
	if StatsFilter(s) != "" || len(StatsExcluded(s)) != 0 || len(StatsDisabled(s)) != 0 || len(StatsModes(s)) != 0 {
		t.Fatal("по умолчанию настройки статистики не пусты")
	}
	pages, size := StatsBounds(s)
	if !reflect.DeepEqual(pages, stats.DefaultPageBounds) || !reflect.DeepEqual(size, stats.DefaultSizeBounds) {
		t.Errorf("границы по умолчанию: %v %v", pages, size)
	}
}

func TestStatsSettingsRoundTrip(t *testing.T) {
	s := storage.NewMemSettings()
	SetStatsFilter(s, "language:japanese")
	ex := map[stats.BlockID]map[string]bool{"artist": {"artist:x": true, "artist:y": true}, stats.ExcludeCustom: {"tag:fav": true}}
	SetStatsExcluded(s, ex)
	SetStatsDisabled(s, map[stats.BlockID]bool{"character": true, stats.BlockQuality: true})
	SetStatsModes(s, map[stats.BlockID]stats.Mode{"artist": stats.ModeSize})
	if err := SetStatsBounds(s, KeyStatsBoundsSize, "1, 2.5, 10"); err != nil {
		t.Fatal(err)
	}
	if err := SetStatsBounds(s, KeyStatsBoundsPages, "10,30"); err != nil {
		t.Fatal(err)
	}

	if StatsFilter(s) != "language:japanese" {
		t.Errorf("фильтр %q", StatsFilter(s))
	}
	if got := StatsExcluded(s); !reflect.DeepEqual(got, ex) {
		t.Errorf("исключения %v", got)
	}
	if got := StatsDisabled(s); !reflect.DeepEqual(got, map[stats.BlockID]bool{"character": true, stats.BlockQuality: true}) {
		t.Errorf("блоки %v", got)
	}
	if got := StatsModes(s); got["artist"] != stats.ModeSize {
		t.Errorf("режимы %v", got)
	}
	pages, size := StatsBounds(s)
	if !reflect.DeepEqual(pages, []float64{10, 30}) || !reflect.DeepEqual(size, []float64{1, 2.5, 10}) {
		t.Errorf("границы %v %v", pages, size)
	}
	opt := StatsOptions(s)
	if !opt.Disabled["character"] || !opt.Excluded["artist"]["artist:y"] || opt.Modes["artist"] != stats.ModeSize {
		t.Errorf("StatsOptions: %+v", opt)
	}
}

func TestStatsOrderLargestSummary(t *testing.T) {
	s := storage.NewMemSettings()
	if len(StatsAscending(s)) != 0 || StatsLargestBy(s) != stats.ModeSize || len(StatsSummaryHidden(s)) != 0 {
		t.Fatal("по умолчанию: всё по убыванию, самые большие по объёму, поля Сводки видны")
	}
	SetStatsAscending(s, map[stats.BlockID]bool{"artist": true, stats.BlockLargest: true, stats.ExcludeHidden: true, "group": false})
	SetStatsLargestBy(s, stats.ModePages)
	SetStatsSummaryHidden(s, map[string]bool{stats.SummaryMedianPages: true, stats.SummaryUnique + "tag": true})
	if got := StatsAscending(s); !reflect.DeepEqual(got, map[stats.BlockID]bool{"artist": true, stats.BlockLargest: true, stats.ExcludeHidden: true}) {
		t.Errorf("порядок %v", got)
	}
	if StatsLargestBy(s) != stats.ModePages {
		t.Error("величина самых больших не сохранилась")
	}
	if got := StatsSummaryHidden(s); len(got) != 2 || !got[stats.SummaryMedianPages] || !got["unique.tag"] {
		t.Errorf("поля Сводки %v", got)
	}
	opt := StatsOptions(s)
	if !opt.Ascending["artist"] || opt.LargestBy != stats.ModePages {
		t.Errorf("StatsOptions: %+v", opt)
	}
	// неизвестные значения отбрасываются
	s.SetString(KeyStatsOrder, `["pages","artist","zzz"]`)
	s.SetString(KeyStatsLargest, "works")
	s.SetString(KeyStatsSummaryHidden, `["dates","zzz"]`)
	if got := StatsAscending(s); len(got) != 1 || !got["artist"] {
		t.Errorf("порядок с мусором %v", got)
	}
	if StatsLargestBy(s) != stats.ModeSize {
		t.Error("неизвестная величина должна давать объём")
	}
	if got := StatsSummaryHidden(s); len(got) != 1 || !got["dates"] {
		t.Errorf("поля с мусором %v", got)
	}
}

func TestStatsSettingsDamaged(t *testing.T) {
	s := storage.NewMemSettings()
	for _, k := range []string{KeyStatsExcluded, KeyStatsBlocks, KeyStatsModes} {
		s.SetString(k, "{не json")
	}
	s.SetString(KeyStatsBoundsPages, "50, 20")
	s.SetString(KeyStatsBoundsSize, "abc")
	if len(StatsExcluded(s)) != 0 || len(StatsDisabled(s)) != 0 || len(StatsModes(s)) != 0 {
		t.Error("битое значение не дало значения по умолчанию")
	}
	pages, size := StatsBounds(s)
	if !reflect.DeepEqual(pages, stats.DefaultPageBounds) || !reflect.DeepEqual(size, stats.DefaultSizeBounds) {
		t.Errorf("битые границы: %v %v", pages, size)
	}
	// неизвестные блоки и режимы отбрасываются
	s.SetString(KeyStatsBlocks, `["character","future"]`)
	s.SetString(KeyStatsModes, `{"artist":"size","pages":"size","group":"zzz"}`)
	if got := StatsDisabled(s); !reflect.DeepEqual(got, map[stats.BlockID]bool{"character": true}) {
		t.Errorf("блоки %v", got)
	}
	if got := StatsModes(s); !reflect.DeepEqual(got, map[stats.BlockID]stats.Mode{"artist": stats.ModeSize}) {
		t.Errorf("режимы %v", got)
	}
}

func TestSetStatsBoundsInvalidKeepsOld(t *testing.T) {
	s := storage.NewMemSettings()
	if err := SetStatsBounds(s, KeyStatsBoundsPages, "10, 30"); err != nil {
		t.Fatal(err)
	}
	var be *stats.BoundsError
	if err := SetStatsBounds(s, KeyStatsBoundsPages, "50, 20"); !errors.As(err, &be) || be.Code != stats.BoundsNotRising {
		t.Fatalf("err = %v", err)
	}
	if err := SetStatsBounds(s, KeyStatsBoundsPages, "2.5"); !errors.As(err, &be) || be.Code != stats.BoundsNotInteger {
		t.Fatalf("дробные страницы: %v", err)
	}
	if pages, _ := StatsBounds(s); !reflect.DeepEqual(pages, []float64{10, 30}) {
		t.Errorf("прежние границы потеряны: %v", pages)
	}
	if err := SetStatsBounds(s, KeyStatsBoundsPages, ""); err != nil {
		t.Fatal(err)
	}
	if pages, _ := StatsBounds(s); !reflect.DeepEqual(pages, stats.DefaultPageBounds) {
		t.Errorf("сброс: %v", pages)
	}
}

type fakeSource struct {
	mu sync.Mutex
	gs []model.Gallery
}

func (f *fakeSource) Galleries() []model.Gallery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gs
}

func statsLib(t *testing.T) (*fakeSource, *search.MemIndex) {
	t.Helper()
	idx := search.NewMemIndex()
	src := &fakeSource{}
	for i := 0; i < 10; i++ {
		g := model.Gallery{Key: model.LocalKey(string(rune('a'+i)) + ".zip"), Title: "w",
			Pages: []model.Page{{Name: "1.jpg"}}, File: model.FileInfo{Size: 10, ModTime: time.Now()}}
		if i < 4 {
			g.AddTag(model.NewTag("language", "japanese"))
		}
		src.gs = append(src.gs, g)
		if err := idx.Upsert(context.Background(), g); err != nil {
			t.Fatal(err)
		}
	}
	return src, idx
}

func request(t *testing.T, svc *StatsService, filter string) StatsResponse {
	t.Helper()
	ch := make(chan StatsResponse, 1)
	svc.Request(filter, stats.Options{}, func(r StatsResponse) { ch <- r })
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("нет ответа")
		return StatsResponse{}
	}
}

func TestStatsServiceFilter(t *testing.T) {
	src, idx := statsLib(t)
	svc := NewStatsService(src, idx)
	if r := request(t, svc, ""); r.Err != nil || r.Library != 10 || r.Result.Works != 10 {
		t.Errorf("без фильтра: %+v", r)
	}
	if r := request(t, svc, "language:japanese"); r.Err != nil || r.Library != 10 || r.Result.Summary.Works != 4 {
		t.Errorf("фильтр: %+v", r)
	}
	if r := request(t, svc, "artist:nobody"); r.Err != nil || r.Result.Works != 0 || r.Library != 10 {
		t.Errorf("пустая выборка: %+v", r)
	}
	if r := request(t, svc, "  "); r.Result.Works != 10 {
		t.Errorf("пробельный фильтр: %+v", r)
	}
}

func TestStatsServiceParseError(t *testing.T) {
	src, idx := statsLib(t)
	r := request(t, NewStatsService(src, idx), "pages:>много")
	var pe *search.ParseError
	if !errors.As(r.Err, &pe) || pe.Token != "pages:>много" {
		t.Fatalf("err = %v", r.Err)
	}
	if r.Result.Summary != nil {
		t.Error("при ошибке разбора статистика посчитана")
	}
}

// Новый запрос отменяет предыдущий: ответ приходит только на последний.
func TestStatsServiceSupersedes(t *testing.T) {
	src, _ := statsLib(t)
	release := make(chan struct{})
	svc := NewStatsService(src, blockingIndex{search.NewMemIndex(), release})
	var mu sync.Mutex
	var got []uint64
	done := make(chan struct{}, 2)
	cb := func(r StatsResponse) {
		mu.Lock()
		got = append(got, r.Seq)
		mu.Unlock()
		done <- struct{}{}
	}
	first := svc.Request("language:japanese", stats.Options{}, cb)
	second := svc.Request("", stats.Options{}, cb)
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("нет ответа")
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(got, []uint64{second}) {
		t.Errorf("ответы %v, ожидался только %d", got, second)
	}
	if svc.Latest(first) || !svc.Latest(second) {
		t.Error("Latest определён неверно")
	}
}

// blockingIndex задерживает поиск до release и учитывает отмену.
type blockingIndex struct {
	*search.MemIndex
	release chan struct{}
}

func (b blockingIndex) Search(ctx context.Context, q search.Query) ([]model.Key, int, error) {
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	return b.MemIndex.Search(ctx, q)
}
