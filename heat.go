package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"iter"
	"main/lib/killstorage-duckdb"
	"main/lib/levelcoords"
	"math"
	"net/http"
	"net/url"
	"os"
	"slices"
	"time"

	"github.com/fogleman/gg"
	"github.com/rs/zerolog/log"
)

func serveHeat(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	level := q.Get("level")
	if level == "" {
		w.WriteHeader(204)
		return
	}

	levelOffsets, err := getLevelOffsets(level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	var out []byte

	perf := time.Now()
	if isDefaultQuery(q) {
		err = cachedHeat.Access(level, func(a *cachedHeatAggregate) (*cachedHeatAggregate, error) {
			if a == nil {
				perf := time.Now()
				a, err = cachedHeatInitAggregate(level)
				log.Info().Msgf("heat perf cache miss took %s", time.Since(perf).String())
			} else {
				log.Info().Msg("heat perf cache hit")
			}
			out = renderTally(levelOffsets, a.Loop(), q)
			return a, err
		})
	} else {
		kq := buildKillQuery(q, level)
		if kq == nil {
			w.WriteHeader(204)
			return
		}
		// log.Info().Msg(kq.Dump())
		a, err := ks.GetKillCountsByCoord(r.Context(), kq)
		if err != nil {
			log.Err(err).Msg("get kills")
			w.WriteHeader(500)
			w.Write([]byte(err.Error()))
			return
		}
		out = renderTally(levelOffsets, func(yield func(*killstorage.KillTally) bool) {
			for i := range a {
				if !yield(&a[i]) {
					return
				}
			}
		}, q)
	}
	log.Info().Msgf("heat perf total %s", time.Since(perf).String())
	if err != nil {
		return
	}

	w.Write(out)
}

func renderTally(levelOffsets levelcoords.LevelCoords, tally iter.Seq[*killstorage.KillTally], q url.Values) []byte {
	perf := time.Now()

	areaW := float32(math.Abs(float64(levelOffsets.TankMap0[0] - levelOffsets.TankMap1[0])))
	areaH := float32(math.Abs(float64(levelOffsets.TankMap0[1] - levelOffsets.TankMap1[1])))
	areaOffsetX := levelOffsets.TankMap0[0]
	areaOffsetZ := levelOffsets.TankMap0[1]
	outputW := int(areaW)
	outputH := int(areaH)
	out := image.NewRGBA(image.Rect(0, 0, outputW, outputH))

	scoreIntensity := int64(urlValueIntOr(q, "scoreIntensity", 0))
	countIntensity := int64(urlValueIntOr(q, "countIntensity", 0))

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := uint64(0)
	blockKills := make([]int64, outputH*outputW)
	blockDeaths := make([]int64, outputH*outputW)

	for v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))
		if tx < 0 || tx >= float64(outputW) || tz < 0 || tz >= float64(outputH) {
			continue
		}
		blockKills[int(tx+tz*float64(outputW))] = int64(v.Kills)
		blockDeaths[int(tx+tz*float64(outputW))] = int64(v.Deaths)
		totalN += uint64(v.Kills + v.Deaths)
	}

	if scoreIntensity == 0 || countIntensity == 0 {
		return nil
		hr := []float64{}
		hd := []float64{}
		hc := []float64{}
		for v := range tally {
			vd := float64(v.Deaths)
			vk := float64(v.Kills)
			if vd == 0 {
				vd = 1
			}
			hr = append(hr, vk/vd)
			hd = append(hd, vk-vd)
			hc = append(hc, vk+vd)
		}
		slices.Sort(hr)
		slices.Sort(hd)
		slices.Sort(hc)
		hr = slices.Compact(hr)
		hd = slices.Compact(hd)
		hc = slices.Compact(hc)
		log.Info().Float64("last", hr[len(hr)-1]).Float64("first", hr[0]).Int("len", len(hr)).Msg("hr")
		log.Info().Float64("last", hc[len(hc)-1]).Float64("first", hc[0]).Int("len", len(hc)).Msg("hc")
		log.Info().Float64("last", hd[len(hd)-1]).Float64("first", hd[0]).Int("len", len(hd)).Msg("hd")
		d := make([]float64, len(hd)-1)
		for i := range d {
			d[i] = hd[i+1] - hd[i]
		}
		saveDat := func(d []float64, fn string) {
			buf := &bytes.Buffer{}
			for i, v := range d {
				fmt.Fprintf(buf, "%v\t%v\n", i, v)
			}
			must(os.WriteFile(fn, buf.Bytes(), 0644))
		}
		saveDat(d, "out.dat")
		return nil
	} else {
		for i, vk := range blockKills {
			o := i * 4
			vd := blockDeaths[i]
			vdelta := vk - vd
			out.Pix[o+0] = uint8(max(min(vdelta*scoreIntensity, 255), 0))
			out.Pix[o+1] = uint8(max(min(vdelta*vdelta, 255), 0) / 2)
			out.Pix[o+2] = uint8(max(min(-vdelta*scoreIntensity, 255), 0))
			out.Pix[o+3] = uint8(min((vk+vd)*countIntensity, 255))
		}
	}

	ret := ggStrings(gg.NewContextForImage(out), 20, 20, color.RGBA{R: 255, G: 255, B: 255, A: 128}, color.RGBA{R: 0, G: 0, B: 0, A: 255},
		"Rendered by FlexCoral at thunder.nanachi.party",
		"Data provided by Lux",
		fmt.Sprintf("Data points: %d", totalN),
		time.Now().Round(0).String(),
	)

	log.Info().Msgf("heat perf render took %s", time.Since(perf).String())

	buf := &bytes.Buffer{}
	ret.EncodePNG(buf)
	return buf.Bytes()
}
