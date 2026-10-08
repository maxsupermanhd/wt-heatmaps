package killstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/rs/zerolog/log"
)

type QueryConditions struct {
	whereConds    []string
	whereArgs     []any
	level         string
	hasTeamFilter bool
	teamFilter    int
	region        *image.Rectangle
}

func (s *KillsStorage) QueryWithLevel(q *QueryConditions, level string) bool {
	q.whereArgs = append(q.whereArgs, level)
	q.whereConds = append(q.whereConds, fmt.Sprintf("level = $%d", len(q.whereArgs)))
	q.level = level
	return true
}

func (s *KillsStorage) QueryWithKillerVehicles(q *QueryConditions, vehicles []string) {
	q.whereArgs = append(q.whereArgs, vehicles)
	q.whereConds = append(q.whereConds, fmt.Sprintf("killer_vehicle = any($%d)", len(q.whereArgs)))
}

func (s *KillsStorage) QueryWithVictimVehicles(q *QueryConditions, vehicles []string) {
	q.whereArgs = append(q.whereArgs, vehicles)
	q.whereConds = append(q.whereConds, fmt.Sprintf("victim_vehicle = any($%d)", len(q.whereArgs)))
}

func (q *QueryConditions) QueryWithSessionTimeMin(tsFrom time.Time) {
	q.whereArgs = append(q.whereArgs, tsFrom)
	q.whereConds = append(q.whereConds, fmt.Sprintf("session_time >= $%d", len(q.whereArgs)))
}

func (q *QueryConditions) QueryWithSessionTimeMax(tsTo time.Time) {
	q.whereArgs = append(q.whereArgs, tsTo)
	q.whereConds = append(q.whereConds, fmt.Sprintf("session_time < $%d", len(q.whereArgs)))
}

func (q *QueryConditions) QueryWithTeam(team int) {
	q.hasTeamFilter = true
	q.teamFilter = team
}

func (q *QueryConditions) QueryWithKillTimeMin(killTimeMin time.Duration) {
	q.whereArgs = append(q.whereArgs, killTimeMin.Milliseconds())
	q.whereConds = append(q.whereConds, fmt.Sprintf("kill_time >= $%d", len(q.whereArgs)))
}

func (q *QueryConditions) QueryWithKillTimeMax(killTimeMax time.Duration) {
	q.whereArgs = append(q.whereArgs, killTimeMax.Milliseconds())
	q.whereConds = append(q.whereConds, fmt.Sprintf("kill_time <= $%d", len(q.whereArgs)))
}

// QueryWithArea keeps the kills inside a box of world meters. The low edge
// counts and the high edge does not, so two boxes that touch do not count a
// kill on the line they share two times.
func (q *QueryConditions) QueryWithArea(x0, z0, x1, z1 float64) {
	q.whereArgs = append(q.whereArgs, min(x0, x1), max(x0, x1), min(z0, z1), max(z0, z1))
	n := len(q.whereArgs)
	q.whereConds = append(q.whereConds, fmt.Sprintf("p.x >= $%d AND p.x < $%d AND p.z >= $%d AND p.z < $%d", n-3, n-2, n-1, n))
	rect := image.Rect(int(min(x0, x1)), int(min(z0, z1)), int(max(z0, z1)), int(max(x0, x1)))
	q.region = &rect
}

func (q *QueryConditions) QueryWithArbitrary(cond string) {
	q.whereConds = append(q.whereConds, cond)
}

func (q *QueryConditions) WhereCase() string {
	if len(q.whereConds) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(q.whereConds, " AND ")
}

func (q *QueryConditions) Dump() string {
	return q.WhereCase() + "\n" + spew.Sdump(q.whereArgs)
}

type KillTally struct {
	X, Z  int
	Score int
	Count int
}

func (s *KillsStorage) GetKillCountsByCoord(ctx context.Context, conds *QueryConditions) ([]KillTally, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	qKillValue := "+1"
	qDeathValue := "-1"
	if conds.hasTeamFilter {
		qKillValue = `CASE WHEN t.killer_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN +1 ELSE 0 END`
		qDeathValue = `CASE WHEN t.victim_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN -1 ELSE 0 END`
	}
	q := `SELECT
  (ROUND(p.x))::int AS x,
  (ROUND(p.z))::int AS z,
  SUM(p.delta) AS score,
  COUNT(p) FILTER (WHERE p.delta <> 0) AS count
FROM kills t
CROSS JOIN LATERAL (
  VALUES
    (t.killer_posx, t.killer_posz, ` + qKillValue + `),
    (t.victim_posx, t.victim_posz, ` + qDeathValue + `)
) AS p(x, z, delta)
` + conds.WhereCase() + `
GROUP BY (ROUND(p.x))::int, (ROUND(p.z))::int;`
	rows, err := s.db.QueryContext(ctx, q, conds.whereArgs...)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	log.Info().Msg(q + "\n" + spew.Sdump(conds.whereArgs))
	return CollectRows(rows, func(row CollectableRow) (ret KillTally, err error) {
		err = row.Scan(&ret.X, &ret.Z, &ret.Score, &ret.Count)
		return
	})
}

type VehicleAreaStat struct {
	Vehicle string
	Kills   int
	Deaths  int
}

func (s *KillsStorage) GetVehicleStatsByArea(ctx context.Context, conds *QueryConditions, limit int) ([]VehicleAreaStat, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	qKillValue := "+1"
	qDeathValue := "-1"
	if conds.hasTeamFilter {
		qKillValue = `CASE WHEN t.killer_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN +1 ELSE 0 END`
		qDeathValue = `CASE WHEN t.victim_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN -1 ELSE 0 END`
	}
	q := `SELECT
  p.vehicle,
  COUNT(*) FILTER (WHERE p.delta > 0) AS kills,
  COUNT(*) FILTER (WHERE p.delta < 0) AS deaths
FROM kills t
CROSS JOIN LATERAL (
  VALUES
    (t.killer_vehicle, t.killer_posx, t.killer_posz, ` + qKillValue + `),
    (t.victim_vehicle, t.victim_posx, t.victim_posz, ` + qDeathValue + `)
) AS p(vehicle, x, z, delta)
` + conds.WhereCase() + `
GROUP BY p.vehicle
ORDER BY COUNT(*) DESC`
	if limit > 0 {
		q += ` LIMIT ` + strconv.Itoa(limit) + `;`
	}
	rows, err := s.db.QueryContext(ctx, q, conds.whereArgs...)
	if err != nil {
		return nil, err
	}
	log.Info().Msg(q + "\n" + spew.Sdump(conds.whereArgs))
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

func (s *KillsStorage) GetAreaArrows(ctx context.Context, conds *QueryConditions) ([]AreaArrow, error) {
	if conds == nil {
		return nil, errors.ErrUnsupported
	}
	if conds.region == nil {
		return nil, errors.ErrUnsupported
	}
	qKillValue := "+1"
	qDeathValue := "-1"
	if conds.hasTeamFilter {
		qKillValue = `CASE WHEN t.killer_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN +1 ELSE 0 END`
		qDeathValue = `CASE WHEN t.victim_team = ` + strconv.Itoa(conds.teamFilter) + ` THEN -1 ELSE 0 END`
	}
	q := `SELECT
      ROUND(p.x)::int AS x,
      ROUND(p.z)::int AS z,
      SUM(p.delta) AS score,
      COUNT(p) FILTER (WHERE p.delta <> 0) AS count
    FROM kills k
    CROSS JOIN LATERAL (
      VALUES
        (k.killer_posx, k.killer_posz, ` + qKillValue + `),
        (k.victim_posx, k.victim_posz, ` + qDeathValue + `)
    ) AS p(x, z, delta)
    ` + conds.WhereCase() + `
    GROUP BY ROUND(p.x)::int, ROUND(p.z)::int
    HAVING score > 0
    ORDER BY count DESC`
	rows, err := s.db.QueryContext(ctx, q, conds.whereArgs...)
	if err != nil {
		return nil, err
	}
	// log.Info().Msg(q + "\n" + spew.Sdump(conds.whereArgs))
	cd, err := CollectRows(rows, func(row CollectableRow) (ret image.Point, err error) {
		var d1, d2 int
		err = row.Scan(&ret.X, &ret.Y, &d1, &d2)
		return
	})
	if err != nil {
		return nil, err
	}
	if len(cd) == 0 {
		return nil, nil
	}

	targetGroup := walkPoints(cd, cd[0], 9)

	targetCoords := make([][]int32, len(targetGroup))
	for i := range targetGroup {
		targetCoords[i] = []int32{int32(targetGroup[i].X), int32(targetGroup[i].Y)}
	}

	q = `SELECT
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
	args := []any{conds.level, conds.region.Min.X, conds.region.Max.X, conds.region.Min.Y, conds.region.Max.Y, targetCoords}
	// log.Info().Msg(q + "\n" + spew.Sdump(args))
	rows, err = s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret AreaArrow, err error) {
		var d1 int
		err = row.Scan(&ret.FromX, &ret.FromZ, &ret.ToX, &ret.ToZ, &d1)
		return
	})
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
