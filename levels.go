package main

import (
	"context"
	"encoding/json"
	"main/frontend"
	"main/lib/caches"
	"main/lib/imagecolorsort"
	"main/lib/killstorage"
	"main/lib/levelcoords"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var (
	levelByColorSorter = imagecolorsort.NewImageColorSort(tankmapFromCache)
	levelAmountsCache  = caches.NewValueRefresh(wb, 30*time.Minute, func() ([]killstorage.AmountsByLevelRow, error) {
		return ks.GetAmountsByLevel(context.Background())
	})
	levelStatsSorted = caches.NewValueRefresh(wb, 30*time.Minute, getSortedLevelStats)
)

func getSortedLevelStats() ([]frontend.LevelStat, error) {
	levelAmounts, err := levelAmountsCache.Get()
	if err != nil {
		return nil, err
	}
	levelNames := make([]string, len(levelAmounts))
	for i, v := range levelAmounts {
		levelNames[i] = v.LevelName
	}
	err = levelByColorSorter.Sort(levelNames)
	if err != nil {
		return nil, err
	}
	levels := make([]frontend.LevelStat, 0, len(levelAmounts))
	for _, levelName := range levelNames {
		levels = append(levels, frontend.LevelStat{
			Level:        levelName,
			LevelDisplay: levelToLocalized(levelName),
			Samples: levelAmounts[slices.IndexFunc(levelAmounts, func(val killstorage.AmountsByLevelRow) bool {
				return val.LevelName == levelName
			})].Count,
		})
	}
	return levels, nil
}

func getLevelOffsets(level string) (levelcoords.LevelCoords, error) {
	return levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
}

func levelToLocalized(n string) string {
	k := strings.TrimPrefix(n, "levels/")
	k = strings.TrimSuffix(k, ".bin")
	k, isWinter := strings.CutSuffix(k, "_snow")
	name, ok := levelNames["location/"+k]
	if !ok {
		log.Warn().Str("level", n).Str("k", k).Msg("unmatched locale name")
		return n
	}
	name = strings.TrimSuffix(name, " - tank battle")
	if isWinter {
		name += " (winter)"
	}
	return name
}

var (
	levelNames = map[string]string{}
)

func initLevelNames() {
	lnBytes, err := os.ReadFile("levelnames.json")
	if err != nil {
		log.Err(err).Msg("reading levelnames.json")
	}
	err = json.Unmarshal(lnBytes, &levelNames)
	if err != nil {
		log.Err(err).Msg("parsing levelnames.json")
	}
}
