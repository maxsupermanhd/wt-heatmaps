package main

import (
	"context"
	"encoding/json"
	"main/frontend"
	"main/lib/imagecolorsort"
	"main/lib/killstorage-duckdb"
	"main/lib/levelcoords"
	"os"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"
)

var levelByColorSorter *imagecolorsort.ImageColorSort[string]

func getSortedLevelStats(ctx context.Context) ([]frontend.LevelStat, error) {
	levelAmounts, err := cachedKSAmountsByLevel.Get(ctx)
	if err != nil {
		return nil, err
	}
	levelNames := make([]string, len(levelAmounts))
	for i, v := range levelAmounts {
		levelNames[i] = v.LevelName
	}
	// a level with no sort value stays in the list
	err = levelByColorSorter.Sort(levelNames)
	if err != nil {
		log.Err(err).Msg("sort levels by tankmap color")
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
	return levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"),
		cfg.GetDString("https://raw.githubusercontent.com/gszabi99/War-Thunder-Datamine/refs/heads/master/aces.vromfs.bin_u/", "urls", "levelCoordsPrefix"),
		level)
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
	levelNamesPath := cfg.GetDString("levelnames.json", "levelNames")
	lnBytes, err := os.ReadFile(levelNamesPath)
	if err != nil {
		log.Err(err).Str("path", levelNamesPath).Msg("reading level names")
	}
	err = json.Unmarshal(lnBytes, &levelNames)
	if err != nil {
		log.Err(err).Str("path", levelNamesPath).Msg("parsing level names")
	}
}
