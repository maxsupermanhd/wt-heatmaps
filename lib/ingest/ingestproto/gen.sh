#!/bin/bash

protoc replay.proto --go_opt=Mreplay.proto=./ingestprotogen --go_out=.
