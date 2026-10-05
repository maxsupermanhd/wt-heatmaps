package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	killstorage "main/lib/killstorage-duckdb"
	"os"
	"os/signal"

	"github.com/maxsupermanhd/flexcorallib/fcl"
	"github.com/maxsupermanhd/flexcorallib/fclcache"
	goflexutils "github.com/maxsupermanhd/go-flexutils"
	"github.com/maxsupermanhd/lac/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

var bgctx, bgctxcancel = signal.NotifyContext(context.Background(), os.Interrupt)

var (
	flConfigPath = flag.String("config", "config.json", "path to config json")
	cfg          lac.Conf
	ks           *killstorage.KillsStorage
	wb           = fcl.NewWorkerPool(bgctx, 2)
)

func main() {
	configLoad(*flConfigPath)
	log.Logger = log.Output(io.MultiWriter(
		zerolog.ConsoleWriter{Out: os.Stderr},
		&lumberjack.Logger{
			Filename: cfg.GetDString("logs/heatmaps.log", "logs", "filename"),
			MaxSize:  cfg.GetDInt(100, "logs", "maxSize"),
			Compress: true,
		}))
	log.Info().Msg("hello world")

	cachedTankmaps = noerr(fclcache.NewFetchFileCache(cfg.GetDString("./cache/tankmaps/", "cacheTankmaps"), tankmapFetchB64LEV))

	var err error

	log.Info().Msg("connecting to database")
	ks, err = killstorage.NewKillsStorage(cfg.GetDString(`database=thunder user=thunder password=warthunder_analytics_or_something`, "db"))
	if err != nil {
		log.Fatal().Err(err).Msg("db connect")
	}

	log.Info().Msg("loading wpcost")
	err = wpcostLoad()
	if err != nil {
		log.Fatal().Err(err).Msg("wpcost load")
	}

	log.Info().Msg("loading level names")
	initLevelNames()

	stopHttp := goflexutils.StartBackgroundRoutine(log.Logger, "http", httpRoutine)
	stopIngest := goflexutils.StartBackgroundRoutine(log.Logger, "ingest", ingestRoutine)

	go reportCall(func() { levelStatsSorted.Refresh(bgctx) }, "warming level sorting cache")
	go reportCall(func() { cachedStatsTables.Refresh(bgctx) }, "warming stat tables cache")

	log.Info().Msg("init done")
	<-bgctx.Done()
	log.Info().Msg("shutting down")

	stopIngest()
	stopHttp()
	ks.Close()

	log.Info().Msg("bye")
}

func reportCall(fn func(), name string) {
	log.Info().Msg(name)
	fn()
	log.Info().Msg("done " + name)
}

func configLoad(configPath string) {
	var err error
	cfg, err = lac.FromFileJSON(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg = lac.NewConf()
			return
		}
		fmt.Fprintln(os.Stderr, "Failed to load config: "+err.Error())
		os.Exit(1)
		panic(err)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func noerr[T any](ret T, err error) T {
	must(err)
	return ret
}
