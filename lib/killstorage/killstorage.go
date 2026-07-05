package killstorage

import (
	"context"
	"errors"
	"fmt"
	"main/lib/caches"
	"main/lib/lux"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
		db:         db,
		partitions: []string{},
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

func prepareDict(db *pgxpool.Pool, tableName string) (*caches.GenIDTwoWayMap[int, string], error) {
	initial, err := queryDict(db, tableName)
	return caches.NewCachedDictTable(initial, genInsertDictTable(db, tableName)), err
}

func queryDict(db *pgxpool.Pool, tableName string) (ret map[int]string, err error) {
	ret = map[int]string{}
	rows, err := db.Query(context.Background(), `select id, name from `+tableName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ret, nil
		}
		return ret, err
	}
	var (
		id   int
		name string
	)
	_, err = pgx.ForEachRow(rows, []any{&id, &name}, func() error {
		ret[id] = name
		return nil
	})
	return
}

func genInsertDictTable(db *pgxpool.Pool, tableName string) caches.GenIDFn[int, string] {
	return func(v string) (ret int, err error) {
		err = db.QueryRow(context.Background(), `select id from `+tableName+` where name = $1`, v).Scan(&ret)
		if err == nil {
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			err = db.QueryRow(context.Background(), `with res as (insert into `+tableName+` (name) values ($1) on conflict do nothing returning id)
select id from res
union all
select id from `+tableName+` where name=$1
limit 1`, v).Scan(&ret)
		}
		return
	}
}

func initDb(dbpath string) (*pgxpool.Pool, error) {
	db, err := pgxpool.New(context.Background(), dbpath)
	if err != nil {
		return nil, err
	}
	for _, dictName := range []string{"level_names", "mission_names", "vehicle_names", "weapon_names"} {
		_, err = db.Exec(context.Background(), `create table if not exists `+dictName+` (id serial primary key, name text not null unique);`)
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(context.Background(), `create table if not exists kills (
	session bigint not null,
	session_time timestamp not null,
	kill_time bigint not null,
	level integer references level_names (id) not null,
	mission integer references mission_names (id) not null,
	killer_id bigint not null,
	killer_team smallint not null,
	killer_vehicle integer references vehicle_names (id) not null,
	killer_posx real not null,
	killer_posz real not null,
	weapon integer references weapon_names (id) not null,
	victim_id bigint not null,
	victim_team smallint not null,
	victim_vehicle integer references vehicle_names (id) not null,
	victim_posx real not null,
	victim_posz real not null
) partition by range (session_time);`)
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
	db *pgxpool.Pool

	lock       sync.Mutex
	cLevels    *caches.GenIDTwoWayMap[int, string]
	cMissions  *caches.GenIDTwoWayMap[int, string]
	cVehicles  *caches.GenIDTwoWayMap[int, string]
	cWeapons   *caches.GenIDTwoWayMap[int, string]
	partitions []string
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
		t := time.Unix(int64(k.SessionTime), 0)
		tableName := fmt.Sprintf("kills_y%dm%dd%d", t.Year(), t.Month(), t.Day())
		if !slices.Contains(s.partitions, tableName) {
			t2 := t.Add(24 * time.Hour)
			_, err = s.db.Exec(context.Background(), fmt.Sprintf(`create table if not exists %s partition of kills
				for values from ('%d-%d-%d 00:00:00') to ('%d-%d-%d 00:00:00')`, tableName,
				t.Year(), t.Month(), t.Day(),
				t2.Year(), t2.Month(), t2.Day()))
			if err != nil {
				s.lock.Unlock()
				return fmt.Errorf("partition %q %q create: %w", t.String(), t2.String(), err)
			}
			s.partitions = append(s.partitions, tableName)
		}
	}
	s.lock.Unlock()
	b := &pgx.Batch{}
	for i, k := range toinsert {
		sessionTime := time.Unix(int64(k.SessionTime), 0)
		b.Queue(`insert into kills (
				session, session_time, kill_time, level, mission,
				killer_id, killer_team, killer_vehicle, killer_posx, killer_posz, weapon,
				victim_id, victim_team, victim_vehicle, victim_posx, victim_posz
			) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);`,
			k.Session, sessionTime, k.KillTime, idsLevel[i], idsMission[i],
			k.KillerID, k.KillerTeam, idsKillerVehicle[i], k.KillerPosX, k.KillerPosZ, idsWeapon[i],
			k.VictimID, k.VictimTeam, idsVictimVehicle[i], k.VictimPosX, k.VictimPosZ)
	}
	br := s.db.SendBatch(context.Background(), b)
	err = br.Close()
	if err != nil {
		os.WriteFile("err.spew", []byte(spew.Sdump(toinsert)), 0644)
		return fmt.Errorf("batch send: %w", err)
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

type queryConditions struct {
	whereCase string
	whereArgs []any
}

func NewKillsQuery(tsFrom, tsTo time.Time) *queryConditions {
	return &queryConditions{
		whereCase: "WHERE session_time >= $1 AND session_time <= $2",
		whereArgs: []any{tsFrom, tsTo},
	}
}

func (s *KillsStorage) QueryWithLevel(q *queryConditions, level string) {
	s.lock.Lock()
	levelID, ok := s.cLevels.GetExistingIDNOLOCK(level)
	s.lock.Unlock()
	if !ok {
		return
	}
	q.whereArgs = append(q.whereArgs, levelID)
	q.whereCase += fmt.Sprintf(" AND level = $%d", len(q.whereArgs))
}

func (q *queryConditions) QueryWithKillerTeam(killerTeam int) {
	q.whereArgs = append(q.whereArgs, killerTeam)
	q.whereCase += fmt.Sprintf(" AND killer_team = $%d", len(q.whereArgs))
}

func (q *queryConditions) QueryWithKillTimeMin(killTimeMin time.Duration) {
	q.whereArgs = append(q.whereArgs, killTimeMin.Milliseconds())
	q.whereCase += fmt.Sprintf(" AND kill_time >= $%d", len(q.whereArgs))
}

func (q *queryConditions) QueryWithKillTimeMax(killTimeMax time.Duration) {
	q.whereArgs = append(q.whereArgs, killTimeMax.Milliseconds())
	q.whereCase += fmt.Sprintf(" AND kill_time <= $%d", len(q.whereArgs))
}

func (s *KillsStorage) QueryWithKillerVehicle(q *queryConditions, vehicle string) {
	s.lock.Lock()
	vehicleID, ok := s.cVehicles.GetExistingIDNOLOCK(vehicle)
	s.lock.Unlock()
	if !ok {
		return
	}
	q.whereArgs = append(q.whereArgs, vehicleID)
	q.whereCase += fmt.Sprintf(" AND killer_vehicle = $%d", len(q.whereArgs))
}

type KillTally struct {
	X, Z  int
	Score int
	Count int
}

func (s *KillsStorage) GetKillCountsByCoord(ctx context.Context, conds *queryConditions) ([]KillTally, error) {
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
` + conds.whereCase + `
GROUP BY (ROUND(p.x))::int, (ROUND(p.z))::int;`
	rows, err := s.db.Query(ctx, q, conds.whereArgs...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	// log.Info().Msg(q)
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (KillTally, error) {
		var ret KillTally
		row.Scan(&ret.X, &ret.Z, &ret.Score, &ret.Count)
		return ret, err
	})
}

func (s *KillsStorage) GetAmountsByLevel(ctx context.Context) (map[string]int, error) {
	rows, err := s.db.Query(ctx, `select level, count(*) from kills group by level order by 2 desc;`)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return map[string]int{}, nil
		}
	}
	var s1, s2 int
	ret := map[string]int{}
	s.lock.Lock()
	defer s.lock.Unlock()
	_, err = pgx.ForEachRow(rows, []any{&s1, &s2}, func() error {
		levelString, ok := s.cLevels.GetValueNOLOCK(s1)
		if ok {
			ret[levelString] = s2
		}
		return nil
	})
	return ret, err
}

// func (s *KillsStorage) GetKillCounts(from time.Time, hours uint) ([]int, error) {
// 	ret := make([]int, hours)
// 	for i := range hours {
// 		tableName := fmt.Sprintf("kills_y%dm%dd%dh%d", from.Year(), from.Month(), from.Day(), from.Hour())
// 		err := s.db.QueryRow(context.Background(), `select (case when c.reltuples < 0 then float8 '0'
//              when c.relpages = 0 then float8 '0'
//              else c.reltuples / c.relpages end
//      * (pg_catalog.pg_relation_size(c.oid) / pg_catalog.current_setting('block_size')::int))::bigint
// from pg_catalog.pg_class c
// where c.oid = 'public.`+tableName+`'::regclass;`).Scan(&ret[i])
// 		if err != nil {
// 			return nil, err
// 		}
// 	}
// 	return ret, nil
// }

func (s *KillsStorage) Close() {
	s.db.Close()
}

func LuxCarveToKills(carve *lux.LuxCarve) (ret []Kill, err error) {
	ret = []Kill{}
	if carve == nil {
		return
	}
	sessionID, err := strconv.ParseUint(carve.SessionID, 10, 64)
	if err != nil {
		return ret, fmt.Errorf("parsing session id number string %q: %w", carve.SessionID, err)
	}
	for _, kill := range carve.Events.Kills {
		if kill.OffendedUid == "0" {
			continue
		}
		if kill.OffenderUid == "0" {
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
		killer, ok := carve.Players[kill.OffenderUid]
		if !ok {
			return ret, fmt.Errorf("killer was not found (%q)", kill.OffenderUid)
		}
		killerTeam, err := strconv.ParseUint(killer.Team, 10, 64)
		if err != nil {
			return ret, fmt.Errorf("parsing killer team string %q: %w", killer.Team, err)
		}
		victim, ok := carve.Players[kill.OffendedUid]
		if !ok {
			return ret, fmt.Errorf("victim was not found (%q)", kill.OffendedUid)
		}
		victimTeam, err := strconv.ParseUint(victim.Team, 10, 64)
		if err != nil {
			return ret, fmt.Errorf("parsing victim team string %q: %w", victim.Team, err)
		}
		if killerTeam > 2 || victimTeam > 2 {
			return ret, fmt.Errorf("victim or killer team oob %d %d", killerTeam, victimTeam)
		}
		ret = append(ret, Kill{
			Session:       sessionID,
			SessionTime:   carve.StartTime,
			KillTime:      uint64(kill.Time),
			Level:         carve.Level,
			Mission:       carve.Mission,
			KillerID:      killerID,
			KillerTeam:    byte(killerTeam),
			KillerVehicle: strings.ToLower(kill.OffenderUnit),
			KillerPosX:    float64(kill.OffenderPos[0]) / 16,
			KillerPosZ:    float64(kill.OffenderPos[2]) / 16,
			Weapon:        kill.Weapon,
			VictimID:      victimID,
			VictimTeam:    byte(victimTeam),
			VictimVehicle: strings.ToLower(kill.OffendedUnit),
			VictimPosX:    float64(kill.OffendedPos[0]) / 16,
			VictimPosZ:    float64(kill.OffendedPos[2]) / 16,
		})
	}
	return
}
