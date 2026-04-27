#!/bin/bash
# Start Tauri dev with a configurable port via TAURI_DEV_PORT env var.
# Usage: TAURI_DEV_PORT=1520 npm run tauri:dev
#        npm run tauri:dev                          (defaults to 1420)

set -euo pipefail
source ~/.cargo/env

PORT="${TAURI_DEV_PORT:-1420}"
CONFIG='{"build":{"devUrl":"http://localhost:'"${PORT}"'"}}'

npm run tauri dev -- --config "$CONFIG"
