#!/bin/bash
set -euxo pipefail

./build.sh
exec ./main "$@"
