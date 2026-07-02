package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"main/lib/caches"
	"main/lib/killstorage"
	"os"
	"os/signal"

	goflexutils "github.com/maxsupermanhd/go-flexutils"
	"github.com/maxsupermanhd/lac/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	flConfigPath  = flag.String("config", "config.json", "path to config json")
	cfg           lac.Conf
	ks            *killstorage.KillsStorage
	tankmapsCache *caches.FetchFileCache
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

	var err error
	ks, err = killstorage.NewKillsStorage(cfg.GetDString(`database=thunder user=thunder password=warthunder_analytics_or_something`, "db"))
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	tankmapsCache, err = caches.NewFetchFileCache(cfg.GetDString("./cache/tankmaps/", "cacheTankmaps"), fetchTankmap)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer ctxCancel()

	stopHttp := goflexutils.StartBackgroundRoutine(log.Logger, "http", httpRoutine)
	stopIngest := goflexutils.StartBackgroundRoutine(log.Logger, "ingest", ingestRoutine)

	<-ctx.Done()
	log.Info().Msg("shutting down")

	stopIngest()
	stopHttp()
	ks.Close()

	log.Info().Msg("bye")
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
