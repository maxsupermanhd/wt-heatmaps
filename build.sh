#!/bin/bash
set -euxo pipefail

tags=()
if [[ ! -f lib/ingest/ingestproto/ingestprotogen/replay.pb.go ]]; then
	tags=(-tags stubproto)
fi

go tool tsgo
go tool esbuild frontend/ts/main.ts --bundle --format=esm --outdir=static/dist --log-level=warning
go tool templ generate
go build -v "${tags[@]}"
