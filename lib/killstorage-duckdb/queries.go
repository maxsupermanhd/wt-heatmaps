package killstorage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

type QueryCondition struct {
	Cond      string
	Args      []any
	Arbitrary any
}

type QueryConditions map[string]*QueryCondition

func (q QueryConditions) Clone() QueryConditions {
	return maps.Clone(q)
}

func (q QueryConditions) Has(k string) bool {
	_, ok := q[k]
	return ok
}

func (q QueryConditions) HasAll(k ...string) bool {
	for _, k2 := range k {
		_, have := q[k2]
		if !have {
			return false
		}
	}
	return true
}

func (q QueryConditions) HasAny(k ...string) bool {
	for _, k2 := range k {
		_, have := q[k2]
		if have {
			return true
		}
	}
	return false
}

func (q QueryConditions) QueryWithLevel(level string) {
	q["level"] = &QueryCondition{
		Cond: "level = ?",
		Args: []any{level},
	}
}

func (q QueryConditions) QueryWithMission(mission string) {
	q["mission"] = &QueryCondition{
		Cond: "mission = ?",
		Args: []any{mission},
	}
}

func (q QueryConditions) QueryWithKillerVehicles(vehicles []string) {
	q["killer_vehicle"] = &QueryCondition{
		Cond: "killer_vehicle in ?",
		Args: []any{vehicles},
	}
}

func (q QueryConditions) QueryWithVictimVehicles(vehicles []string) {
	q["victim_vehicle"] = &QueryCondition{
		Cond: "victim_vehicle in ?",
		Args: []any{vehicles},
	}
}

func (q QueryConditions) QueryWithSessionTimeMin(tsFrom time.Time) {
	q["session_time"] = &QueryCondition{
		Cond: "session_time >= ?",
		Args: []any{tsFrom},
	}
}

func (q QueryConditions) QueryWithSessionTimeMax(tsTo time.Time) {
	q["session_time"] = &QueryCondition{
		Cond: "session_time < ?",
		Args: []any{tsTo},
	}
}

func (q QueryConditions) QueryWithTeam(team int) {
	q["team"] = &QueryCondition{
		Cond:      "",
		Args:      []any{},
		Arbitrary: team,
	}
}

func (q QueryConditions) QueryWithKillerTeam(team int) {
	q["killer_team"] = &QueryCondition{
		Cond: "killer_team = ?",
		Args: []any{team},
	}
}

func (q QueryConditions) QueryWithVictimTeam(team int) {
	q["victim_team"] = &QueryCondition{
		Cond: "victim_team = ?",
		Args: []any{team},
	}
}

func (q QueryConditions) QueryWithKillTimeMin(killTimeMin time.Duration) {
	q["kill_time"] = &QueryCondition{
		Cond: "kill_time >= ?",
		Args: []any{int(killTimeMin.Milliseconds())},
	}
}

func (q QueryConditions) QueryWithKillTimeMax(killTimeMax time.Duration) {
	q["kill_time"] = &QueryCondition{
		Cond: "kill_time <= ?",
		Args: []any{int(killTimeMax.Milliseconds())},
	}
}

func (q QueryConditions) QueryWithAreaRectangle(x0, z0, x1, z1 float64) {
	q["areaRectangle"] = &QueryCondition{
		Cond: "",
		Args: []any{min(x0, x1), max(x0, x1), min(z0, z1), max(z0, z1)},
	}
}

func (q QueryConditions) QueryWithAreaCircle(x, z, r float64) {
	q["areaCircle"] = &QueryCondition{
		Cond: "",
		Args: []any{x, z, r},
	}
}

func (q QueryConditions) QueryWithArbitrary(name, cond string, args ...any) {
	q[name] = &QueryCondition{
		Cond: cond,
		Args: args,
	}
}

func (q QueryConditions) WhereCase(only ...string) string {
	if len(q) == 0 {
		return ""
	}
	t := []string{}
	if len(only) == 0 {
		for _, k := range slices.Sorted(maps.Keys(q)) {
			c := q[k]
			if c.Cond == "" {
				continue
			}
			t = append(t, c.Cond)
		}
	} else {
		for _, k := range only {
			c, ok := q[k]
			if !ok {
				continue
			}
			if c.Cond == "" {
				continue
			}
			t = append(t, c.Cond)
		}
	}
	return "WHERE " + strings.Join(t, " AND ")
}

func (q QueryConditions) Args(only ...string) []any {
	if len(q) == 0 {
		return []any{}
	}
	ret := []any{}
	if len(only) == 0 {
		for _, k := range slices.Sorted(maps.Keys(q)) {
			c := q[k]
			if c.Cond == "" {
				continue
			}
			ret = append(ret, c.Args...)
		}
	} else {
		for _, k := range only {
			c, ok := q[k]
			if !ok {
				continue
			}
			if c.Cond == "" {
				continue
			}
			ret = append(ret, c.Args...)
		}
	}
	return ret
}

func (q QueryConditions) Dump() string {
	return q.WhereCase() + " " + spew.Sdump(q)
}

func (q QueryConditions) SeparatedConds() (qKillerCond, qVictimCond string) {
	if condTeam := q["team"]; condTeam != nil {
		qKillerCond += ` and killer_team = ` + strconv.Itoa(condTeam.Arbitrary.(int)) + ` `
		qVictimCond += ` and victim_team = ` + strconv.Itoa(condTeam.Arbitrary.(int)) + ` `
	}
	if condArea := q["areaRectangle"]; condArea != nil {
		qKillerCond += fmt.Sprintf(` and round(killer_posx)::int between %v and %v and round(killer_posz)::int between %v and %v `,
			condArea.Args[0], condArea.Args[1], condArea.Args[2], condArea.Args[3])
		qVictimCond += fmt.Sprintf(` and round(victim_posx)::int between %v and %v and round(victim_posz)::int between %v and %v `,
			condArea.Args[0], condArea.Args[1], condArea.Args[2], condArea.Args[3])
	}
	if condArea := q["areaCircle"]; condArea != nil {
		qKillerCond += fmt.Sprintf(` and ST_Distance(ST_Point(%v, %v), ST_Point(killer_posx, killer_posz)) <= %v`,
			condArea.Args[0], condArea.Args[1], condArea.Args[2])
		qVictimCond += fmt.Sprintf(` and ST_Distance(ST_Point(%v, %v), ST_Point(victim_posx, victim_posz)) <= %v`,
			condArea.Args[0], condArea.Args[1], condArea.Args[2])
	}
	return
}

type KillTally struct {
	X, Z   int32
	Kills  uint32
	Deaths uint32
}

func (s *KillsStorage) GetKillCountsByCoord(ctx context.Context, conds QueryConditions) ([]KillTally, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	if !conds.HasAny("level", "mission") {
		return nil, errors.New("specify either level or mission")
	}
	qKillerCond, qVictimCond := conds.SeparatedConds()
	q := `
with unpacked as (
	select round(killer_posx)::int as px, round(killer_posz)::int as pz, 1 as iskill, 0 as isdeath
	from kills
	` + conds.WhereCase() + qKillerCond + `
	union all
	select round(victim_posx)::int as px, round(victim_posz)::int as pz, 0 as iskill, 1 as isdeath
	from kills
	` + conds.WhereCase() + qVictimCond + `
)
select px, pz, sum(iskill) as kills, sum(isdeath) as deaths
from unpacked
group by px, pz
order by px, pz`
	argsOnce := conds.Args()
	args := append(argsOnce, argsOnce...)
	// log.Info().Msg(q + "\n\n" + spew.Sdump(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret KillTally, err error) {
		err = row.Scan(&ret.X, &ret.Z, &ret.Kills, &ret.Deaths)
		return
	})
}

type VehicleAreaStat struct {
	Vehicle string
	Kills   int
	Deaths  int
}

func (s *KillsStorage) GetVehicleStatsByArea(ctx context.Context, conds QueryConditions, limit int) ([]VehicleAreaStat, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	if !conds.HasAny("level", "mission") {
		return nil, errors.New("specify either level or mission")
	}
	qKillerCond, qVictimCond := conds.SeparatedConds()
	q := `
with unpacked as (
	select killer_vehicle as vehicle, count(*) as kills, 0 as deaths
	from kills
	` + conds.WhereCase() + qKillerCond + `
	group by vehicle
	union all
	select victim_vehicle as vehicle, 0 as kills, count(*) as deaths
	from kills
	` + conds.WhereCase() + qVictimCond + `
	group by vehicle
)
select vehicle, sum(kills), sum(deaths)
from unpacked
group by vehicle
order by sum(kills) + sum(deaths) desc`
	if limit > 0 {
		q += ` LIMIT ` + strconv.Itoa(limit) + `;`
	}
	argsOnce := conds.Args()
	args := append(argsOnce, argsOnce...)
	// log.Info().Msg(q + "\n\n" + spew.Sdump(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret VehicleAreaStat, err error) {
		err = row.Scan(&ret.Vehicle, &ret.Kills, &ret.Deaths)
		return
	})
}

type AreaArrow struct {
	FromX int
	FromZ int
	ToX   int
	ToZ   int
}

func (s *KillsStorage) GetAreaArrows(ctx context.Context, conds QueryConditions) ([]AreaArrow, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	if !conds.HasAny("level", "mission") {
		return nil, errors.New("specify either level or mission")
	}
	if !conds.Has("areaRectangle") {
		return nil, errors.New("no rectangle area specified")
	}

	areaHeat, err := s.GetKillCountsByCoord(ctx, conds)
	if err != nil {
		return nil, err
	}

	sort.Slice(areaHeat, func(i, j int) bool {
		return areaHeat[i].Kills < areaHeat[j].Kills
	})
	areaPoints := make([]image.Point, len(areaHeat))
	for i := range areaHeat {
		areaPoints[i] = image.Point{X: int(areaHeat[i].X), Y: int(areaHeat[i].Z)}
	}

	targetGroup := walkPoints(areaPoints, areaPoints[0], 9)

	targetCoords := make([][]int32, len(targetGroup))
	for i := range targetGroup {
		targetCoords[i] = []int32{int32(targetGroup[i].X), int32(targetGroup[i].Y)}
	}

	q := `SELECT
	  ROUND(k.killer_posx)::int AS killer_x,
	  ROUND(k.killer_posz)::int AS killer_z,
	  ROUND(k.victim_posx)::int AS victim_x,
	  ROUND(k.victim_posz)::int AS victim_z,
	  COUNT(k) AS count
	FROM kills k
	WHERE level = $1 AND
	NOT (k.victim_posx BETWEEN $2 AND $3 AND k.victim_posz BETWEEN $4 AND $5) AND
	  (ROUND(k.killer_posx)::int, ROUND(k.killer_posz)::int) IN (
			SELECT a['unnest'][1], a['unnest'][2] FROM UNNEST($6::int[][]) a)
	GROUP BY ROUND(k.killer_posx)::int, ROUND(k.killer_posz)::int, ROUND(k.victim_posx)::int, ROUND(k.victim_posz)::int
	ORDER BY count DESC
	LIMIT 10`
	areaArgs := conds["areaRectangle"].Args
	args := []any{conds["level"].Args[0], areaArgs[0], areaArgs[1], areaArgs[2], areaArgs[3], targetCoords}
	// log.Info().Msg(q + "\n\n" + spew.Sdump(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret AreaArrow, err error) {
		var d1 int
		err = row.Scan(&ret.FromX, &ret.FromZ, &ret.ToX, &ret.ToZ, &d1)
		return
	})
}

type PosStat struct {
	Tally struct {
		Kills, Deaths int
	}
	Angles struct {
		ToKills, DeathsFrom AngleCountTallySlice
	}
}

type AngleCountTally struct {
	Angle uint16
	Count uint16
}

type AngleCountTallySlice []AngleCountTally

func (a AngleCountTallySlice) MarshalJSON() ([]byte, error) {
	ret := &bytes.Buffer{}
	ret.WriteString("\"Angle,Count;")
	for _, v := range a {
		fmt.Fprintf(ret, "%d,%d;", v.Angle, v.Count)
	}
	ret.WriteString("\"")
	return ret.Bytes(), nil
}

func (s *KillsStorage) GetPosStat(ctx context.Context, conds QueryConditions) (*PosStat, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	if !conds.HasAny("level", "mission") {
		return nil, errors.New("specify either level or mission")
	}
	if !conds.HasAny("areaCircle") {
		return nil, errors.New("no circle area specified")
	}
	areaHeat, err := s.GetKillCountsByCoord(ctx, conds)
	if err != nil {
		return nil, err
	}

	areaKills, areaDeaths := 0, 0
	for _, v := range areaHeat {
		areaKills += int(v.Kills)
		areaDeaths += int(v.Deaths)
	}
	ret := &PosStat{
		Tally: struct {
			Kills, Deaths int
		}{
			Kills:  areaKills,
			Deaths: areaDeaths,
		},
		Angles: struct {
			ToKills, DeathsFrom AngleCountTallySlice
		}{},
	}

	qKillerCond, qVictimCond := conds.SeparatedConds()
	q := `
select round(degrees(
		ST_Azimuth(
		ST_Point(round(killer_posx), round(killer_posz)),
		ST_Point(round(victim_posx), round(victim_posz)))
	)), count(*)
from kills
` + conds.WhereCase() + qKillerCond + `
group by 1
order by 2 desc`
	args := conds.Args()
	// log.Info().Msg(q + "\n\n" + spew.Sdump(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	ret.Angles.ToKills, err = CollectRows(rows, func(row CollectableRow) (ret AngleCountTally, err error) {
		err = row.Scan(&ret.Angle, &ret.Count)
		return
	})
	if err != nil {
		return nil, err
	}

	q = `
select round(degrees(
		ST_Azimuth(
		ST_Point(round(victim_posx), round(victim_posz)),
		ST_Point(round(killer_posx), round(killer_posz)))
	)), count(*)
from kills
` + conds.WhereCase() + qVictimCond + `
group by 1
order by 2 desc`
	// log.Info().Msg(q + "\n\n" + spew.Sdump(args))
	rows, err = s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	ret.Angles.DeathsFrom, err = CollectRows(rows, func(row CollectableRow) (ret AngleCountTally, err error) {
		err = row.Scan(&ret.Angle, &ret.Count)
		return
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func walkPoints(group []image.Point, start image.Point, iters int) []image.Point {
	targetGroup := []image.Point{}
	toVisit := []image.Point{start}
	next := []image.Point{}
	for len(toVisit) > 0 && iters > 0 {
		iters--
		for _, v := range toVisit {
			if !slices.Contains(group, v) {
				continue
			}
			if !slices.Contains(targetGroup, v) {
				targetGroup = append(targetGroup, v)
			}
			for _, v2 := range []image.Point{
				image.Point{X: v.X - 1, Y: v.Y},
				image.Point{X: v.X + 1, Y: v.Y},
				image.Point{X: v.X, Y: v.Y - 1},
				image.Point{X: v.X, Y: v.Y + 1}} {
				if !slices.Contains(targetGroup, v2) {
					next = append(next, v2)
				}
			}
		}
		toVisit = toVisit[:0]
		next, toVisit = toVisit, next
	}
	return targetGroup
}
