package levelcoords

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type LevelCoords struct {
	Map0     [2]float32 `json:"mapCoord0"`
	Map1     [2]float32 `json:"mapCoord1"`
	TankMap0 [2]float32 `json:"tankMapCoord0"`
	TankMap1 [2]float32 `json:"tankMapCoord1"`
}

var (
	cachedCoordsLock sync.Mutex
	cachedCoords     = map[string]LevelCoords{}
)

func GetLevelCoordsCached(cacheFile, level string) (ret LevelCoords, err error) {
	cachedCoordsLock.Lock()
	ret, ok := cachedCoords[level]
	cachedCoordsLock.Unlock()
	if ok {
		return ret, nil
	}

	fileBytes, err := os.ReadFile(cacheFile)
	if err == nil {
		var fileData map[string]LevelCoords
		err = json.Unmarshal(fileBytes, &fileData)
		if err != nil {
			return ret, err
		}
		cachedCoordsLock.Lock()
		maps.Insert(cachedCoords, maps.All(fileData))
		ret, ok = cachedCoords[level]
		cachedCoordsLock.Unlock()
		if ok {
			return ret, nil
		}
	} else {
		if !errors.Is(err, os.ErrNotExist) {
			return ret, err
		}
	}

	fetchUrl := "https://raw.githubusercontent.com/gszabi99/War-Thunder-Datamine/refs/heads/master/aces.vromfs.bin_u/" + strings.TrimSuffix(level, ".bin") + ".blkx"
	resp, err := http.Get(fetchUrl)
	if err != nil {
		return ret, err
	}
	if resp.StatusCode != 200 {
		return ret, fmt.Errorf("request to %q returned %d", fetchUrl, resp.StatusCode)
	}
	err = json.NewDecoder(resp.Body).Decode(&ret)
	resp.Body.Close()

	cachedCoordsLock.Lock()
	cachedCoords[level] = ret
	fileBytes, err = json.Marshal(cachedCoords)
	cachedCoordsLock.Unlock()
	if err != nil {
		return
	}
	err = os.MkdirAll(filepath.Dir(cacheFile), 0755)
	if err != nil {
		return
	}
	err = os.WriteFile(cacheFile, fileBytes, 0644)
	return
}
