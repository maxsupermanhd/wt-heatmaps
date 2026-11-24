package main

import (
	"context"
	"iter"
	"main/lib/killstorage-duckdb"
	lm "main/lib/lockingmap"
	"maps"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/maxsupermanhd/flexcorallib/fclcache"
	"github.com/rs/zerolog/log"
)

var (
	cachedKSAmountsByLevel = fclcache.NewValueRefresh(wb, 20*time.Minute, func(ctx context.Context) ([]killstorage.AmountsByLevelRow, error) {
		perf := time.Now()
		log.Info().Msg("cache update start by level")
		defer func() { log.Info().Str("perf", time.Since(perf).String()).Msg("cache update done by level") }()
		return ks.GetAmountsByLevel(ctx)
	})
	cachedKSAmountsByDay = fclcache.NewValueRefresh(wb, 20*time.Minute, func(ctx context.Context) (map[time.Time]int, error) {
		perf := time.Now()
		log.Info().Msg("cache update start by day")
		defer func() { log.Info().Str("perf", time.Since(perf).String()).Msg("cache update done by day") }()
		return ks.GetAmountsByDay(ctx)
	})
	cachedKSAmountsByVehicle = fclcache.NewValueRefresh(wb, 20*time.Minute, func(ctx context.Context) (map[string]int, error) {
		perf := time.Now()
		log.Info().Msg("cache update start by vehicle")
		defer func() { log.Info().Str("perf", time.Since(perf).String()).Msg("cache update done by vehicle") }()
		return ks.GetAmountsByVehicle(ctx)
	})
	cachedKSAmountsByInteresting = fclcache.NewValueRefresh(wb, 20*time.Minute, func(ctx context.Context) ([]killstorage.InterestKillsDeathsRow, error) {
		perf := time.Now()
		log.Info().Msg("cache update start interesting")
		defer func() { log.Info().Str("perf", time.Since(perf).String()).Msg("cache update done interesting") }()
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

	cachedHeat = lm.LockingMap[*cachedHeatAggregate]{}
)

type cachedHeatAggregatePoint struct {
	z      int32
	kills  uint32
	deaths uint32
}

type cachedHeatAggregateRow struct {
	x   int32
	row []cachedHeatAggregatePoint
}

type cachedHeatAggregate struct {
	tally     []cachedHeatAggregateRow
	sumKills  uint64
	sumDeaths uint64
}

func (a *cachedHeatAggregate) Loop() iter.Seq[*killstorage.KillTally] {
	val := &killstorage.KillTally{}
	return func(yield func(*killstorage.KillTally) bool) {
		for _, row := range a.tally {
			val.X = row.x
			for _, pix := range row.row {
				val.Z = pix.z
				val.Kills = pix.kills
				val.Deaths = pix.deaths
				if !yield(val) {
					return
				}
			}
		}
	}
}

func cachedHeatGetDataPointsPerLevel() map[string]int {
	ret := map[string]int{}
	cachedHeat.AccessExclusive(func(m map[string]*cachedHeatAggregate) (map[string]*cachedHeatAggregate, error) {
		for k, v := range m {
			ret[k] = int(v.sumKills)
		}
		return m, nil
	})
	return ret
}

func cachedHeatInitAggregate(ctx context.Context, k string) (*cachedHeatAggregate, error) {
	a := &cachedHeatAggregate{}
	kq := killstorage.QueryConditions{}
	kq.QueryWithDirectWeapon()
	kq.QueryWithLevel(k)
	t, err := ks.GetKillCountsByCoord(ctx, kq)
	if err != nil {
		log.Err(err).Msg("GetKillCountsByCoord while caching")
		return nil, err
	}
	if len(t) == 0 {
		return a, nil
	}
	slices.SortFunc(t, func(a, b killstorage.KillTally) int {
		dx := a.X - b.X
		if dx != 0 {
			return int(dx)
		}
		return int(a.Z - b.Z)
	})
	prevx := t[0].X
	a.tally = append(a.tally, cachedHeatAggregateRow{
		x: prevx,
		row: []cachedHeatAggregatePoint{cachedHeatAggregatePoint{
			z:      t[0].Z,
			kills:  t[0].Kills,
			deaths: t[0].Deaths,
		}},
	})
	a.sumKills = uint64(t[0].Kills)
	a.sumDeaths = uint64(t[0].Deaths)
	for _, v := range t[1:] {
		a.sumDeaths += uint64(v.Deaths)
		a.sumKills += uint64(v.Kills)
		if prevx == v.X {
			a.tally[len(a.tally)-1].row = append(a.tally[len(a.tally)-1].row, cachedHeatAggregatePoint{
				z:      v.Z,
				kills:  v.Kills,
				deaths: v.Deaths,
			})
		} else {
			prevx = v.X
			a.tally = append(a.tally, cachedHeatAggregateRow{
				x: v.X,
				row: []cachedHeatAggregatePoint{cachedHeatAggregatePoint{
					z:      v.Z,
					kills:  v.Kills,
					deaths: v.Deaths,
				}},
			})
		}
	}
	return a, nil
}

func cachedHeatPopulate(ctx context.Context, k string, kills []killstorage.Kill) error {
	return cachedHeat.Access(k, func(a *cachedHeatAggregate) (*cachedHeatAggregate, error) {
		if a == nil {
			return cachedHeatInitAggregate(ctx, k)
		}
		addTally := func(tx, tz int32, tk, td uint32) {
			ri := slices.IndexFunc(a.tally, func(row cachedHeatAggregateRow) bool { return row.x == tx })
			if ri < 0 {
				a.tally = append(a.tally, cachedHeatAggregateRow{
					x:   tx,
					row: []cachedHeatAggregatePoint{{z: tz, kills: tk, deaths: td}},
				})
				return
			}
			row := &a.tally[ri]
			pi := slices.IndexFunc(row.row, func(pix cachedHeatAggregatePoint) bool { return pix.z == tz })
			if pi < 0 {
				row.row = append(row.row, cachedHeatAggregatePoint{z: tz, kills: tk, deaths: td})
				return
			}
			row.row[pi].kills += tk
			row.row[pi].deaths += td
		}
		for _, v := range kills {
			// the same filter as QueryWithDirectWeapon in cachedHeatInitAggregate
			if v.Weapon == "" || v.Weapon == "artillery" {
				continue
			}
			a.sumKills++
			a.sumDeaths++
			addTally(int32(math.Round(v.KillerPosX)), int32(math.Round(v.KillerPosZ)), 1, 0)
			addTally(int32(math.Round(v.VictimPosX)), int32(math.Round(v.VictimPosZ)), 0, 1)
		}
		return a, nil
	})
}
