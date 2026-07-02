package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
)

func fetchTankmap(kb64 string) ([]byte, error) {
	kb, err := base64.StdEncoding.DecodeString(kb64)
	if err != nil {
		return nil, err
	}
	k := string(kb)
	k = strings.TrimSuffix(k, ".bin")
	k = strings.TrimPrefix(k, "levels/")
	fetchUrl := "https://raw.githubusercontent.com/LivingTheDagor/WtMiniMapPictures/refs/heads/main/2048/" + k + "_tankmap.png"
	resp, err := http.Get(fetchUrl)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("status " + resp.Status)
	}
	ret, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return ret, err
}

func serveCachedMinimaps(w http.ResponseWriter, r *http.Request) {
	ret, err := tankmapsCache.Get(base64.StdEncoding.EncodeToString([]byte(r.PathValue("k"))))
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	w.WriteHeader(200)
	w.Write(ret)
}
