#!/bin/bash

find -name '*.go' -and -not -name '*_templ.go' -or -name '*.templ' -or -name build.sh -or -name tsconfig.json -or -path ./static/style.css -or -path './frontend/ts/*.ts' -not -path './frontend/ts/vendor/*' | entr -r ./run.sh "$@"
