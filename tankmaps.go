package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"main/lib/caches"
	"net/http"
	"strings"

	"golang.org/x/image/draw"
)

var cachedTankmaps *caches.FetchFileCache

func tankmapFetchB64LEV(kb64 string) ([]byte, error) {
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
		return nil, fmt.Errorf("%s while downloading %q", resp.Status, fetchUrl)
	}
	ret, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return ret, err
}

func tankmapFromCache(id string) (*image.RGBA, error) {
	ret, err := cachedTankmaps.Get(base64.StdEncoding.EncodeToString([]byte(id)))
	if err != nil {
		return nil, err
	}
	im, err := png.Decode(bytes.NewReader(ret))
	imRGBA, ok := im.(*image.RGBA)
	if !ok {
		imNRGBA, ok := im.(*image.NRGBA)
		if !ok {
			return nil, fmt.Errorf("not rgba or nrgba from tankmap cache: %T", im)
		}
		imRGBA = &image.RGBA{
			Pix:    imNRGBA.Pix,
			Stride: imNRGBA.Stride,
			Rect:   imNRGBA.Rect,
		}
	}
	return imRGBA, nil
}

func serveCachedMinimaps(w http.ResponseWriter, r *http.Request) {
	resizeTo := 0
	switch r.PathValue("size") {
	case "128":
		resizeTo = 128
	case "2048":
		resizeTo = 2048
	default:
		w.WriteHeader(400)
		return
	}
	ret, err := cachedTankmaps.Get(base64.StdEncoding.EncodeToString([]byte(r.PathValue("k"))))
	if err != nil {
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}
	if resizeTo == 2048 {
		w.Header().Add("Cache-Control", "max-age=1814400")
		w.WriteHeader(200)
		w.Write(ret)
		return
	}
	if resizeTo != 2048 {
		im, err := png.Decode(bytes.NewBuffer(ret))
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		out := image.NewRGBA(image.Rect(0, 0, resizeTo, resizeTo))
		draw.ApproxBiLinear.Scale(out, out.Rect, im, im.Bounds(), draw.Over, nil)
		w.Header().Add("Cache-Control", "max-age=1814400")
		w.WriteHeader(200)
		png.Encode(w, out)
	}

}
