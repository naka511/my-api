#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR/web/classic"

export PATH="${HOME}/.local/sdk/node-v22.16.0-darwin-arm64/bin:${HOME}/.local/sdk/go/bin:${HOME}/.local/bin:${PATH}"
export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=4096}"

exec npm run dev -- --host 0.0.0.0 --port 3001
