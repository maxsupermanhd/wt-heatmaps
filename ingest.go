package main

import (
	"context"
	"encoding/json"
	"main/lib/killstorage"
	"main/lib/lux"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

func ingestRoutine(exitChan <-chan struct{}) {
	luxToken, haveLuxToken := cfg.GetString("luxToken")
	if !haveLuxToken {
		return
	}
	carvesChan := make(chan *lux.LuxCarve, 16)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Go(func() {
		for carve := range carvesChan {
			kills, err := killstorage.LuxCarveToKills(carve)
			if err != nil {
				log.Err(err).Msg("carve to kills fail")
				b, _ := json.Marshal(carve)
				os.WriteFile("dump.json", b, 0644)
				continue
			}
			err = ks.StoreKills(kills)
			log.Err(err).Int("n", len(kills)).Msg("storing kills")
		}
		log.Info().Msg("carve loop exited")
	})
	wg.Go(func() {
		defer log.Info().Msg("lux loop exited")
		defer close(carvesChan)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			err := lux.FetchFromLux(log.Logger, ctx.Done(), carvesChan, luxToken)
			log.Err(err).Msg("lux fetch exited")
		}
	})

	<-exitChan

	cancel()

	wg.Wait()

}
