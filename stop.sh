#!/bin/bash

set -e

cd "$(dirname "$0")"

APP_UNIX_SOCKET="${APP_UNIX_SOCKET:-/tmp/intrasudo26-web.sock}"
RUN_DIR=".run"

stop_pid() {
  if [ -n "$1" ] && kill -0 "$1" 2>/dev/null; then
    echo "Stopping $2 PID $1"
    kill "$1" 2>/dev/null || true
  fi
}

stop_pid_file() {
  if [ -f "$1" ]; then
    pid="$(cat "$1" 2>/dev/null || true)"
    stop_pid "$pid" "$2"
  fi
}

stop_port() {
  pids="$(lsof -ti :"$1" 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    for pid in $pids; do
      stop_pid "$pid" "port $1"
    done
  fi
}

stop_pattern() {
  pids="$(pgrep -f "$1" 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    for pid in $pids; do
      stop_pid "$pid" "$2"
    done
  fi
}

force_pid() {
  if [ -n "$1" ] && kill -0 "$1" 2>/dev/null; then
    echo "Force killing PID $1"
    kill -9 "$1" 2>/dev/null || true
  fi
}

echo "Stopping services..."

stop_pid_file "$RUN_DIR/bot.pid" "discord bot"
stop_pid_file "$RUN_DIR/dawn.pid" "load balancer"
stop_pid_file "$RUN_DIR/main.pid" "main application"
stop_port 8080
stop_port 8081
stop_pattern "./intrasudo26" "main application"
stop_pattern "./dawn -config" "load balancer"
stop_pattern "python3 bot.py" "discord bot"

sleep 1

for file in "$RUN_DIR/bot.pid" "$RUN_DIR/dawn.pid" "$RUN_DIR/main.pid"; do
  if [ -f "$file" ]; then
    pid="$(cat "$file" 2>/dev/null || true)"
    force_pid "$pid"
  fi
done

for port in 8080 8081; do
  pids="$(lsof -ti :"$port" 2>/dev/null || true)"
  if [ -n "$pids" ]; then
    for pid in $pids; do
      force_pid "$pid"
    done
  fi
done

rm -f "$APP_UNIX_SOCKET"
rm -f "$RUN_DIR/main.pid" "$RUN_DIR/dawn.pid" "$RUN_DIR/bot.pid"

echo "Stop complete"
