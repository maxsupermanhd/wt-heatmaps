package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/maxsupermanhd/flexcorallib/fclcache"
	"golang.org/x/image/draw"
)

var (
	cachedTankmaps    *fclcache.FetchFileCache
	tankmapHTTPClient = &http.Client{Timeout: 30 * time.Second}
	// the level name goes into the fetch URL and the cache file name
	tankmapLevelRegexp = regexp.MustCompile(`^levels/[A-Za-z0-9_-]+\.bin$`)
)

var tankmapNotFoundErr = errors.New("tankmap not found")

func tankmapFetchB64LEV(kb64 string) ([]byte, error) {
	kb, err := base64.StdEncoding.DecodeString(kb64)
	if err != nil {
		return nil, err
	}
	k := string(kb)
	if !tankmapLevelRegexp.MatchString(k) {
		return nil, fmt.Errorf("level name %q is not valid", k)
	}
	k = strings.TrimSuffix(k, ".bin")
	k = strings.TrimPrefix(k, "levels/")
	fetchUrl := cfg.GetDString("https://raw.githubusercontent.com/LivingTheDagor/WtMiniMapPictures/refs/heads/main/2048/", "urls", "tankmapsPrefix") + k + "_tankmap.png"
	resp, err := tankmapHTTPClient.Get(fetchUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, tankmapNotFoundErr
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s while downloading %q", resp.Status, fetchUrl)
	}
	return io.ReadAll(resp.Body)
}

func tankmapFromCache(id string) ([]byte, error) {
	return cachedTankmaps.Get(base64.StdEncoding.EncodeToString([]byte(id)))
}

var (
	cachedSmallTankmaps     = map[string][]byte{}
	cachedSmallTankmapsLock sync.Mutex
)

func serveCachedMinimaps(w http.ResponseWriter, r *http.Request) {
	resizeTo := 0
	switch r.PathValue("size") {
	case "128":
		resizeTo = 128
	case "2048":
		resizeTo = 2048
	default:
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	reqLevel := strings.TrimSuffix(r.PathValue("k"), ".png")
	if !tankmapLevelRegexp.MatchString(reqLevel) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	k := base64.StdEncoding.EncodeToString([]byte(reqLevel))
	if resizeTo == 128 {
		cachedSmallTankmapsLock.Lock()
		cachedSmall, ok := cachedSmallTankmaps[k]
		cachedSmallTankmapsLock.Unlock()
		if ok {
			w.WriteHeader(http.StatusOK)
			w.Write(cachedSmall)
			return
		}
	}
	ret, err := cachedTankmaps.Get(k)
	if err != nil {
		if errors.Is(err, tankmapNotFoundErr) {
			w.WriteHeader(http.StatusNoContent)
			w.Write([]byte(err.Error()))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	if resizeTo == 2048 {
		w.Header().Add("Cache-Control", "public, max-age=1814400")
		w.WriteHeader(http.StatusOK)
		w.Write(ret)
		return
	}
	if resizeTo != 2048 {
		im, err := png.Decode(bytes.NewBuffer(ret))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		out := image.NewRGBA(image.Rect(0, 0, resizeTo, resizeTo))
		draw.ApproxBiLinear.Scale(out, out.Rect, im, im.Bounds(), draw.Over, nil)
		w.Header().Add("Cache-Control", "public, max-age=1814400")
		w.WriteHeader(http.StatusOK)
		buf := &bytes.Buffer{}
		png.Encode(io.MultiWriter(w, buf), out)
		cachedSmallTankmapsLock.Lock()
		cachedSmallTankmaps[k] = buf.Bytes()
		cachedSmallTankmapsLock.Unlock()
	}

}
