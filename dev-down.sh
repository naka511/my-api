#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_DIR="$ROOT_DIR/.pids"
UID_VALUE="$(id -u)"

launchctl bootout "gui/${UID_VALUE}" "${HOME}/Library/LaunchAgents/com.newapi.dev.backend.plist" >/dev/null 2>&1 || true
launchctl bootout "gui/${UID_VALUE}" "${HOME}/Library/LaunchAgents/com.newapi.dev.frontend.plist" >/dev/null 2>&1 || true

stop_by_pid_file() {
  local file="$1"
  if [ -f "$file" ]; then
    local pid
    pid="$(cat "$file")"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" || true
    fi
    rm -f "$file"
  fi
}

stop_by_pid_file "$PID_DIR/backend.pid"
stop_by_pid_file "$PID_DIR/frontend.pid"

pkill -f "/tmp/new-api-server" || true
pkill -f "rsbuild dev --host 0.0.0.0 --port 3001" || true
pkill -f "vite --host 0.0.0.0 --port 3001" || true
pkill -f "npm run dev -- --host 0.0.0.0 --port 3001" || true
pkill -f "bun run dev --host 0.0.0.0 --port 3001" || true

echo "Stopped dev services."
