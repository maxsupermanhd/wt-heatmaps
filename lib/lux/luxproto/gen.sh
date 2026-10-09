#!/bin/bash

protoc replay.proto --go_opt=Mreplay.proto=./luxprotogen --go_out=.
