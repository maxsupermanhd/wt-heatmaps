package main

import (
	"fmt"
	"image"
	"image/color"
	"main/frontend"
	"main/lib/killstorage"
	"main/lib/levelcoords"
	"maps"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/fogleman/gg"
	"github.com/rs/zerolog/log"
)

func makeHTTPServeMux() http.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", httpLog(handle404))
	mux.HandleFunc("GET /static/", httpLog(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP))
	mux.HandleFunc("GET /{$}", httpLog(compRender(serveIndex)))

	mux.HandleFunc("GET /minimap/{size}/{k...}", serveCachedMinimaps)
	mux.HandleFunc("GET /heat", httpLog(serveHeat))
	// mux.HandleFunc("GET /frontend/mapUpdate", httpLog(compRender(serveFrontendMapUpdate)))
	// mux.HandleFunc("GET /ws/frontend", httpLog(websocket.Server{
	// 	Handler: handleWsFrontend,
	// }.ServeHTTP))

	mux.HandleFunc("GET /missions...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /clans...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /players...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /sessions...", httpLog(servePermaRedirect("/")))

	return mux.ServeHTTP
}

func serveIndex(w http.ResponseWriter, r *http.Request) templ.Component {
	levels, err := ks.GetAmountsByLevel(r.Context())
	if err != nil {
		log.Err(err).Msg("get amounts by level")
		levels = map[string]int{}
	}
	vehicles := slices.Collect(maps.Values(ks.GetDictVehicles()))
	slices.Sort(vehicles)
	return frontend.Page(frontend.Index(levels, vehicles))
}

// func serveFrontendMapUpdate(w http.ResponseWriter, r *http.Request) templ.Component {
// 	// perf := time.Now()
// 	q := r.URL.Query()
// 	level := q.Get("level")
// 	if !slices.Contains(slices.Collect(maps.Values(ks.GetDictLevels())), level) {
// 		return nil
// 	}
// 	// levelOffsets, err := levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
// 	// if err != nil {
// 	// 	return frontend.MapUpdateError(err)
// 	// }
// 	return frontend.MapUpdate(frontend.MapUpdateParams{
// 		Level:      level,
// 		HeatParams: q,
// 		// Offsets: levelOffsets,
// 		// Msg:     fmt.Sprintf("Took: %s", time.Since(perf).String()),
// 	})
// }

func serveHeat(w http.ResponseWriter, r *http.Request) {
	perf := time.Now()
	q := r.URL.Query()
	level := q.Get("level")
	if level == "" {
		w.WriteHeader(204)
		return
	}

	levelOffsets, err := levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	kq := &killstorage.QueryConditions{} //(time.Now().Add(-7*24*time.Hour), time.Now())
	ks.QueryWithLevel(kq, level)
	killerTeamStr := q.Get("killerTeam")
	if killerTeamStr != "" {
		killerTeam, err := strconv.Atoi(killerTeamStr)
		if err == nil {
			kq.QueryWithKillerTeam(killerTeam)
		}
	}
	killTimeMinStr := q.Get("killTimeMin")
	if killTimeMinStr != "" {
		killTimeMin, err := strconv.Atoi(killTimeMinStr)
		if err == nil {
			kq.QueryWithKillTimeMin(time.Duration(killTimeMin) * time.Second)
		}
	}
	killTimeMaxStr := q.Get("killTimeMax")
	if killTimeMaxStr != "" {
		killTimeMax, err := strconv.Atoi(killTimeMaxStr)
		if err == nil {
			kq.QueryWithKillTimeMax(time.Duration(killTimeMax) * time.Second)
		}
	}
	killerVehicle := q.Get("killerVehicle")
	if killerVehicle != "" {
		ks.QueryWithKillerVehicle(kq, killerVehicle)
	}
	tally, err := ks.GetKillCountsByCoord(r.Context(), kq)
	if err != nil {
		log.Err(err).Msg("get kills")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	areaW := float32(math.Abs(float64(levelOffsets.TankMap0[0] - levelOffsets.TankMap1[0])))
	areaH := float32(math.Abs(float64(levelOffsets.TankMap0[1] - levelOffsets.TankMap1[1])))
	areaOffsetX := levelOffsets.TankMap0[0]
	areaOffsetZ := levelOffsets.TankMap0[1]
	outputW := int(areaW)
	outputH := int(areaH)
	out := image.NewRGBA(image.Rect(0, 0, outputW, outputH))

	scoreIntensityStr := q.Get("scoreIntensity")
	scoreIntensity, err := strconv.Atoi(scoreIntensityStr)
	if err != nil {
		scoreIntensity = 32
	}
	countIntensityStr := q.Get("countIntensity")
	countIntensity, err := strconv.Atoi(countIntensityStr)
	if err != nil {
		countIntensity = 32
	}

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := 0
	for _, v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))
		out.SetRGBA(int(tx), int(tz), color.RGBA{
			R: uint8(max(min(v.Score*scoreIntensity, 255), 0)),
			G: 0,
			B: uint8(max(min(-v.Score*scoreIntensity, 255), 0)),
			A: uint8(min(v.Count*countIntensity, 255)),
		})
		totalN += v.Count
	}
	ggStrings(gg.NewContextForImage(out), 20, 20, color.RGBA{R: 255, G: 255, B: 255, A: 128}, color.RGBA{R: 0, G: 0, B: 0, A: 255},
		"Rendered by FlexCoral at thunder.nanachi.party",
		"Data provided by Lux",
		fmt.Sprintf("Data points: %d", totalN),
		time.Now().Round(0).String(),
	).EncodePNG(w)
	log.Info().Dur("perf", time.Since(perf)).Int("nPix", len(tally)).Int("nDp", totalN).Msg("heat")
}

func compRender(f func(w http.ResponseWriter, r *http.Request) templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := f(w, r)
		if c != nil {
			c.Render(r.Context(), w)
		}
	}
}

func ggStrings(ctx *gg.Context, ox, oy float64, colBg, colFg color.RGBA, vals ...string) *gg.Context {
	for _, v := range vals {
		sw, sh := ctx.MeasureString(v)
		ctx.SetRGBA(float64(colBg.R)/255, float64(colBg.G)/255, float64(colBg.B)/255, float64(colBg.A)/255)
		ctx.DrawRectangle(ox-1, oy+3, sw+1, sh)
		ctx.Fill()
		ctx.SetRGBA(float64(colFg.R)/255, float64(colFg.G)/255, float64(colFg.B)/255, float64(colFg.A)/255)
		oy += sh
		ctx.DrawString(v, ox, oy)
		oy += 2
	}
	return ctx
}

func servePermaRedirect(location string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", location)
		w.WriteHeader(http.StatusMovedPermanently)
	}
}

// func handleWsFrontend(ws *websocket.Conn) {
// 	type FrontendForm struct {
// 		Level string
// 	}
// 	for {
// 		var ff FrontendForm
// 		err := htmxCodec.Receive(ws, &ff)
// 		if err != nil {
// 			return
// 		}
// 		spew.Dump(ff)
// 		err = wsSendText(ws, `<template><svg><image hx-swap-oob="true" id="tankmap" href="/minimap/`+ff.Level+`"></image></svg></template>`)
// 		// err = wsSendComp(ws, "div", "tankmap", frontend.SVGTankmapImage("/minimap/"+level))
// 		if err != nil {
// 			log.Err(err).Msg("sending")
// 		}
// 	}
// }

// func wsSendText(ws *websocket.Conn, content string) error {
// 	fw, err := ws.NewFrameWriter(websocket.TextFrame)
// 	if err != nil {
// 		return err
// 	}
// 	fw.Write([]byte(content))
// 	return fw.Close()
// }

// func wsSendElem(ws *websocket.Conn, elem, id, content string) error {
// 	fw, err := ws.NewFrameWriter(websocket.TextFrame)
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Fprintf(fw, `<%s id="%s">%s</%s>`, elem, id, content, elem)
// 	return fw.Close()
// }

// func wsSendComp(ws *websocket.Conn, elem, id string, content templ.Component) error {
// 	fw, err := ws.NewFrameWriter(websocket.TextFrame)
// 	if err != nil {
// 		return err
// 	}
// 	body := &strings.Builder{}
// 	fmt.Fprintf(body, `<%s id="%s">`, elem, id)
// 	err = content.Render(context.Background(), body)
// 	if err != nil {
// 		return err
// 	}
// 	fmt.Fprintf(body, `</%s>`, elem)
// 	_, err = fw.Write([]byte(body.String()))
// 	if err != nil {
// 		return err
// 	}
// 	return fw.Close()
// }

// var htmxCodec = websocket.Codec{
// 	Marshal: func(v any) (data []byte, payloadType byte, err error) {
// 		switch d := v.(type) {
// 		case string:
// 			data = []byte(d)
// 		case []byte:
// 			data = d
// 		}
// 		return data, websocket.TextFrame, err
// 	},
// 	Unmarshal: func(data []byte, payloadType byte, v any) error {
// 		return json.Unmarshal(data, &v)
// 	},
// }
