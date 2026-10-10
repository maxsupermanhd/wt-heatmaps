package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"main/frontend"
	killstorage "main/lib/killstorage-duckdb"
	"main/lib/staticassets"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/fogleman/gg"
	"github.com/maxsupermanhd/flexcorallib/fclcache"
	"github.com/rs/zerolog/log"
)

func makeHTTPServeMux() http.HandlerFunc {
	staticAssets := staticassets.New("static", "/static/")
	frontend.StaticAssets = staticAssets
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", httpLog(handle404))
	mux.HandleFunc("GET /robots.txt", httpLog(handleRobots))
	mux.HandleFunc("GET /static/{name...}", httpLog(staticAssets.ServeHTTP))
	mux.HandleFunc("GET /{$}", httpLog(ensureCached(compRenderFn(serveIndex), levelStatsSorted)))
	mux.HandleFunc("GET /stats", httpLog(ensureCached(compRenderFn(serveStats), cachedStatsTables)))
	mux.HandleFunc("GET /about", httpLog(compRender(frontend.Page(frontend.About()))))
	mux.HandleFunc("GET /about/api", httpLog(compRender(frontend.Page(frontend.API()))))
	mux.HandleFunc("GET /about/changelog", httpLog(compRender(frontend.Page(frontend.Changelog()))))
	mux.HandleFunc("GET /waitroom/{p...}", httpLog(compRenderFn(seveWaitroom)))

	mux.HandleFunc("GET /minimap/{size}/{k...}", serveCachedMinimaps)
	mux.HandleFunc("GET /render/heat", httpLog(serveHeat))
	mux.HandleFunc("GET /data/arrows", httpLog(serveAreaArrows))
	mux.HandleFunc("GET /data/areastats", httpLog(compRenderFn(serveAreaStats)))
	mux.HandleFunc("GET /api/v1/region", httpLog(serveRegion))

	mux.HandleFunc("GET /debug/duckdbmemory", httpLog(serveDebugDuckdbMemory))
	mux.HandleFunc("GET /debug/wpcost", httpLog(serveDebugWpcost))

	mux.HandleFunc("GET /missions...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /clans...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /players...", httpLog(servePermaRedirect("/")))
	mux.HandleFunc("GET /sessions...", httpLog(servePermaRedirect("/")))

	return mux.ServeHTTP
}

var (
	waitroomChecksMu sync.Mutex
	waitroomChecks   = map[string][]fclcache.ValueCacheCommon{}
)

func ensureCached(work http.HandlerFunc, checks ...fclcache.ValueCacheCommon) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		waitroomChecksMu.Lock()
		waitroomChecks[r.URL.Path] = checks
		waitroomChecksMu.Unlock()
		for _, c := range checks {
			if !c.Ready() {
				c.Refresh(r.Context())
			}
		}
		for _, c := range checks {
			if !c.Ready() {
				w.Header().Add("Location", "/waitroom/"+url.PathEscape(r.URL.Path))
				w.WriteHeader(http.StatusFound)
				return
			}
		}
		work(w, r)
	}
}

func seveWaitroom(w http.ResponseWriter, r *http.Request) templ.Component {
	p := r.PathValue("p")
	if p == "" || p[0] != '/' {
		w.Header().Add("Location", "/")
		w.WriteHeader(http.StatusFound)
		return nil
	}
	waitroomChecksMu.Lock()
	checks := waitroomChecks[p]
	waitroomChecksMu.Unlock()
	isReady := true
	for _, c := range checks {
		if !c.Ready() {
			c.Refresh(r.Context())
			isReady = false
		}
	}
	if isReady {
		w.Header().Add("Location", p)
		w.WriteHeader(http.StatusFound)
		return nil
	}
	w.Header().Add("Cache-Control", "no-store")
	w.Header().Add("Refresh", "4")
	return frontend.WaitroomPage()
}

func serveIndex(w http.ResponseWriter, r *http.Request) templ.Component {
	levels, err := levelStatsSorted.Get(r.Context())
	if err != nil {
		log.Err(err).Msg("level stats sorted")
		return frontend.Page(frontend.TextNode("something went really wrong"))
	}
	vehicles, err := ks.GetVehicles(r.Context())
	if err != nil {
		log.Err(err).Msg("level stats sorted")
		return frontend.Page(frontend.TextNode("something went really wrong"))
	}
	slices.Sort(vehicles)
	return frontend.Page(frontend.Index(levels, vehicleEconomyCatalog.GetRankMax()))
}

func serveHeat(w http.ResponseWriter, r *http.Request) {
	perf := time.Now()
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

	kq := buildKillQuery(q, level)
	if kq == nil {
		w.WriteHeader(204)
		return
	}

	// log.Info().Msg(kq.Dump())

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

	scoreIntensity := urlValueIntOr(q, "scoreIntensity", 32)
	countIntensity := urlValueIntOr(q, "countIntensity", 32)

	// log.Info().Msgf("scale %f %f area %f %f offsets %#v", scaleW, scaleH, areaW, areaH, levelOffsets)
	totalN := 0
	for _, v := range tally {
		tx := math.Round(float64(float64((float32(v.X)-areaOffsetX)/areaW) * float64(outputW)))
		tz := math.Round(float64(float64(1-(float32(v.Z)-areaOffsetZ)/areaH) * float64(outputH)))

		out.SetRGBA(int(tx), int(tz), color.RGBA{
			R: uint8(max(min(v.Score*scoreIntensity, 255), 0)),
			G: uint8(max(min(v.Score*v.Score, 255), 0) / 2),
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

func serveAreaArrows(w http.ResponseWriter, r *http.Request) {
	perf := time.Now()
	q := r.URL.Query()
	level := q.Get("level")
	box, ok := areaBoxFromQuery(q)
	if level == "" || !ok {
		w.WriteHeader(400)
		return
	}

	levelOffsets, err := getLevelOffsets(level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		w.Write([]byte(err.Error()))
		return
	}

	kq := buildKillQuery(q, level)
	if kq == nil {
		w.WriteHeader(204)
		return
	}
	kq.QueryWithArea(levelOffsets.TankMapAreaToWorld(box))

	tally, err := ks.GetAreaArrows(r.Context(), kq)
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
	// outputW := int(areaW)
	// outputH := int(areaH)
	// out := gg.NewContext(outputW, outputH)
	// out.SetRGBA255(255, 30, 30, 255)

	fmt.Fprint(w, `<g id="mapviewLines" stroke="#f11a" width="100%" height="100%" stroke-width="0.3" stroke-linecap="round">`)

	totalN := 0
	for _, v := range tally {
		t1x := 0.25 + (math.Round(float64(float64((float32(v.FromX)-areaOffsetX)/areaW) * float64(2048))))
		t1z := 0.25 + (math.Round(float64(float64(1-(float32(v.FromZ)-areaOffsetZ)/areaH) * float64(2048))))
		t2x := 0.25 + (math.Round(float64(float64((float32(v.ToX)-areaOffsetX)/areaW) * float64(2048))))
		t2z := 0.25 + (math.Round(float64(float64(1-(float32(v.ToZ)-areaOffsetZ)/areaH) * float64(2048))))

		fmt.Fprintf(w, `<line x1="%v" y1="%v" x2="%v" y2="%v"></line>`, t1x, t1z, t2x, t2z)

		// out.DrawLine(t1x, t1z, t2x, t2z)
		// out.Stroke()
		// out.SetRGBA(int(tx), int(tz), color.RGBA{R: 255, G: 30, B: 30, A: 255})
		totalN++
	}
	fmt.Fprint(w, `</g>`)
	// out.EncodePNG(w)
	log.Info().Dur("perf", time.Since(perf)).Int("nPix", len(tally)).Int("nDp", totalN).Msg("heat")
}

func buildKillQuery(q url.Values, level string) *killstorage.QueryConditions {
	kq := &killstorage.QueryConditions{}
	if !ks.QueryWithLevel(kq, level) {
		return nil
	}
	if val := urlValueInt(q, "team"); val != nil {
		kq.QueryWithTeam(*val)
	}
	if val := urlValueInt(q, "killTimeMin"); val != nil {
		kq.QueryWithKillTimeMin(time.Duration(*val) * time.Second)
	}
	if val := urlValueInt(q, "killTimeMax"); val != nil {
		kq.QueryWithKillTimeMax(time.Duration(*val) * time.Second)
	}
	killerVehicles := vehicleEconomyCatalog.GetAllInRange(urlValueInt(q, "killerBattleRatingMin"), urlValueInt(q, "killerBattleRatingMax"))
	if val := q.Get("killerVehicleClass"); val != "" {
		log.Info().Str("killerVehicleClass", val).Msg("killerVehicleClass")
		if len(killerVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.UnitClass == val {
					killerVehicles = append(killerVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range killerVehicles {
				if vehicleEconomyCatalog.Vehicles[v].UnitClass == val {
					filtered = append(filtered, v)
				}
			}
			killerVehicles = filtered
		}
	}
	if val := q.Get("killerVehicleCountry"); val != "" {
		if len(killerVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.Country == val {
					killerVehicles = append(killerVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range killerVehicles {
				if vehicleEconomyCatalog.Vehicles[v].Country == val {
					filtered = append(filtered, v)
				}
			}
			killerVehicles = filtered
		}
	}
	if len(killerVehicles) > 0 {
		for i := range killerVehicles {
			killerVehicles[i] = "tankmodels/" + killerVehicles[i]
		}
		ks.QueryWithKillerVehicles(kq, killerVehicles)
	}
	victimVehicles := vehicleEconomyCatalog.GetAllInRange(urlValueInt(q, "victimBattleRatingMin"), urlValueInt(q, "victimBattleRatingMax"))
	if val := q.Get("victimVehicleClass"); val != "" {
		if len(victimVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.UnitClass == val {
					victimVehicles = append(victimVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range victimVehicles {
				if vehicleEconomyCatalog.Vehicles[v].UnitClass == val {
					filtered = append(filtered, v)
				}
			}
			victimVehicles = filtered
		}
	}
	if val := q.Get("victimVehicleCountry"); val != "" {
		if len(victimVehicles) == 0 {
			for k, v := range vehicleEconomyCatalog.Vehicles {
				if v.Country == val {
					victimVehicles = append(victimVehicles, k)
				}
			}
		} else {
			filtered := []string{}
			for _, v := range victimVehicles {
				if vehicleEconomyCatalog.Vehicles[v].Country == val {
					filtered = append(filtered, v)
				}
			}
			victimVehicles = filtered
		}
	}
	if len(victimVehicles) > 0 {
		for i := range victimVehicles {
			victimVehicles[i] = "tankmodels/" + victimVehicles[i]
		}
		ks.QueryWithVictimVehicles(kq, victimVehicles)
	}
	return kq
}

const areaStatsLimit = 25

func serveAreaStats(w http.ResponseWriter, r *http.Request) templ.Component {
	q := r.URL.Query()
	level := q.Get("level")
	box, ok := areaBoxFromQuery(q)
	if level == "" || !ok {
		w.WriteHeader(400)
		return nil
	}

	levelOffsets, err := getLevelOffsets(level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		return nil
	}
	x0, z0, x1, z1 := levelOffsets.TankMapAreaToWorld(box)

	kq := buildKillQuery(q, level)
	if kq == nil {
		w.WriteHeader(204)
		return nil
	}
	kq.QueryWithArea(x0, z0, x1, z1)

	stats, err := ks.GetVehicleStatsByArea(r.Context(), kq, areaStatsLimit)
	if err != nil {
		log.Err(err).Msg("get vehicle stats by area")
		w.WriteHeader(500)
		return nil
	}

	rows := make([]frontend.AreaVehicleStat, len(stats))
	total := 0
	for i, v := range stats {
		vehicleName := strings.TrimPrefix(v.Vehicle, "tankmodels/")
		rows[i] = frontend.AreaVehicleStat{
			Vehicle: frontendVehicle(vehicleName, vehicleEconomyCatalog.Vehicles[vehicleName]),
			Kills:   v.Kills,
			Deaths:  v.Deaths,
		}
		total += v.Kills + v.Deaths
	}
	return frontend.AreaStats(rows, int(math.Round(math.Abs(x1-x0))), int(math.Round(math.Abs(z1-z0))), total, areaStatsLimit)
}

type RegionVehicleStat struct {
	Vehicle string `json:"vehicle"`
	Kills   int    `json:"kills"`
	Deaths  int    `json:"deaths"`
}

// serveRegion answers the same question as serveAreaStats, in JSON.
func serveRegion(w http.ResponseWriter, r *http.Request) {
	if !apiAuthorizedFor(w, r, "region") {
		return
	}
	q := r.URL.Query()
	level := q.Get("level")
	box, ok := areaBoxFromQuery(q)
	if level == "" || !ok {
		w.WriteHeader(400)
		return
	}

	levelOffsets, err := getLevelOffsets(level)
	if err != nil {
		log.Err(err).Msg("get level offsets")
		w.WriteHeader(500)
		return
	}
	x0, z0, x1, z1 := levelOffsets.TankMapAreaToWorld(box)

	kq := buildKillQuery(q, level)
	if kq == nil {
		w.WriteHeader(204)
		return
	}
	kq.QueryWithArea(x0, z0, x1, z1)

	stats, err := ks.GetVehicleStatsByArea(r.Context(), kq, areaStatsLimit)
	if err != nil {
		log.Err(err).Msg("get vehicle stats by area")
		w.WriteHeader(500)
		return
	}

	// Made with a length, so an empty box encodes as [] and not as null.
	rows := make([]RegionVehicleStat, len(stats))
	for i, v := range stats {
		rows[i] = RegionVehicleStat{
			Vehicle: strings.TrimPrefix(v.Vehicle, "tankmodels/"),
			Kills:   v.Kills,
			Deaths:  v.Deaths,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(rows)
	if err != nil {
		log.Err(err).Msg("write region response")
	}
}

func areaBoxFromQuery(vals url.Values) (box [4]float64, ok bool) {
	for i, name := range []string{"u0", "v0", "u1", "v1"} {
		v, err := strconv.ParseFloat(vals.Get(name), 64)
		if err != nil || math.IsNaN(v) {
			return box, false
		}
		box[i] = v
	}
	return box, box[0] != box[2] && box[1] != box[3]
}

func urlValueInt(vals url.Values, name string) *int {
	if !vals.Has(name) {
		return nil
	}
	ret, err := strconv.Atoi(vals.Get(name))
	if err != nil {
		return nil
	}
	return &ret
}

func urlValueIntOr(vals url.Values, name string, or int) int {
	if !vals.Has(name) {
		return or
	}
	ret, err := strconv.Atoi(vals.Get(name))
	if err != nil {
		return or
	}
	return ret
}

func compRenderFn(f func(w http.ResponseWriter, r *http.Request) templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c := f(w, r)
		if c != nil {
			c.Render(r.Context(), w)
		}
	}
}

func compRender(c templ.Component) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		c.Render(r.Context(), w)
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

func serveDebugWpcost(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "\t")
	e.Encode(vehicleEconomyCatalog.Vehicles)
}

func serveDebugDuckdbMemory(w http.ResponseWriter, r *http.Request) {
	ret, err := ks.DebugMemory(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error() + "\n\n"))
		return
	}
	w.WriteHeader(http.StatusOK)
	e := json.NewEncoder(w)
	e.SetIndent("", "\t")
	e.Encode(ret)
}

func servePermaRedirect(location string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Location", location)
		w.WriteHeader(http.StatusMovedPermanently)
	}
}

func handleRobots(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	fmt.Fprint(w, "User-agent: *\nAllow: /\n")
}

func apiAuthorizedFor(w http.ResponseWriter, r *http.Request, intent string) bool {
	auth := r.Header.Get("Authorization")
	have, ok := cfg.GetSliceString(append([]string{"auth"}, auth, "intents")...)
	if !ok {
		if w != nil {
			denyApiAuth(w)
		}
		return false
	}
	if !slices.Contains(have, intent) {
		if w != nil {
			denyApiAuth(w)
		}
		return false
	}
	return true
}

func denyApiAuth(w http.ResponseWriter) {
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("sorry, you are not allowed to use this, feel free to request access in discord @flexcoral (343418440423309314) https://discord.gg/hQEvvtwfhJ\n\n"))
}
