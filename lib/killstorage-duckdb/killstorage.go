package killstorage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"main/lib/caches"
	"main/lib/lux/luxproto/luxprotogen"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"

	_ "github.com/duckdb/duckdb-go/v2"
)

/*

# db prep

create user thunder with password 'warthunder_analytics_or_something';
create database thunder with owner thunder;

*/

func NewKillsStorage(dbpath string) (*KillsStorage, error) {
	db, err := initDb(dbpath)
	if err != nil {
		return nil, err
	}
	ret := &KillsStorage{
		db: db,
	}
	ret.cLevels, err = prepareDict(db, "level_names")
	if err != nil {
		return nil, err
	}
	ret.cMissions, err = prepareDict(db, "mission_names")
	if err != nil {
		return nil, err
	}
	ret.cVehicles, err = prepareDict(db, "vehicle_names")
	if err != nil {
		return nil, err
	}
	ret.cWeapons, err = prepareDict(db, "weapon_names")
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func prepareDict(db *sql.DB, tableName string) (*caches.GenIDTwoWayMap[int, string], error) {
	initial, err := queryDict(db, tableName)
	return caches.NewCachedDictTable(initial, genInsertDictTable(db, tableName)), err
}

func queryDict(db *sql.DB, tableName string) (ret map[int]string, err error) {
	ret = map[int]string{}
	rows, err := db.QueryContext(context.Background(), `select id, name from `+tableName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ret, nil
		}
		return ret, err
	}
	defer rows.Close()
	var (
		id   int
		name string
	)
	for rows.Next() {
		err = rows.Scan(&id, &name)
		if err != nil {
			return ret, err
		}
		ret[id] = name
	}
	return
}

func genInsertDictTable(db *sql.DB, tableName string) caches.GenIDFn[int, string] {
	return func(v string) (ret int, err error) {
		err = db.QueryRowContext(context.Background(), `select id from `+tableName+` where name = $1`, v).Scan(&ret)
		if err == nil {
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			err = db.QueryRowContext(context.Background(), `with res as (insert into `+tableName+` (name) values ($1) on conflict do nothing returning id)
select id from res
union all
select id from `+tableName+` where name=$1
limit 1`, v).Scan(&ret)
		}
		return
	}
}

func initDb(dbpath string) (*sql.DB, error) {
	db, err := sql.Open("duckdb", dbpath)
	if err != nil {
		return nil, err
	}
	for _, dictName := range []string{"level_names", "mission_names", "vehicle_names", "weapon_names"} {
		_, err = db.ExecContext(context.Background(), `create sequence if not exists `+dictName+`_id_seq;`)
		if err != nil {
			return nil, err
		}
		_, err = db.ExecContext(context.Background(), `create table if not exists `+dictName+` (id integer primary key default nextval('`+dictName+`_id_seq'), name text not null unique);`)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	_, err = db.ExecContext(context.Background(), `create table if not exists kills (
	session ubigint not null,
	session_time timestamp not null,
	kill_time ubigint not null,
	level integer references level_names (id) not null,
	mission integer references mission_names (id) not null,
	killer_id ubigint not null,
	killer_team utinyint not null,
	killer_vehicle integer references vehicle_names (id) not null,
	killer_posx real not null,
	killer_posz real not null,
	weapon integer references weapon_names (id) not null,
	victim_id ubigint not null,
	victim_team utinyint not null,
	victim_vehicle integer references vehicle_names (id) not null,
	victim_posx real not null,
	victim_posz real not null
);`)
	if err != nil {
		return nil, err
	}
	return db, nil
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
	db *sql.DB

	lock      sync.Mutex
	cLevels   *caches.GenIDTwoWayMap[int, string]
	cMissions *caches.GenIDTwoWayMap[int, string]
	cVehicles *caches.GenIDTwoWayMap[int, string]
	cWeapons  *caches.GenIDTwoWayMap[int, string]
}

func (s *KillsStorage) StoreKills(toinsert []Kill) error {
	s.lock.Lock()
	idsLevel := make([]int, len(toinsert))
	idsMission := make([]int, len(toinsert))
	idsKillerVehicle := make([]int, len(toinsert))
	idsVictimVehicle := make([]int, len(toinsert))
	idsWeapon := make([]int, len(toinsert))
	var err error
	for i, k := range toinsert {
		idsLevel[i], err = s.cLevels.GetIDNOLOCK(k.Level)
		if err != nil {
			s.lock.Unlock()
			return fmt.Errorf("get id of level %q (kill %d): %w", k.Level, i, err)
		}
		idsMission[i], err = s.cMissions.GetIDNOLOCK(k.Mission)
		if err != nil {
			s.lock.Unlock()
			return fmt.Errorf("get id of mission %q (kill %d): %w", k.Level, i, err)
		}
		idsKillerVehicle[i], err = s.cVehicles.GetIDNOLOCK(k.KillerVehicle)
		if err != nil {
			s.lock.Unlock()
			return fmt.Errorf("get id of killer vehicle %q (kill %d): %w", k.KillerVehicle, i, err)
		}
		idsWeapon[i], err = s.cWeapons.GetIDNOLOCK(k.Weapon)
		if err != nil {
			s.lock.Unlock()
			return fmt.Errorf("get id of weapon %q (kill %d): %w", k.Weapon, i, err)
		}
		idsVictimVehicle[i], err = s.cVehicles.GetIDNOLOCK(k.VictimVehicle)
		if err != nil {
			s.lock.Unlock()
			return fmt.Errorf("get id of victim vehicle %q (kill %d): %w", k.VictimVehicle, i, err)
		}
	}
	s.lock.Unlock()
	for i, k := range toinsert {
		sessionTime := time.Unix(int64(k.SessionTime), 0)
		s.db.Exec(`insert into kills (
				session, session_time, kill_time, level, mission,
				killer_id, killer_team, killer_vehicle, killer_posx, killer_posz, weapon,
				victim_id, victim_team, victim_vehicle, victim_posx, victim_posz
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);`,
			k.Session, sessionTime, k.KillTime, idsLevel[i], idsMission[i],
			k.KillerID, k.KillerTeam, idsKillerVehicle[i], k.KillerPosX, k.KillerPosZ, idsWeapon[i],
			k.VictimID, k.VictimTeam, idsVictimVehicle[i], k.VictimPosX, k.VictimPosZ)
		if err != nil {
			os.WriteFile("err.spew", []byte(spew.Sdump(toinsert)), 0644)
			return fmt.Errorf("kills insert: %w", err)
		}
	}
	return nil
}

func (s *KillsStorage) GetMeta() (levels, missions, vehicles, weapons []string) {
	s.lock.Lock()
	levels = slices.Collect(maps.Values(s.cLevels.Values))
	missions = slices.Collect(maps.Values(s.cMissions.Values))
	vehicles = slices.Collect(maps.Values(s.cVehicles.Values))
	weapons = slices.Collect(maps.Values(s.cWeapons.Values))
	s.lock.Unlock()
	return
}

func (s *KillsStorage) GetDictLevels() (levels map[int]string) {
	s.lock.Lock()
	levels = maps.Clone(s.cLevels.Values)
	s.lock.Unlock()
	return
}

func (s *KillsStorage) GetDictVehicles() (vehicles map[int]string) {
	s.lock.Lock()
	vehicles = maps.Clone(s.cVehicles.Values)
	s.lock.Unlock()
	return
}

type QueryConditions struct {
	whereConds []string
	whereArgs  []any
}

func (s *KillsStorage) QueryWithLevel(q *QueryConditions, level string) bool {
	s.lock.Lock()
	levelID, ok := s.cLevels.GetExistingIDNOLOCK(level)
	s.lock.Unlock()
	if !ok {
		return false
	}
	q.whereArgs = append(q.whereArgs, levelID)
	q.whereConds = append(q.whereConds, fmt.Sprintf("level = $%d", len(q.whereArgs)))
	return true
}

func (s *KillsStorage) QueryWithKillerVehicles(q *QueryConditions, vehicles []string) {
	s.lock.Lock()
	vehicleIDs := make([]int, 0, len(vehicles))
	for _, v := range vehicles {
		id, ok := s.cVehicles.GetExistingIDNOLOCK(v)
		if ok {
			vehicleIDs = append(vehicleIDs, id)
		}
	}
	s.lock.Unlock()
	q.whereArgs = append(q.whereArgs, vehicleIDs)
	q.whereConds = append(q.whereConds, fmt.Sprintf("killer_vehicle = any($%d)", len(q.whereArgs)))
}

func (s *KillsStorage) QueryWithVictimVehicles(q *QueryConditions, vehicles []string) {
	s.lock.Lock()
	vehicleIDs := make([]int, 0, len(vehicles))
	for _, v := range vehicles {
		id, ok := s.cVehicles.GetExistingIDNOLOCK(v)
		if ok {
			vehicleIDs = append(vehicleIDs, id)
		}
	}
	s.lock.Unlock()
	q.whereArgs = append(q.whereArgs, vehicleIDs)
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

func (q *QueryConditions) QueryWithKillerTeam(killerTeam int) {
	q.whereArgs = append(q.whereArgs, killerTeam)
	q.whereConds = append(q.whereConds, fmt.Sprintf("killer_team = $%d", len(q.whereArgs)))
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
	q := `SELECT
  (ROUND(p.x))::int AS x,
  (ROUND(p.z))::int AS z,
  SUM(p.delta)      AS score,
  COUNT(p)          AS count
FROM kills t
CROSS JOIN LATERAL (
  VALUES
    (t.killer_posx, t.killer_posz,  1),
    (t.victim_posx, t.victim_posz, -1)
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
  n.name,
  COUNT(*) FILTER (WHERE p.delta > 0) AS kills,
  COUNT(*) FILTER (WHERE p.delta < 0) AS deaths
FROM kills t
CROSS JOIN LATERAL (
  VALUES
    (t.killer_vehicle, t.killer_posx, t.killer_posz,  1),
    (t.victim_vehicle, t.victim_posx, t.victim_posz, -1)
) AS p(vehicle, x, z, delta)
JOIN vehicle_names n ON n.id = p.vehicle
` + conds.WhereCase() + `
GROUP BY n.name
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
	rows, err := s.db.QueryContext(ctx, `select name, count(*) from kills left join level_names on level_names.id = kills.level group by 1 order by 2 desc;`)
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
	rows, err := s.db.QueryContext(ctx, `select vn.name, count(*) from kills left join vehicle_names as vn on vn.id = killer_vehicle group by vn.name`)
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
	rows, err := s.db.QueryContext(ctx, `select vn.name, count(*) from kills left join vehicle_names as vn on vn.id = victim_vehicle group by vn.name`)
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
	rows, err := s.db.QueryContext(ctx, `select
		vn.name, sum(p.seen) as s
	from kills k
	cross join lateral (
  values
    (k.killer_vehicle, 1),
    (k.victim_vehicle, 1)
	) as p(vehicle, seen)
	left join vehicle_names as vn on vn.id = p.vehicle
	group by vn.name
	order by s desc`)
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
