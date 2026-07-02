package main

import (
	"image"
	"image/color"
	"image/png"
	"main/frontend"
	"main/lib/killstorage"
	"main/lib/levelcoords"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/rs/zerolog/log"
)

func makeHTTPServeMux() http.HandlerFunc {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", httpLog(handle404))
	mux.HandleFunc("GET /static/", httpLog(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))).ServeHTTP))
	mux.HandleFunc("GET /{$}", httpLog(compRender(serveIndex)))

	mux.HandleFunc("GET /minimap/{k...}", httpLog(serveCachedMinimaps))
	mux.HandleFunc("GET /heat", httpLog(serveHeat))
	// mux.HandleFunc("GET /frontend/mapUpdate", httpLog(compRender(serveFrontendMapUpdate)))
	// mux.HandleFunc("GET /ws/frontend", httpLog(websocket.Server{
	// 	Handler: handleWsFrontend,
	// }.ServeHTTP))

	return mux.ServeHTTP
}

func serveIndex(w http.ResponseWriter, r *http.Request) templ.Component {
	levels, missions, vehicles, weapons := ks.GetMeta()
	_ = levels
	_ = missions
	_ = vehicles
	_ = weapons
	return frontend.Page(frontend.Index(levels))
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

	levelOffsets, err := levelcoords.GetLevelCoordsCached(cfg.GetDString("cache/offsets.json", "cacheOffsets"), level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	kq := killstorage.NewKillsQuery(time.Now().Add(-7*24*time.Hour), time.Now())
	ks.QueryWithLevel(kq, level)
	tally, err := ks.GetKillCountsByCoord(r.Context(), kq)
	if err != nil {
		log.Err(err).Msg("get kills")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	out := image.NewRGBA(image.Rect(0, 0, 2048, 2048))

	areaW := float32(math.Abs(float64(levelOffsets.TankMap0[0] - levelOffsets.TankMap1[0])))
	areaH := float32(math.Abs(float64(levelOffsets.TankMap0[1] - levelOffsets.TankMap1[1])))
	areaOffsetX := levelOffsets.TankMap0[0]
	areaOffsetZ := levelOffsets.TankMap0[1]
	outputW := int(areaW)
	outputH := int(areaH)

	scaleW := float32(areaW / 2048)
	scaleH := float32(areaH / 2048)

	for _, v := range tally {
		tx := int(float64((float32(v.X)-areaOffsetX)/areaW)*float64(outputW)) / int(scaleH)
		tz := int(float64(1-(float32(v.Z)-areaOffsetZ)/areaH)*float64(outputH)) / int(scaleW)
		out.SetRGBA(int(tx), int(tz), color.RGBA{
			R: uint8(max(min(v.Score*scoreIntensity, 255), 0)),
			G: 0,
			B: uint8(max(min(-v.Score*scoreIntensity, 255), 0)),
			A: uint8(min(v.Count*countIntensity, 255)),
		})
	}
	png.Encode(w, out)
	log.Info().Dur("perf", time.Since(perf)).Int("n", len(tally)).Msg("heat")
}

func compRender(f func(w http.ResponseWriter, r *http.Request) templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := f(w, r)
		if c != nil {
			c.Render(r.Context(), w)
		}
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
