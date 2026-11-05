package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/draw"
)

func main() {
	server := `http://192.168.42.100:40124`
	level := `levels/avg_mozdok.bin`
	outDir := `out`
	must(os.MkdirAll(outDir, 0764))
	inMinimapRsp := noerr(http.Get(server + "/minimap/2048/" + level + ".png"))
	if inMinimapRsp.StatusCode != 200 {
		panic(inMinimapRsp.Status)
	}
	inMinimap := noerr(png.Decode(inMinimapRsp.Body)).(*image.NRGBA)
	vals := url.Values{}
	vals.Add(`level`, level)
	vals.Add(`scoreIntensity`, `64`)
	vals.Add(`countIntensity`, `64`)
	outBuf := &bytes.Buffer{}
	const offset = 40
	for i := range 600 / 5 {
		log.Println(i)
		vals.Set(`killTimeMin`, strconv.Itoa(offset+i*5))
		vals.Set(`killTimeMax`, strconv.Itoa(offset+i*5+30))
		u := `http://192.168.42.100:40124/render/heat?` + vals.Encode()
		log.Println("GET " + u)
		rsp := noerr(http.Get(u))
		if rsp.StatusCode != 200 {
			panic(rsp.Status)
		}
		log.Println("draw")
		inHeat := noerr(png.Decode(rsp.Body)).(*image.NRGBA)
		out := image.NewRGBA(inHeat.Rect)
		draw.NearestNeighbor.Scale(out, out.Rect, inMinimap, inMinimap.Rect, draw.Over, nil)
		draw.Draw(out, out.Rect, image.NewUniform(color.RGBA{A: 0xaa}), image.Point{}, draw.Over)
		draw.Draw(out, out.Rect, inHeat, image.Point{}, draw.Over)
		log.Println("copy")
		outBuf.Reset()
		must(png.Encode(outBuf, out))
		must(os.WriteFile(filepath.Join(outDir, fmt.Sprintf("out-%04d.png", i)), outBuf.Bytes(), 0644))
	}
	log.Println("closing")
	log.Println("done")
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func noerr[T any](ret T, err error) T {
	if err != nil {
		panic(err)
	}
	return ret
}
