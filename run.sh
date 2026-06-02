#!/bin/bash

set -e

echo "███████╗ ██╗  ██╗ ██╗   ██╗ ███╗   ██╗"
echo "██╔════╝ ██║  ██║ ██║   ██║ ████╗  ██║"
echo "█████╗     ███╔═╝ ██║   ██║ ██╔██╗ ██║"
echo "██╔══╝   ██╔══██║ ██║   ██║ ██║╚██╗██║"
echo "███████╗ ██║  ██║ ╚██████╔╝ ██║ ╚████║"
echo "╚══════╝ ╚═╝  ╚═╝  ╚═════╝  ╚═╝  ╚═══╝"

cd "$(dirname "$0")"

export APP_UNIX_SOCKET="${APP_UNIX_SOCKET:-/tmp/intrasudo26-web.sock}"
export BACKEND_BASE="${BACKEND_BASE:-http://127.0.0.1:8080}"
export BOT_HOST="${BOT_HOST:-127.0.0.1}"
export BOT_PORT="${BOT_PORT:-5555}"
export BOT_SERVICE_URL="${BOT_SERVICE_URL:-http://127.0.0.1:${BOT_PORT}}"

DAWN_CONFIG="${DAWN_CONFIG:-config.json}"
RUN_DIR=".run"
mkdir -p "$RUN_DIR"
rm -f "$RUN_DIR/main.pid" "$RUN_DIR/dawn.pid" "$RUN_DIR/bot.pid"

stop_pid() {
    if [ -n "$1" ] && kill -0 "$1" 2>/dev/null; then
        kill "$1" 2>/dev/null || true
    fi
}

cleanup() {
    echo "Stopping all services..."
    stop_pid "$BOT_PID"
    stop_pid "$DAWN_PID"
    stop_pid "$MAIN_PID"
    rm -f "$APP_UNIX_SOCKET"
    rm -f "$RUN_DIR/main.pid" "$RUN_DIR/dawn.pid" "$RUN_DIR/bot.pid"
    echo "All services stopped."
    exit 0
}

check_alive() {
    if ! kill -0 "$1" 2>/dev/null; then
        echo "$2 failed to start"
        cleanup
    fi
}

trap cleanup SIGINT SIGTERM

echo "Building main application"
go build -o intrasudo26 .

echo "Building load balancer"
(cd dawn && go build -o dawn ./cmd)

echo "Starting main application"
./intrasudo26 &
MAIN_PID=$!
echo "$MAIN_PID" > "$RUN_DIR/main.pid"
sleep 1
check_alive "$MAIN_PID" "Main application"

echo "Starting load balancer"
(cd dawn && ./dawn -config "$DAWN_CONFIG") &
DAWN_PID=$!
echo "$DAWN_PID" > "$RUN_DIR/dawn.pid"
sleep 1
check_alive "$DAWN_PID" "Load balancer"

if command -v python3 >/dev/null 2>&1; then
    echo "Starting discord bot"
    PYTHONUNBUFFERED=1 python3 bot.py &
    BOT_PID=$!
    echo "$BOT_PID" > "$RUN_DIR/bot.pid"
    sleep 1
    check_alive "$BOT_PID" "Discord bot"
else
    echo "python3 not found; discord bot not started"
    BOT_PID=""
fi

echo "All services started:"
echo "Main App PID: $MAIN_PID"
echo "Load Balancer PID: $DAWN_PID"
echo "Discord Bot PID: ${BOT_PID:-not started}"
echo "Website: http://127.0.0.1:8080/"
echo "Load Balancer Panel: http://127.0.0.1:8080/lb/panel/"
echo "Backend Socket: $APP_UNIX_SOCKET"
echo "running"

wait
