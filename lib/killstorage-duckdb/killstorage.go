package killstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"main/lib/lux/luxproto/luxprotogen"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"

	"github.com/duckdb/duckdb-go/v2"
	_ "github.com/duckdb/duckdb-go/v2"
)

func NewKillsStorage(dbpath string) (*KillsStorage, error) {
	dbConnector, db, err := initDb(dbpath)
	if err != nil {
		return nil, err
	}
	ret := &KillsStorage{
		db:          db,
		dbConnector: dbConnector,
	}
	return ret, nil
}

func initDb(dbpath string) (*duckdb.Connector, *sql.DB, error) {
	dbConnector, err := duckdb.NewConnector(dbpath, nil)
	if err != nil {
		return nil, nil, err
	}
	db := sql.OpenDB(dbConnector)
	if err != nil {
		return nil, nil, err
	}
	_, err = db.ExecContext(context.Background(), `create table if not exists kills (
	session ubigint not null,
	session_time timestamp not null,
	kill_time ubigint not null,
	level varchar not null,
	mission varchar not null,
	killer_id ubigint not null,
	killer_team utinyint not null,
	killer_vehicle varchar not null,
	killer_posx real not null,
	killer_posz real not null,
	weapon varchar not null,
	victim_id ubigint not null,
	victim_team utinyint not null,
	victim_vehicle varchar not null,
	victim_posx real not null,
	victim_posz real not null
);`)
	if err != nil {
		return nil, nil, err
	}
	return dbConnector, db, nil
}

type Kill struct {
	Session       uint64
	SessionTime   uint64
	KillTime      uint64
	Level         string
	Mission       string
	KillerID      uint64
	KillerTeam    byte
	KillerVehicle string
	KillerPosX    float64
	KillerPosZ    float64
	Weapon        string
	VictimID      uint64
	VictimTeam    byte
	VictimVehicle string
	VictimPosX    float64
	VictimPosZ    float64
}

type KillsStorage struct {
	dbConnector *duckdb.Connector
	db          *sql.DB
}

func (s *KillsStorage) StoreKills(toinsert []Kill) error {
	conn, err := s.dbConnector.Connect(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	appender, err := duckdb.NewAppenderFromConn(conn, "", "kills")
	if err != nil {
		return err
	}
	defer appender.Close()
	for _, k := range toinsert {
		sessionTime := time.Unix(int64(k.SessionTime), 0)
		err = appender.AppendRow(k.Session, sessionTime, k.KillTime, k.Level, k.Mission,
			k.KillerID, k.KillerTeam, k.KillerVehicle, k.KillerPosX, k.KillerPosZ, k.Weapon,
			k.VictimID, k.VictimTeam, k.VictimVehicle, k.VictimPosX, k.VictimPosZ)
		if err != nil {
			os.WriteFile("err.spew", []byte(spew.Sdump(toinsert)), 0644)
			return fmt.Errorf("kills insert: %w", err)
		}
	}
	err = appender.Flush()
	if err != nil {
		return err
	}
	return nil
}

func (s *KillsStorage) GetVehicles(ctx context.Context) (vehicles []string, err error) {
	rows, err := s.db.QueryContext(ctx, `select distinct v from (
		select distinct killer_vehicle as v from kills
		union all
		select distinct victim_vehicle as v from kills);`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []string{}, nil
		}
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret string, err error) {
		err = row.Scan(&ret)
		return
	})
}

type QueryConditions struct {
	whereConds    []string
	whereArgs     []any
	hasTeamFilter bool
	teamFilter    int
}

func (s *KillsStorage) QueryWithLevel(q *QueryConditions, level string) bool {
	q.whereArgs = append(q.whereArgs, level)
	q.whereConds = append(q.whereConds, fmt.Sprintf("level = $%d", len(q.whereArgs)))
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
	q.whereConds = append(q.whereConds, fmt.Sprintf("p.x >= $%d and p.x < $%d and p.z >= $%d and p.z < $%d", n-3, n-2, n-1, n))
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
		qKillValue = `case when t.killer_team = ` + strconv.Itoa(conds.teamFilter) + ` then +1 else 0 end`
		qDeathValue = `case when t.victim_team = ` + strconv.Itoa(conds.teamFilter) + ` then -1 else 0 end`
	}
	q := `SELECT
  (ROUND(p.x))::int AS x,
  (ROUND(p.z))::int AS z,
  SUM(p.delta)      AS score,
  COUNT(p)          AS count
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
	// log.Info().Msg(q)
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
	q := `SELECT
  p.vehicle,
  COUNT(*) FILTER (WHERE p.delta > 0) AS kills,
  COUNT(*) FILTER (WHERE p.delta < 0) AS deaths
FROM kills t
CROSS JOIN LATERAL (
  VALUES
    (t.killer_vehicle, t.killer_posx, t.killer_posz,  1),
    (t.victim_vehicle, t.victim_posx, t.victim_posz, -1)
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
	return CollectRows(rows, func(row CollectableRow) (ret VehicleAreaStat, err error) {
		err = row.Scan(&ret.Vehicle, &ret.Kills, &ret.Deaths)
		return
	})
}

type AmountsByLevelRow struct {
	LevelName string
	Count     int
}

func (s *KillsStorage) GetAmountsByLevel(ctx context.Context) ([]AmountsByLevelRow, error) {
	rows, err := s.db.QueryContext(ctx, `select level, count(*) from kills group by 1 order by 2 desc;`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []AmountsByLevelRow{}, nil
		}
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (ret AmountsByLevelRow, err error) {
		err = row.Scan(&ret.LevelName, &ret.Count)
		return
	})
}

func (s *KillsStorage) GetAmountsByDay(ctx context.Context) (map[time.Time]int, error) {
	rows, err := s.db.QueryContext(ctx, `select date_trunc('day', session_time), count(*) from kills group by 1 order by 1 desc;`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[time.Time]int{}, nil
		}
		return nil, err
	}
	var s1 time.Time
	var s2 int
	ret := map[time.Time]int{}
	err = ForEachRow(rows, []any{&s1, &s2}, func() error {
		ret[s1] = s2
		return nil
	})
	return ret, err
}

func (s *KillsStorage) GetAmountsByKillerVehicle(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `select killer_vehicle, count(*) from kills group by killer_vehicle`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]int{}, nil
		}
		return nil, err
	}
	var name string
	var c int
	ret := map[string]int{}
	err = ForEachRow(rows, []any{&name, &c}, func() error {
		ret[name] = c
		return nil
	})
	return ret, err
}

func (s *KillsStorage) GetAmountsByVictimVehicle(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `select victim_vehicle, count(*) from kills group by victim_vehicle`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]int{}, nil
		}
		return nil, err
	}
	var name string
	var c int
	ret := map[string]int{}
	err = ForEachRow(rows, []any{&name, &c}, func() error {
		ret[name] = c
		return nil
	})
	return ret, err
}

func (s *KillsStorage) GetAmountsByVehicle(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `select v, sum(c) from (
		select k.killer_vehicle as v, count(*) as c from kills k group by v
		union all
		select k.victim_vehicle as v, count(*) as c from kills k group by v
	) group by v;`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]int{}, nil
		}
		return nil, err
	}
	var name string
	var c int
	ret := map[string]int{}
	err = ForEachRow(rows, []any{&name, &c}, func() error {
		ret[name] = c
		return nil
	})
	return ret, err
}

type InterestKillsDeathsRow struct {
	ID     uint64
	Kills  int
	Deaths int
}

func (s *KillsStorage) GetAmountsOfInteresting(ctx context.Context, ids []uint64) ([]InterestKillsDeathsRow, error) {
	rows, err := s.db.QueryContext(ctx, `with interest_ids(id) AS (select unnest($1::ubigint[]))
	select r.id,
    coalesce(sum(case when k.killer_id = r.id then 1 else 0 end), 0) as killer_count,
    coalesce(sum(case when k.victim_id = r.id then 1 else 0 end), 0) as victim_count
	from interest_ids r
	left join kills k on (k.killer_id = r.id or k.victim_id = r.id)
	group by r.id
	order by r.id;
`, ids)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []InterestKillsDeathsRow{}, nil
		}
		return nil, err
	}
	return CollectRows(rows, func(row CollectableRow) (r InterestKillsDeathsRow, err error) {
		err = row.Scan(&r.ID, &r.Kills, &r.Deaths)
		return
	})
}

func (s *KillsStorage) Close() {
	s.db.Close()
}

func LuxCarveToKills(carve *luxprotogen.Replay) (ret []Kill, err error) {
	ret = []Kill{}
	if carve == nil {
		return
	}
	if carve.Light == nil {
		return ret, errors.New("carve.Light is nill")
	}
	sessionID, err := strconv.ParseUint(carve.Light.Id, 10, 64)
	if err != nil {
		return ret, fmt.Errorf("parsing session id number string %q: %w", carve.Light.Id, err)
	}
	for _, kill := range carve.Kills {
		if kill == nil {
			continue
		}
		if len(kill.OffendedUid) == 0 || kill.OffendedUid == "0" || kill.OffendedUid[0] == '-' {
			continue
		}
		if len(kill.OffenderUid) == 0 || kill.OffenderUid == "0" || kill.OffenderUid[0] == '-' {
			continue
		}
		killerID, err := strconv.ParseUint(kill.OffenderUid, 10, 64)
		if err != nil {
			return ret, fmt.Errorf("parsing offender uid string %q: %w", kill.OffenderUid, err)
		}
		victimID, err := strconv.ParseUint(kill.OffendedUid, 10, 64)
		if err != nil {
			return ret, fmt.Errorf("parsing offended uid string %q: %w", kill.OffendedUid, err)
		}
		if len(kill.OffendedPos) != 3 {
			return ret, fmt.Errorf("offended pos is not 3 elements: %v", kill.OffendedPos)
		}
		if len(kill.OffenderPos) != 3 {
			return ret, fmt.Errorf("offender pos is not 3 elements: %v", kill.OffenderPos)
		}
		killerTeam, err := luxCarveToKillsGetPlayerTeam(carve.Light.Players, kill.OffenderUid)
		if err != nil {
			return ret, err
		}
		victimTeam, err := luxCarveToKillsGetPlayerTeam(carve.Light.Players, kill.OffendedUid)
		if err != nil {
			return ret, err
		}
		if killerTeam > 2 || victimTeam > 2 {
			return ret, fmt.Errorf("victim or killer team oob %d %d", killerTeam, victimTeam)
		}
		if !strings.HasPrefix(strings.ToLower(kill.OffenderUnitId), "tankmodels/") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(kill.OffendedUnitId), "tankmodels/") {
			continue
		}
		ret = append(ret, Kill{
			Session:       sessionID,
			SessionTime:   uint64(carve.Light.StartTs),
			KillTime:      uint64(kill.Time),
			Level:         carve.Light.LevelPath,
			Mission:       carve.Light.MissionPath,
			KillerID:      killerID,
			KillerTeam:    byte(killerTeam),
			KillerVehicle: strings.ToLower(kill.OffenderUnitId),
			KillerPosX:    float64(kill.OffenderPos[0]) / float64(carve.PosQuant),
			KillerPosZ:    float64(kill.OffenderPos[2]) / float64(carve.PosQuant),
			Weapon:        kill.UsedWeaponId,
			VictimID:      victimID,
			VictimTeam:    byte(victimTeam),
			VictimVehicle: strings.ToLower(kill.OffendedUnitId),
			VictimPosX:    float64(kill.OffendedPos[0]) / float64(carve.PosQuant),
			VictimPosZ:    float64(kill.OffendedPos[2]) / float64(carve.PosQuant),
		})
	}
	return
}

func luxCarveToKillsGetPlayerTeam(players []*luxprotogen.Player, uid string) (int, error) {
	for _, p := range players {
		if p == nil {
			continue
		}
		if p.Uid == uid {
			return int(p.Team), nil
		}
	}
	return 0, fmt.Errorf("player was not found (%q)", uid)
}

// from pgx

// AppendRows iterates through rows, calling fn for each row, and appending the results into a slice of T.
//
// This function closes the rows automatically on return.
func AppendRows[T any, S ~[]T](slice S, rows *sql.Rows, fn RowToFunc[T]) (S, error) {
	defer rows.Close()

	for rows.Next() {
		value, err := fn(rows)
		if err != nil {
			return nil, err
		}
		slice = append(slice, value)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return slice, nil
}

// CollectRows iterates through rows, calling fn for each row, and collecting the results into a slice of T.
//
// This function closes the rows automatically on return.
func CollectRows[T any](rows *sql.Rows, fn RowToFunc[T]) ([]T, error) {
	return AppendRows([]T{}, rows, fn)
}

type RowToFunc[T any] func(row CollectableRow) (T, error)

type CollectableRow interface {
	Scan(dest ...any) error
}

// ForEachRow iterates through rows. For each row it scans into the elements of scans and calls fn. If any row
// fails to scan or fn returns an error the query will be aborted and the error will be returned. Rows will be closed
// when ForEachRow returns.
func ForEachRow(rows *sql.Rows, scans []any, fn func() error) error {
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(scans...)
		if err != nil {
			return err
		}

		err = fn()
		if err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	return nil
}
