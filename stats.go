package main

import (
	"context"
	"fmt"
	"main/frontend"
	killstorage "main/lib/killstorage-duckdb"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/maxsupermanhd/flexcorallib/fclcache"
	"github.com/rs/zerolog/log"
)

var (
	cachedKSAmountsByLevel = fclcache.NewValueRefresh(wb, 10*time.Minute, func(ctx context.Context) ([]killstorage.AmountsByLevelRow, error) {
		return ks.GetAmountsByLevel(ctx)
	})
	cachedKSAmountsByDay = fclcache.NewValueRefresh(wb, 10*time.Minute, func(ctx context.Context) (map[time.Time]int, error) {
		return ks.GetAmountsByDay(ctx)
	})
	cachedKSAmountsByVehicle = fclcache.NewValueRefresh(wb, 10*time.Minute, func(ctx context.Context) (map[string]int, error) {
		return ks.GetAmountsByVehicle(ctx)
	})
	cachedKSAmountsByInteresting = fclcache.NewValueRefresh(wb, 10*time.Minute, func(ctx context.Context) ([]killstorage.InterestKillsDeathsRow, error) {
		idsStrings, ok := cfg.GetKeys("interesting")
		if !ok {
			return nil, nil
		}
		req := map[uint64]string{}
		for _, idString := range idsStrings {
			id, err := strconv.ParseUint(idString, 10, 64)
			if err != nil {
				continue
			}
			name, ok := cfg.GetString("interesting", idString)
			if ok {
				req[id] = name
			}
		}
		return ks.GetAmountsOfInteresting(ctx, slices.Collect(maps.Keys(req)))
	})
)

func getStatsTables(ctx context.Context) ([]frontend.StatsTable, error) {
	ret := []frontend.StatsTable{}
	tables := []func(ctx context.Context) ([]frontend.StatsTable, error){
		statsGetByLevel,
		statsGetByDay,
		statsGetByVehicles,
		statsGetInteresting,
	}
	for _, fn := range tables {
		st, err := fn(ctx)
		if err != nil {
			continue
		}
		ret = append(ret, st...)
	}
	return ret, nil
}

func statsGetByLevel(ctx context.Context) ([]frontend.StatsTable, error) {
	byLevel, err := cachedKSAmountsByLevel.Get(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by level")
		return nil, err
	}
	byLevelMax := 0
	for _, v := range byLevel {
		byLevelMax = max(byLevelMax, v.Count)
	}
	byLevelRows := [][]templ.Component{}
	ingestCurrentPreferencesLock.Lock()
	reqMaps := ingestCurrentPreferences.reqMaps
	ingestCurrentPreferencesLock.Unlock()
	for _, k := range byLevel {
		levelName := levelToLocalized(k.LevelName)
		isRequested := ""
		if slices.Contains(reqMaps, levelName) {
			isRequested = "↓"
		}
		byLevelRows = append(byLevelRows, []templ.Component{
			frontend.TextNode(isRequested),
			frontend.TextNode(levelName),
			frontend.TextNode(strconv.Itoa(k.Count)),
			frontend.StatElementFixedPercentBar(float64(k.Count) / float64(byLevelMax)),
		})
	}
	return []frontend.StatsTable{{
		Caption:      "Records by level<br/>at " + time.Now().Format(time.DateTime),
		ColumnLabels: []string{"", "Level", "Count"},
		Rows:         byLevelRows,
	}}, nil
}

func statsGetByDay(ctx context.Context) ([]frontend.StatsTable, error) {
	byDay, err := cachedKSAmountsByDay.Get(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by day")
		return nil, err
	}
	byDayMax := 0
	total := 0
	for _, v := range byDay {
		total += v
		byDayMax = max(byDayMax, v)
	}
	byDayRows := [][]templ.Component{
		[]templ.Component{
			frontend.TextNode("Total"),
			frontend.TextNode(strconv.Itoa(total)),
			frontend.TextNode(""),
		},
	}
	for _, k := range slices.SortedFunc(maps.Keys(byDay), func(a, b time.Time) int {
		return b.Compare(a)
	}) {
		byDayRows = append(byDayRows, []templ.Component{
			frontend.TextNode(k.Format(time.DateOnly)),
			frontend.TextNode(strconv.Itoa(byDay[k])),
			frontend.StatElementFixedPercentBar(float64(byDay[k]) / float64(byDayMax)),
		})
	}
	return []frontend.StatsTable{{
		Caption:      "Records by date<br/>at " + time.Now().Format(time.DateTime),
		ColumnLabels: []string{"Time (UTC)", "Count", ""},
		Rows:         byDayRows,
	}}, nil
}

func statsGetByVehicles(ctx context.Context) ([]frontend.StatsTable, error) {
	byVehicle, err := cachedKSAmountsByVehicle.Get(ctx)
	if err != nil {
		log.Err(err).Msg("cache update amounts by br")
		return nil, err
	}
	vehicles := map[string]int{}
	for br := range vehicleEconomyCatalog.GetRankMax() {
		for _, v := range vehicleEconomyCatalog.GetAllByRank(br) {
			vehicles[strings.TrimPrefix(v, "tankmodels/")] = br
		}
	}
	byBR := map[int]int{}
	for v, c := range byVehicle {
		br, ok := vehicles[strings.TrimPrefix(v, "tankmodels/")]
		if !ok {
			continue
		}
		byBR[br] = byBR[br] + c
	}
	tableByBR := frontend.StatsTable{
		Caption:      "Records by BR<br/>at " + time.Now().Format(time.DateTime),
		ColumnLabels: []string{"", "BR", "Count", ""},
		Rows:         [][]templ.Component{},
	}
	byBRMax := 0
	for _, v := range byBR {
		byBRMax = max(byBRMax, v)
	}
	ingestCurrentPreferencesLock.Lock()
	reqBrs := ingestCurrentPreferences.reqBrs
	ingestCurrentPreferencesLock.Unlock()
	for k := range slices.Sorted(maps.Keys(byBR)) {
		isRequested := ""
		if slices.Contains(reqBrs, frontend.BRString(k)) {
			isRequested = "↓"
		}
		tableByBR.Rows = append(tableByBR.Rows, []templ.Component{
			frontend.TextNode(isRequested),
			frontend.TextNode(frontend.BRString(k)),
			frontend.TextNode(strconv.Itoa(byBR[k])),
			frontend.StatElementFixedPercentBar(float64(byBR[k]) / float64(byBRMax)),
		})
	}
	tableByVehicles := frontend.StatsTable{
		Caption:      "Records by vehicles<br/>at " + time.Now().Format(time.DateTime),
		ColumnLabels: []string{"Vehicle", "BR", "Count"},
		Rows:         [][]templ.Component{},
	}
	type vehiclePair struct {
		name  string
		count int
	}
	vehiclesSlice := []vehiclePair{}
	for v, c := range byVehicle {
		vehiclesSlice = append(vehiclesSlice, vehiclePair{
			name:  v,
			count: c,
		})
	}
	slices.SortFunc(vehiclesSlice, func(a, b vehiclePair) int {
		return b.count - a.count
	})
	if len(vehiclesSlice) > 100 {
		vehiclesSlice = vehiclesSlice[:100]
	}
	for _, vp := range vehiclesSlice {
		name := strings.TrimPrefix(vp.name, "tankmodels/")
		tableByVehicles.Rows = append(tableByVehicles.Rows, []templ.Component{
			frontend.VehicleLabel(frontendVehicle(name, vehicleEconomyCatalog.Vehicles[name])),
			frontend.TextNode(frontend.BRString(vehicles[name])),
			frontend.TextNode(strconv.Itoa(vp.count)),
		})
	}
	return []frontend.StatsTable{tableByBR, tableByVehicles}, nil
}

func statsGetInteresting(ctx context.Context) ([]frontend.StatsTable, error) {
	interesting, err := cachedKSAmountsByInteresting.Get(ctx)
	if err != nil {
		log.Err(err).Msg("cache update interesting")
		return nil, err
	}
	ret := [][]templ.Component{}
	for _, row := range interesting {
		row.Deaths = max(row.Deaths, 1)
		ret = append(ret, []templ.Component{
			frontend.TextNode(cfg.GetDString("???", "interesting", strconv.FormatUint(row.ID, 10))),
			frontend.TextNode(strconv.FormatUint(row.ID, 10)),
			frontend.TextNode(strconv.Itoa(row.Kills)),
			frontend.TextNode(strconv.Itoa(row.Deaths)),
			frontend.TextNode(fmt.Sprintf("%.2f", float64(row.Kills)/float64(row.Deaths))),
		})
	}
	return []frontend.StatsTable{{
		Caption:      "Interesting<br/>at " + time.Now().Format(time.DateTime),
		ColumnLabels: []string{"Name", "ID", "K", "D", "K/D"},
		Rows:         ret,
	}}, nil
}

func serveStats(_ http.ResponseWriter, r *http.Request) templ.Component {
	tables, err := getStatsTables(r.Context())
	if err != nil {
		log.Err(err).Msg("getting stats tables")
	}
	return frontend.Page(frontend.Stats(tables, int(ingestStatSessionRate5m.Load()), int(ingestStatKillsRate5m.Load())))
}
