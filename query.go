package main

import (
	"main/lib/killstorage-duckdb"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
)

func isDefaultQuery(vals url.Values) bool {
	d := map[string]string{
		"team":                  "",
		"killTimeMin":           "",
		"killTimeMax":           "",
		"killerBattleRatingMin": "",
		"killerBattleRatingMax": "",
		"killerVehicleClass":    "",
		"killerVehicleCountry":  "",
		"victimBattleRatingMin": "",
		"victimBattleRatingMax": "",
		"victimVehicleClass":    "",
		"victimVehicleCountry":  "",
	}
	for k, v := range vals {
		dv, ok := d[k]
		if !ok {
			continue
		}
		if len(v) != 1 {
			continue
		}
		if dv != v[0] {
			return false
		}
	}
	return true
}

func buildKillQuery(q url.Values, level string) *killstorage.QueryConditions {
	kq := &killstorage.QueryConditions{}
	if !ks.QueryWithLevel(kq, level) {
		return nil
	}
	if val := urlValueInt(q, "team"); val != nil {
		kq.QueryWithTeam(*val)
	}
	if val := urlValueInt(q, "killTimeMin"); val != nil {
		kq.QueryWithKillTimeMin(time.Duration(*val) * time.Second)
	}
	if val := urlValueInt(q, "killTimeMax"); val != nil {
		kq.QueryWithKillTimeMax(time.Duration(*val) * time.Second)
	}
	killerVehicles := vehicleEconomyCatalog.GetAllInRange(urlValueInt(q, "killerBattleRatingMin"), urlValueInt(q, "killerBattleRatingMax"))
	if val := q.Get("killerVehicleClass"); val != "" {
		log.Info().Str("killerVehicleClass", val).Msg("killerVehicleClass")
		if len(killerVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.UnitClass == val {
					killerVehicles = append(killerVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range killerVehicles {
				if vehicleEconomyCatalog.Vehicles[v].UnitClass == val {
					filtered = append(filtered, v)
				}
			}
			killerVehicles = filtered
		}
	}
	if val := q.Get("killerVehicleCountry"); val != "" {
		if len(killerVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.Country == val {
					killerVehicles = append(killerVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range killerVehicles {
				if vehicleEconomyCatalog.Vehicles[v].Country == val {
					filtered = append(filtered, v)
				}
			}
			killerVehicles = filtered
		}
	}
	if len(killerVehicles) > 0 {
		for i := range killerVehicles {
			killerVehicles[i] = "tankmodels/" + killerVehicles[i]
		}
		ks.QueryWithKillerVehicles(kq, killerVehicles)
	}
	victimVehicles := vehicleEconomyCatalog.GetAllInRange(urlValueInt(q, "victimBattleRatingMin"), urlValueInt(q, "victimBattleRatingMax"))
	if val := q.Get("victimVehicleClass"); val != "" {
		if len(victimVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.UnitClass == val {
					victimVehicles = append(victimVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range victimVehicles {
				if vehicleEconomyCatalog.Vehicles[v].UnitClass == val {
					filtered = append(filtered, v)
				}
			}
			victimVehicles = filtered
		}
	}
	if val := q.Get("victimVehicleCountry"); val != "" {
		if len(victimVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.Country == val {
					victimVehicles = append(victimVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range victimVehicles {
				if vehicleEconomyCatalog.Vehicles[v].Country == val {
					filtered = append(filtered, v)
				}
			}
			victimVehicles = filtered
		}
	}
	if len(victimVehicles) > 0 {
		for i := range victimVehicles {
			victimVehicles[i] = "tankmodels/" + victimVehicles[i]
		}
		ks.QueryWithVictimVehicles(kq, victimVehicles)
	}
	return kq
}
