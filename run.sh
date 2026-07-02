#!/bin/bash

templ generate && go build -v && ./main "$@"
