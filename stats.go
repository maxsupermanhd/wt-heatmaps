package main

import (
	"context"
	"main/frontend"
	"main/lib/caches"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"
)

var cachedStatsTables = caches.NewValueRefresh(wb, 10*time.Minute, func() ([]frontend.StatsTable, error) {
	return collectStatsTables(context.Background())
})

func collectStatsTables(ctx context.Context) ([]frontend.StatsTable, error) {
	ret := []frontend.StatsTable{}
	tables := []func(ctx context.Context) (*frontend.StatsTable, error){
		statsGetByLevel,
		statsGetByDay,
		statsGetByBR,
	}
	for _, fn := range tables {
		st, err := fn(ctx)
		if err != nil {
			continue
		}
		ret = append(ret, *st)
	}
	return ret, nil
}

func statsGetByLevel(ctx context.Context) (*frontend.StatsTable, error) {
	byLevel, err := ks.GetAmountsByLevel(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by level")
		return nil, err
	}
	byLevelMax := 0
	for _, v := range byLevel {
		byLevelMax = max(byLevelMax, v.Count)
	}
	byLevelRows := [][]templ.Component{}
	for _, k := range byLevel {
		byLevelRows = append(byLevelRows, []templ.Component{
			frontend.TextNode(levelToLocalized(k.LevelName)),
			frontend.TextNode(strconv.Itoa(k.Count)),
			frontend.StatElementFixedPercentBar(float64(k.Count) / float64(byLevelMax)),
		})
	}
	return &frontend.StatsTable{
		Caption:      "Records by level",
		ColumnLabels: []string{"Level", "Count"},
		Rows:         byLevelRows,
	}, nil
}

func statsGetByDay(ctx context.Context) (*frontend.StatsTable, error) {
	byDay, err := ks.GetAmountsByDay(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by day")
		return nil, err
	}
	byDayMax := 0
	for _, v := range byDay {
		byDayMax = max(byDayMax, v)
	}
	byDayRows := [][]templ.Component{}
	for _, k := range slices.SortedFunc(maps.Keys(byDay), func(a, b time.Time) int {
		return b.Compare(a)
	}) {
		byDayRows = append(byDayRows, []templ.Component{
			frontend.TextNode(k.Format(time.DateOnly)),
			frontend.TextNode(strconv.Itoa(byDay[k])),
			frontend.StatElementFixedPercentBar(float64(byDay[k]) / float64(byDayMax)),
		})
	}
	return &frontend.StatsTable{
		Caption:      "Records by date",
		ColumnLabels: []string{"Time (UTC)", "Count", ""},
		Rows:         byDayRows,
	}, nil
}

func statsGetByBR(ctx context.Context) (*frontend.StatsTable, error) {
	byVehicle, err := ks.GetAmountsByVehicle(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by br")
		return nil, err
	}
	vehicles := map[string]int{}
	for br := range battleRatingGetter.GetRankMax() {
		for _, v := range battleRatingGetter.GetAllByRank(br) {
			vehicles[strings.TrimPrefix(v, "tankmodels/")] = br
		}
	}
	byBR := map[int]int{}
	for v, c := range byVehicle {
		br, ok := vehicles[v]
		if !ok {
			continue
		}
		byBR[br] = byBR[br] + c
	}
	ret := &frontend.StatsTable{
		Caption:      "Records by BR",
		ColumnLabels: []string{"BR", "Count"},
		Rows:         [][]templ.Component{},
	}
	byBRMax := 0
	for _, v := range byBR {
		byBRMax = max(byBRMax, v)
	}
	for k := range slices.Sorted(maps.Keys(byBR)) {
		ret.Rows = append(ret.Rows, []templ.Component{
			frontend.TextNode(frontend.BRString(k)),
			frontend.StatElementFixedPercentBar(float64(byBR[k]) / float64(byBRMax)),
		})
	}
	return ret, nil
}

func serveStats(_ http.ResponseWriter, _ *http.Request) templ.Component {
	tables, _ := cachedStatsTables.Get()
	updatedAt := cachedStatsTables.LastRefresh()
	return frontend.Page(frontend.Stats(tables, updatedAt, int(ingestStatSessionRate5m.Load()), int(ingestStatKillsRate5m.Load())))
}
