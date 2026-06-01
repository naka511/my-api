#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOG_DIR="$ROOT_DIR/logs"
PID_DIR="$ROOT_DIR/.pids"
mkdir -p "$LOG_DIR" "$PID_DIR"

NODE_BIN_DIR="${HOME}/.local/sdk/node-v22.16.0-darwin-arm64/bin"
GO_BIN_DIR="${HOME}/.local/sdk/go/bin"
BUN_BIN_DIR="${HOME}/.bun/bin"
export PATH="${NODE_BIN_DIR}:${GO_BIN_DIR}:${BUN_BIN_DIR}:${PATH}"
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"

if [ ! -x "${GO_BIN_DIR}/go" ]; then
  echo "go not found: ${GO_BIN_DIR}/go"
  exit 1
fi

export NODE_OPTIONS="${NODE_OPTIONS:---max-old-space-size=8192}"

echo "Build classic frontend (same as Zeabur)..."
(cd "$ROOT_DIR/web/classic" && bun --bun run build) >/dev/null

echo "Build default frontend for backend embed..."
(cd "$ROOT_DIR/web/default" && bun --bun run build) >/dev/null

echo "Build backend binary..."
(cd "$ROOT_DIR" && go build -o /tmp/new-api-server ./main.go)

echo "Start backend/frontend. Keep this terminal open."
"$ROOT_DIR/.dev/run-backend.sh" >"$LOG_DIR/backend.log" 2>&1 &
BACKEND_PID=$!
echo "$BACKEND_PID" >"$PID_DIR/backend.pid"

"$ROOT_DIR/.dev/run-frontend-classic.sh" >"$LOG_DIR/frontend.log" 2>&1 &
FRONTEND_PID=$!
echo "$FRONTEND_PID" >"$PID_DIR/frontend.pid"

cleanup() {
  kill "$BACKEND_PID" "$FRONTEND_PID" >/dev/null 2>&1 || true
  rm -f "$PID_DIR/backend.pid" "$PID_DIR/frontend.pid"
}
trap cleanup EXIT INT TERM

sleep 2

echo "Done:"
echo "  Backend  http://localhost:3000"
echo "  Frontend http://localhost:3001"
echo "  Logs     $LOG_DIR/backend.log"
echo "           $LOG_DIR/frontend.log"
echo
echo "Press Ctrl+C here when you want to stop local dev."

wait "$BACKEND_PID" "$FRONTEND_PID"
