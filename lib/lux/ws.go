package lux

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"main/lib/lux/luxproto/luxprotogen"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/zstd"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/shamaton/msgpack/v3"
	"golang.org/x/net/websocket"
	"google.golang.org/protobuf/proto"
)

func FetchFromLux(log zerolog.Logger, exitChan <-chan struct{}, carvesChan chan<- *luxprotogen.Replay, preferences <-chan any, token string) error {
	ws, err := dialLux(log, token)
	if err != nil {
		return fmt.Errorf("dial lux: %w", err)
	}
	shouldCloseWriter := make(chan struct{})
	wsClose := sync.OnceFunc(func() {
		ws.Close()
		close(shouldCloseWriter)
	})
	defer wsClose()

	var wg sync.WaitGroup
	var errWrite, errRead error

	wg.Go(func() {
		defer log.Info().Msg("write pump exited")
		codec := websocket.Codec{
			Marshal: func(v any) (data []byte, payloadType byte, err error) {
				data, err = json.Marshal(v)
				return data, websocket.TextFrame, err
			}}
		for {
			select {
			case <-exitChan:
				wsClose()
				return
			case <-shouldCloseWriter:
				return
			case v, ok := <-preferences:
				if !ok {
					wsClose()
					return
				}
				errWrite = codec.Send(ws, v)
				if errWrite != nil {
					wsClose()
					return
				}
				log.Info().Msg("sent preferences")
			}
		}
	})

	wg.Go(func() {
		defer log.Info().Msg("read pump exited")
		msgpack.StructAsArray = false
		msgDecompressed := make([]byte, 0, 20_000_000)
		var msg []byte
		var msgType byte
		codec := websocket.Codec{
			Unmarshal: func(data []byte, payloadType byte, v any) error {
				msgType = payloadType
				*(v.(*[]byte)) = data
				return nil
			},
		}
		for {
			errRead = codec.Receive(ws, &msg)
			if errRead != nil {
				wsClose()
				return
			}
			switch msgType {
			case websocket.TextFrame:
				log.Info().Str("data", string(msg)).Msg("text frame")
			case websocket.BinaryFrame:
				// log.Info().Int("data", len(msg)).Msg("binary frame")
				// timings := time.Now()

				msgDecompressed, errRead = zstd.Decompress(msgDecompressed, msg)
				// timingsDecomp := time.Since(timings)

				if errRead != nil {
					wsClose()
					return
				}

				var m luxprotogen.Envelope

				// timings = time.Now()

				errRead = proto.Unmarshal(msgDecompressed, &m)
				if errRead != nil {
					log.Err(err).Msg("reading envelope")
					wsClose()
					return
				}
				if m.Kind != "full" {
					log.Error().Msg("not full replay kind")
					wsClose()
					return
				}
				if m.SchemaVersion != 2 {
					log.Error().Msg("proto schema wrong version")
					wsClose()
					return
				}

				var carve luxprotogen.Replay
				errRead = proto.Unmarshal(m.Body, &carve)
				if errRead != nil {
					log.Err(err).Msg("reading envelope body")
					wsClose()
					return
				}

				// timingsParse := time.Since(timings)
				// log.Info().Str("timingsDecomp", timingsDecomp.Round(time.Nanosecond).String()).
				// 	Str("timingsParse", timingsParse.Round(time.Nanosecond).String()).
				// 	Int("len", len(msgDecompressed)).Msg("got battle report")

				select {
				case carvesChan <- &carve:
				default:
				}
			}
		}
	})

	log.Info().Msg("lux ws open")

	wg.Wait()

	return errors.Join(errWrite, errRead)
}

func dialLux(log zerolog.Logger, token string) (*websocket.Conn, error) {
	wsConfig, err := websocket.NewConfig("wss://wtapi.dev/v1/replays/ws/random", "https://wtapi.dev/")
	if err != nil {
		return nil, err
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
		return nil, err
	}
	log.Info().Msg("lux connected")
	wsConn.SetDeadline(time.Now().Add(5 * time.Second))
	// ws, err := websocket.NewClient(wsConfig, &wsInspectRWC{conn: wsConn})
	ws, err := websocket.NewClient(wsConfig, wsConn)
	wsConn.SetDeadline(time.Time{})
	return ws, err
}

type wsInspectRWC struct {
	conn io.ReadWriteCloser
}

func (ins *wsInspectRWC) Read(p []byte) (n int, err error) {
	n, err = ins.conn.Read(p)
	log.Err(err).Str("v", strings.Trim(string(p), "\x00")).Msg("inspect read")
	return
}
func (ins *wsInspectRWC) Write(p []byte) (n int, err error) {
	log.Err(err).Str("v", strings.Trim(string(p), "\x00")).Msg("inspect write")
	return ins.conn.Write(p)
}
func (ins *wsInspectRWC) Close() error {
	return ins.conn.Close()
}
