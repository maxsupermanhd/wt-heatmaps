package main

import (
	"context"
	"iter"
	"main/lib/killstorage-duckdb"
	lm "main/lib/lockingmap"
	"math"
	"slices"

	"github.com/rs/zerolog/log"
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

var (
	cachedHeat = lm.LockingMap[*cachedHeatAggregate]{}
)

func cachedHeatInitAggregate(k string) (*cachedHeatAggregate, error) {
	a := &cachedHeatAggregate{}
	kq := &killstorage.QueryConditions{}
	ks.QueryWithLevel(kq, k)
	t, err := ks.GetKillCountsByCoord(context.Background(), kq)
	if err != nil {
		log.Err(err).Msg("GetKillCountsByCoord while caching")
		return a, err
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

func cachedHeatPopulate(k string, kills []killstorage.Kill) error {
	return cachedHeat.Access(k, func(a *cachedHeatAggregate) (*cachedHeatAggregate, error) {
		if a == nil {
			var err error
			a, err = cachedHeatInitAggregate(k)
			if err != nil {
				return a, err
			}
		}
		a.sumDeaths += uint64(len(kills))
		a.sumKills += uint64(len(kills))
		for _, v := range kills {
			kx := int32(math.Round(v.KillerPosX))
			kz := int32(math.Round(v.KillerPosZ))
			vx := int32(math.Round(v.VictimPosX))
			vz := int32(math.Round(v.VictimPosZ))

			// todo: binary search
			addTally := func(rows []cachedHeatAggregateRow, tx, tz int32, tk, td uint32) []cachedHeatAggregateRow {
				rowFound := false
				for _, row := range rows {
					if row.x != tx {
						continue
					}
					rowFound = true
					pixFound := false
					for _, pix := range row.row {
						if pix.z != tz {
							continue
						}
						pixFound = true
						pix.deaths += td
						pix.kills += tk
						break
					}
					if !pixFound {
						row.row = append(row.row, cachedHeatAggregatePoint{
							z:      tz,
							kills:  tk,
							deaths: td,
						})
					}
					break
				}
				if !rowFound {
					rows = append(rows, cachedHeatAggregateRow{
						x: tx,
						row: []cachedHeatAggregatePoint{cachedHeatAggregatePoint{
							z:      tz,
							kills:  tk,
							deaths: td,
						}},
					})
				}
				return rows
			}

			a.tally = addTally(a.tally, kx, kz, 1, 0)
			a.tally = addTally(a.tally, vx, vz, 0, 1)
		}
		return a, nil
	})
}
