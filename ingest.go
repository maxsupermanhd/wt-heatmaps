package main

import (
	"context"
	"encoding/json"
	"fmt"
	"main/frontend"
	"main/lib/killstorage"
	"main/lib/lux"
	"main/lib/lux/luxproto/luxprotogen"
	"main/lib/ratetrack"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	ingestStatSessionRate5m = &atomic.Int64{}
	ingestStatKillsRate5m   = &atomic.Int64{}
)

func ingestRoutine(exitChan <-chan struct{}) {
	luxToken, haveLuxToken := cfg.GetString("lux", "token")
	if !haveLuxToken {
		log.Info().Msg("no lux.token so not ingesting")
		return
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	carvesChan := make(chan *luxprotogen.Replay, 16)
	preferencesChan := make(chan map[string]any, 16)
	reconnectExitChan := make(chan struct{})
	updatePreferencesExitChan := make(chan struct{})

	wg.Go(func() {
		prefs, err := getPreferences()
		if err == nil {
			preferencesChan <- prefs
		}
		log.Err(err).Str("prefs", fmt.Sprintf("%#+v", prefs)).Msg("got initial preferences")
		for {
			select {
			case <-updatePreferencesExitChan:
				return
			case <-time.After(20 * time.Minute):
				prefs, err := getPreferences()
				log.Err(err).Str("prefs", fmt.Sprintf("%#+v", prefs)).Msg("got preferences update")
				if err == nil {
					select {
					case preferencesChan <- prefs:
						log.Info().Msg("ingest preferences updated")
					default:
						log.Warn().Msg("ingest preferences not updated")
					}
				}
			}
		}
	})
	wg.Go(func() {
		rate := ratetrack.NewRateTracker(5*time.Minute, ingestStatSessionRate5m, ingestStatKillsRate5m)
		for carve := range carvesChan {
			kills, err := killstorage.LuxCarveToKills(carve)
			if err != nil {
				log.Err(err).Msg("carve to kills fail")
				b, _ := json.Marshal(carve)
				os.WriteFile("dump.json", b, 0644)
				continue
			}
			err = ks.StoreKills(kills)
			if err != nil {
				log.Err(err).Int("n", len(kills)).Msg("storing kills")
			}
			rate.Measure(len(kills))
		}
		log.Info().Msg("carve loop exited")
	})
	wg.Go(func() {
		defer log.Info().Msg("lux loop exited")
		defer close(carvesChan)
		for {
			select {
			case <-reconnectExitChan:
				return
			case <-time.After(5 * time.Second):
			}
			err := lux.FetchFromLux(log.Logger, ctx.Done(), carvesChan, preferencesChan, luxToken)
			log.Err(err).Msg("lux fetch exited")
		}
	})

	<-exitChan

	cancel()
	close(reconnectExitChan)
	close(updatePreferencesExitChan)

	wg.Wait()
}

func getPreferences() (ret map[string]any, err error) {
	byLevel, err := ks.GetAmountsByLevel(context.Background())
	if err != nil {
		return
	}
	reqMaps := []string{}
	i := 0
	for _, v := range slices.Backward(byLevel) {
		if i >= cfg.GetDInt(20, "lux", "fetchMapsCount") {
			break
		}
		if v.LevelName == "levels/avg_nuclear_incident.bin" {
			continue
		}
		reqMaps = append(reqMaps, levelToLocalized(v.LevelName))
		i++
	}
	reqMaps = append(reqMaps, "Falkland Islands")
	byVehicle, err := ks.GetAmountsByVehicle(context.Background())
	if err != nil {
		return
	}
	vehicles := map[string]int{}
	for br := range vehicleEconomyCatalog.GetRankMax() {
		for _, v := range vehicleEconomyCatalog.GetAllByRank(br) {
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
	reqBRsInternal := slices.SortedFunc(maps.Keys(byBR), func(a, b int) int {
		return byBR[a] - byBR[b]
	})
	mainCond := []map[string]any{
		{"maps": reqMaps},
	}
	if len(byBR) != 0 {
		reqBRsInternal = reqBRsInternal[:len(reqBRsInternal)-len(reqBRsInternal)/3-1]
		reqBRs := make([]string, len(reqBRsInternal))
		for i := range reqBRsInternal {
			reqBRs[i] = frontend.BRString(reqBRsInternal[i])
		}
		mainCond = append(mainCond, map[string]any{"brs": reqBRs})
	}
	ret = map[string]any{
		"op": "and",
		"items": []map[string]any{
			{
				"modes": []string{"tank_event_in_random_battles_historical"},
			}, {
				"op":    "or",
				"items": mainCond,
			},
		},
	}
	return
}
