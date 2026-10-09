package killstorage

import (
	"context"
	"errors"
	"fmt"
	"main/lib/lux/luxproto/luxprotogen"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/duckdb/duckdb-go/v2"
)

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

func LuxCarveToKills(carve *luxprotogen.Replay) (ret []Kill, err error) {
	ret = []Kill{}
	if carve == nil {
		return
	}
	if carve.Light == nil {
		return ret, errors.New("carve.Light is nill")
	}
	if len(carve.SpawnSide) != 2 {
		return nil, fmt.Errorf("SpawnSize has incorrect length: %d", len(carve.SpawnSide))
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
		killerTeam, err := luxCarveToKillsGetPlayerTeam(carve, kill.OffenderUid)
		if err != nil {
			return ret, err
		}
		victimTeam, err := luxCarveToKillsGetPlayerTeam(carve, kill.OffendedUid)
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

func luxCarveToKillsGetPlayerTeam(carve *luxprotogen.Replay, uid string) (int, error) {
	for _, p := range carve.Light.Players {
		if p == nil {
			continue
		}
		if p.Uid == uid {
			if p.Team != 1 && p.Team != 2 {
				return 0, fmt.Errorf("player %q has weird team: %d", uid, p.Team)
			}
			return int(carve.SpawnSide[p.Team-1]), nil
		}
	}
	return 0, fmt.Errorf("player was not found (%q)", uid)
}
