package wt

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
)

type WpcostVehicle struct {
	EconomicRankHistorical int    `json:"economicRankHistorical"`
	CostGold               int    `json:"costGold"`
	Event                  string `json:"event"`
}

func (v WpcostVehicle) IsPremium() bool {
	return v.CostGold > 0
}

func (v WpcostVehicle) IsEvent() bool {
	return v.Event != ""
}

func (val *WpcostVehicle) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if val == nil {
		return nil
	}
	switch b[0] {
	case '{':
		type tmp struct {
			EconomicRankHistorical int    `json:"economicRankHistorical"`
			CostGold               int    `json:"costGold"`
			Event                  string `json:"event"`
		}
		var tmpval tmp
		err := json.Unmarshal(b, &tmpval)
		if err != nil {
			return err
		}
		val.EconomicRankHistorical = tmpval.EconomicRankHistorical
		val.CostGold = tmpval.CostGold
		val.Event = tmpval.Event
	default:
		var tmpval int
		err := json.Unmarshal(b, &tmpval)
		if err != nil {
			return err
		}
		val.EconomicRankHistorical = tmpval
	}
	return nil
}

type VehicleEconomyCatalog struct {
	Vehicles       map[string]*WpcostVehicle
	byBattleRating [][]string
	rankMax        int
}

func NewVehicleEconomyCatalog(wpcostJsonReader io.Reader) (*VehicleEconomyCatalog, error) {
	var wpcost map[string]*WpcostVehicle
	err := json.NewDecoder(wpcostJsonReader).Decode(&wpcost)
	if err != nil {
		return nil, err
	}
	rankMax, ok := wpcost["economicRankMax"]
	if !ok {
		return nil, errors.New("economicRankMax not found in json")
	}
	delete(wpcost, "economicRankMax")
	byBattleRating := make([][]string, rankMax.EconomicRankHistorical+1)
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
		rankMax:        rankMax.EconomicRankHistorical,
	}
	return ret, nil
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
