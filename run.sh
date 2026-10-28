#!/bin/bash

if [[ ! -f lib/ingest/ingestproto/ingestprotogen/replay.pb.go ]] ; then
	t="-tags stubproto"
fi

set -x

templ generate && go build -v $t && ./main "$@"
