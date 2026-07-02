package lux

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/DataDog/zstd"
	"github.com/rs/zerolog"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/net/websocket"
)

var ErrChannelNotAccepting = errors.New("channel not accepting")

func FetchFromLux(log zerolog.Logger, exitChan <-chan struct{}, carvesChan chan<- *LuxCarve, token string) error {
	wsConfig, err := websocket.NewConfig("wss://wtapi.dev/v1/replays/ws/random", "https://wtapi.dev/")
	if err != nil {
		return err
	}
	wsConfig.Header.Add("Authorization", token)
	wsConfig.TlsConfig = &tls.Config{
		ServerName:         "wtapi.dev",
		InsecureSkipVerify: false,
		MinVersion:         0,
		MaxVersion:         0,
	}
	log.Info().Msg("dialing lux")
	wsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", "wtapi.dev:443", wsConfig.TlsConfig.Clone())
	if err != nil {
		return err
	}
	log.Info().Msg("lux connected")
	wsConn.SetDeadline(time.Now().Add(5 * time.Second))
	ws, err := websocket.NewClient(wsConfig, wsConn)
	if err != nil {
		return err
	}
	wsClose := sync.OnceFunc(func() {
		ws.Close()
	})
	defer wsClose()
	wsConn.SetDeadline(time.Time{})

	shouldClose := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		select {
		case <-exitChan:
			wsClose()
		case <-shouldClose:
		}
	})
	defer wg.Wait()
	defer close(shouldClose)

	log.Info().Msg("lux ws open")

	i := 0
	msgpack.StructAsArray = false
	msgDecompressed := make([]byte, 0, 20_000_000)
	for {
		var msg []byte
		err = rawMessageCodec.Receive(ws, &msg)
		if err != nil {
			return err
		}
		msgDecompressed, err = zstd.Decompress(msgDecompressed, msg)
		if err != nil {
			return err
		}
		var carve LuxCarve
		err = msgpack.Unmarshal(msgDecompressed, &carve)
		if err != nil {
			return err
		}

		select {
		case <-exitChan:
			return nil
		case carvesChan <- &carve:
		default:
			return ErrChannelNotAccepting
		}
		i++
	}
}

var rawMessageCodec = websocket.Codec{
	Marshal: func(v any) (data []byte, payloadType byte, err error) {
		data, err = json.Marshal(v)
		return data, websocket.TextFrame, err
	},
	Unmarshal: func(data []byte, payloadType byte, v any) error {
		*(v.(*[]byte)) = data
		return nil
	},
}
