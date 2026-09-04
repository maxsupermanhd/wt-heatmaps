package main

import (
	"fmt"
	"io"
	"main/frontend"
	"main/lib/wt"
	"net/http"
	"os"
	"path/filepath"
)

var (
	vehicleEconomyCatalog wt.VehicleEconomyCatalog
)

func frontendVehicle(id string, v *wt.WpcostVehicle) frontend.Vehicle {
	if v == nil {
		return frontend.Vehicle{
			ID: id,
		}
	}
	return frontend.Vehicle{
		ID:        id,
		Rank:      v.EconomicRankHistorical,
		IsPremium: v.IsPremium(),
		IsEvent:   v.IsEvent(),
	}
}

func wpcostLoad() error {
	cacheFilePath := "cache/wpcost.json"
	f, err := os.Open(cacheFilePath)
	if err == nil {
		tmp, err := wt.NewVehicleEconomyCatalog(f)
		f.Close()
		if err != nil {
			return err
		}
		vehicleEconomyCatalog = *tmp
		return nil
	}
	err = os.MkdirAll(filepath.Dir(cacheFilePath), 0766)
	if err != nil {
		return err
	}
	rsp, err := http.Get("https://raw.githubusercontent.com/gszabi99/War-Thunder-Datamine/refs/heads/master/char.vromfs.bin_u/config/wpcost.blkx")
	if err != nil {
		return err
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != 200 {
		return fmt.Errorf("Fetching wpcost code %s", rsp.Status)
	}
	// written to a temporary file first, a partial cache file makes every
	// following start fail on decode
	f, err = os.CreateTemp(filepath.Dir(cacheFilePath), "wpcost-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	r := io.TeeReader(rsp.Body, f)
	tmp, err := wt.NewVehicleEconomyCatalog(r)
	if err != nil {
		f.Close()
		return err
	}
	_, err = io.Copy(f, rsp.Body)
	if err != nil {
		f.Close()
		return err
	}
	err = f.Close()
	if err != nil {
		return err
	}
	err = os.Rename(f.Name(), cacheFilePath)
	if err != nil {
		return err
	}
	vehicleEconomyCatalog = *tmp
	return nil
}
