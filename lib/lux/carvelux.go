package lux

import "encoding/json"

type LuxCarve struct {
	SessionID string  `msgpack:"_id"`
	StartTime uint64  `msgpack:"start_ts"`
	Duration  float32 `msgpack:"duration"`

	Level      string `msgpack:"level_path"`
	Mission    string `msgpack:"mission_path"`
	Difficulty string `msgpack:"difficulty"`

	Players  map[string]LuxCarvePlayer `msgpack:"players"`
	Entities []LuxCarveEntities        `msgpack:"entities"`
	Events   LuxCarveEvents            `msgpack:"events"`
}

type LuxCarveEntities struct {
	UserID   string  `msgpack:"uid"`
	UnitName string  `msgpack:"unit"`
	T        []int64 `msgpack:"t" json:",omitempty"`
	X        []int64 `msgpack:"x" json:",omitempty"`
	Y        []int64 `msgpack:"y" json:",omitempty"`
	Z        []int64 `msgpack:"z" json:",omitempty"`
}

type LuxCarveEvents struct {
	Kills []LuxCarveKill `msgpack:"kills"`
}

type LuxCarveKill struct {
	Time         uint32  `msgpack:"time"`
	OffenderUid  string  `msgpack:"offender_uid"`
	OffenderUnit string  `msgpack:"offender_unit"`
	OffenderPos  []int64 `msgpack:"offender_pos"`
	OffendedUid  string  `msgpack:"offended_uid"`
	OffendedUnit string  `msgpack:"offended_unit"`
	OffendedPos  []int64 `msgpack:"offended_pos"`
	Weapon       string  `msgpack:"used_weapon"`
}

type LuxCarvePlayer struct {
	Name string `msgpack:"name"`
	Team string `msgpack:"team"`
	Slot uint16 `msgpack:"pid"`
}

func (e LuxCarveEntities) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"UserID":   e.UserID,
		"UnitName": e.UnitName,
		"Path": map[string]any{
			"Len":  len(e.T),
			"Tmin": castNumber[int64](e.T[0]),
		},
	})
}

func castNumber[T int64 | uint64](val any) T {
	switch v := val.(type) {
	case uint8:
		return T(v)
	case uint16:
		return T(v)
	case uint32:
		return T(v)
	case uint64:
		return T(v)
	case int8:
		return T(v)
	case int16:
		return T(v)
	case int32:
		return T(v)
	case int64:
		return T(v)
	}
	return 0
}
