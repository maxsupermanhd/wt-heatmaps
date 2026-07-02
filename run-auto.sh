#!/bin/bash

find -name '*.go' -and -not -name '*_templ.go' -or -name '*.templ' | entr -r ./run.sh "$@"
