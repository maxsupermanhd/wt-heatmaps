package wt

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

type WpcostVehicle struct {
	EconomicRankHistorical int    `json:"economicRankHistorical"`
	CostGold               int    `json:"costGold"`
	Event                  string `json:"event"`
	Country                string `json:"country"`
	UnitClass              string `json:"unitClass"`
}

func (v WpcostVehicle) IsPremium() bool {
	return v.CostGold > 0
}

func (v WpcostVehicle) IsEvent() bool {
	return v.Event != ""
}

func unmarshalWpcostBytes(wpcostJsonReader io.Reader) (map[string]*WpcostVehicle, int, error) {
	dec := json.NewDecoder(wpcostJsonReader)
	ret := make(map[string]*WpcostVehicle)
	var rankMax int

	_, err := dec.Token()
	if err != nil {
		return nil, 0, err
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, 0, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, 0, fmt.Errorf("key %v not string", tok)
		}

		if key == "economicRankMax" {
			if err := dec.Decode(&rankMax); err != nil {
				return nil, 0, err
			}
			continue
		}

		var value WpcostVehicle
		if err := dec.Decode(&value); err != nil {
			return nil, 0, fmt.Errorf("key %q: %w", key, err)
		}
		ret[key] = &value
	}
	return ret, rankMax, nil
}

type VehicleEconomyCatalog struct {
	Vehicles       map[string]*WpcostVehicle
	byBattleRating [][]string
	rankMax        int
}

func NewVehicleEconomyCatalog(wpcostJsonReader io.Reader) (*VehicleEconomyCatalog, error) {
	wpcost, rankMax, err := unmarshalWpcostBytes(wpcostJsonReader)
	if err != nil {
		return nil, err
	}
	var errs []error
	for k, v := range wpcost {
		if strings.IndexAny(k, "QWERTYUIOPASDFGHJKLZXCVBNM") != -1 {
			_, ok := wpcost[strings.ToLower(k)]
			if ok {
				errs = append(errs, fmt.Errorf("case fold collision %q %q", k, strings.ToLower(k)))
			}
			wpcost[strings.ToLower(k)] = v
		}
	}
	byBattleRating := make([][]string, rankMax+1)
	for k, v := range wpcost {
		if v.EconomicRankHistorical < 0 || v.EconomicRankHistorical >= len(byBattleRating) {
			continue
		}
		byBattleRating[v.EconomicRankHistorical] = append(byBattleRating[v.EconomicRankHistorical], k)
	}
	for i := range byBattleRating {
		if byBattleRating[i] == nil {
			byBattleRating[i] = []string{}
		}
		slices.Sort(byBattleRating[i])
	}
	ret := &VehicleEconomyCatalog{
		Vehicles:       wpcost,
		byBattleRating: byBattleRating,
		rankMax:        rankMax,
	}
	return ret, errors.Join(errs...)
}

func (brg *VehicleEconomyCatalog) GetAllByRank(rank int) []string {
	if rank < 0 {
		return nil
	}
	if rank >= len(brg.byBattleRating) {
		return nil
	}
	return brg.byBattleRating[rank]
}

func (brg *VehicleEconomyCatalog) GetRankMax() int {
	return brg.rankMax
}

func (brg *VehicleEconomyCatalog) GetAllInRange(brminp, brmaxp *int) []string {
	if brminp == nil && brmaxp == nil {
		return nil
	}
	brmin := 0
	brmax := brg.rankMax
	if brminp != nil && *brminp <= brg.rankMax && *brminp >= 0 {
		brmin = *brminp
	}
	if brmaxp != nil && *brmaxp <= brg.rankMax && *brmaxp >= 0 {
		brmax = *brmaxp
	}
	if brmin > brmax {
		brmin, brmax = brmax, brmin
	}
	ret := []string{}
	for i := brmin; i <= brmax; i++ {
		ret = append(ret, brg.GetAllByRank(i)...)
	}
	return ret
}
