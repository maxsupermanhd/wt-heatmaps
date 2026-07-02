package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"

	"github.com/DataDog/zstd"
	"golang.org/x/net/websocket"
)

func main() {
	wsConfig := noerr(websocket.NewConfig("wss://wtapi.dev/v1/replays/ws/random", "https://wtapi.dev/"))
	wsConfig.Header.Add("Authorization", "Bearer 343418440423309314_4bba0a14-53ff-45a9-ac31-559565e30e7b")
	wsConfig.TlsConfig = &tls.Config{
		ServerName:         "wtapi.dev",
		InsecureSkipVerify: false,
		MinVersion:         0,
		MaxVersion:         0,
	}
	fmt.Println("dial")
	wsConn := noerr(tls.Dial("tcp", "wtapi.dev:443", wsConfig.TlsConfig.Clone()))
	ws := noerr(websocket.NewClient(wsConfig, wsConn))
	i := 0
	msgDecompressed := make([]byte, 0, 20_000_000)
	for {
		var msg []byte
		fmt.Println("msg read")
		must(rawMessageCodec.Receive(ws, &msg))
		fmt.Println("got msg ", i, len(msg))
		must(os.WriteFile(fmt.Sprintf("out%04d.dat", i), noerr(zstd.Decompress(msgDecompressed, msg)), 0644))
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
