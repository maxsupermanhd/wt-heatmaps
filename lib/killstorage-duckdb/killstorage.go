package killstorage

import (
	"context"
	"database/sql"
	"errors"

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
	db.SetMaxOpenConns(2)
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

type MemoryStatValue struct {
	Memory  int
	Storage int
}

func (s *KillsStorage) DebugMemory(ctx context.Context) (map[string]MemoryStatValue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT * FROM duckdb_memory()`)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return map[string]MemoryStatValue{}, nil
		}
		return nil, err
	}
	var s1 string
	var s2, s3 int
	ret := map[string]MemoryStatValue{}
	err = ForEachRow(rows, []any{&s1, &s2, &s3}, func() error {
		ret[s1] = MemoryStatValue{
			Memory:  s2,
			Storage: s3,
		}
		return nil
	})
	return ret, err
}

func (s *KillsStorage) Close() {
	s.db.Close()
}
