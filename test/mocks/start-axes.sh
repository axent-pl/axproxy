#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"

docker run \
  --name axes \
  -e SERVER_ADDRESS=:8080 \
  -p 8080:8080 \
  -v "${SCRIPT_DIR}/config.json:/app/assets/config/config.json:ro" \
  prond/axes:nightly
