package killstorage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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
