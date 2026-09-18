package levelcoords

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
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

// TankMapAreaToWorld turns a box given as fractions of the tank map view into
// world meters. It inverts the transform that the heatmap is painted with, so
// the z axis grows upwards while the view grows downwards. A fraction outside
// 0 to 1 is clamped to the edge of the map.
//
// A fraction of the view names the edge between two heatmap pixels, but a
// heatmap pixel holds the kills that round to its own whole meter. The edge
// therefore sits half a meter below the center of the pixel on x, and half a
// meter above it on z, which the flipped axis reverses. The half meter here
// keeps the box the browser drew and the box the query counts the same.
func (c LevelCoords) TankMapAreaToWorld(box [4]float64) (x0, z0, x1, z1 float64) {
	for i, v := range box {
		box[i] = min(max(v, 0), 1)
	}
	originX := float64(c.TankMap0[0]) - 0.5
	originZ := float64(c.TankMap0[1]) + 0.5
	areaW := math.Abs(float64(c.TankMap0[0] - c.TankMap1[0]))
	areaH := math.Abs(float64(c.TankMap0[1] - c.TankMap1[1]))
	return originX + box[0]*areaW,
		originZ + (1-box[1])*areaH,
		originX + box[2]*areaW,
		originZ + (1-box[3])*areaH
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
	if err != nil {
		return
	}

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
