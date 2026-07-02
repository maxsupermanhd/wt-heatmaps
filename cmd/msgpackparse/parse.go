package main

import (
	"encoding/json"
	"fmt"
	"main/lib/lux"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/shamaton/msgpack/v3"
)

func main() {
	fmt.Println("file read")
	dataBytes := noerr(os.ReadFile("../ingest/out0000.dat"))

	dumpCarveAny(dataBytes)
	dumpCarveStruct(dataBytes)
}

func dumpCarveAny(data []byte) {
	var carve any

	perf := time.Now()
	fmt.Println("unmarshal to any")

	msgpack.StructAsArray = false
	must(msgpack.Unmarshal(data, &carve))

	fmt.Println("done", time.Since(perf))

	spew.Config.SortKeys = true
	must(os.WriteFile("carve.spew", []byte(spew.Sdump(carve)), 0644))
}

func dumpCarveStruct(data []byte) {
	var carve lux.LuxCarve

	perf := time.Now()
	fmt.Println("unmarshal to carve")

	msgpack.StructAsArray = false
	must(msgpack.Unmarshal(data, &carve))

	fmt.Println("done", time.Since(perf))

	must(os.WriteFile("carve.json", noerr(json.MarshalIndent(carve, "", "\t")), 0644))
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
