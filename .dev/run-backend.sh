#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

export PATH="${HOME}/.local/sdk/go/bin:${HOME}/.local/sdk/node-v22.16.0-darwin-arm64/bin:${HOME}/.local/bin:${PATH}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

exec /tmp/new-api-server
