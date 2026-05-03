#!/bin/bash

set -e
echo "███████╗ ██╗  ██╗ ██╗   ██╗ ███╗   ██╗"
echo "██╔════╝ ██║  ██║ ██║   ██║ ████╗  ██║"
echo "█████╗     ███╔═╝ ██║   ██║ ██╔██╗ ██║"
echo "██╔══╝   ██╔══██║ ██║   ██║ ██║╚██╗██║"
echo "███████╗ ██║  ██║ ╚██████╔╝ ██║ ╚████║"
echo "╚══════╝ ╚═╝  ╚═╝  ╚═════╝  ╚═╝  ╚═══╝"
echo "Building intrasudo26"

cd "$(dirname "$0")"

echo "Building main application"
go build -o intrasudo26 .

sleep 2

echo "Starting main application"
./intrasudo26 &
MAIN_PID=$!

sleep 2

if command -v python3 >/dev/null 2>&1; then
    echo "Starting discord bot"
    PYTHONUNBUFFERED=1 python3 bot.py &
    BOT_PID=$!
    sleep 1
else
    echo "python3 not found; discord bot not started"
    BOT_PID=""
fi

echo "All services started:"
echo "Main App PID: $MAIN_PID"
echo "Discord Bot PID: $BOT_PID"

echo "running"

cleanup() {
    echo "Stopping all services..."
    kill $BOT_PID 2>/dev/null || true
    kill $MAIN_PID 2>/dev/null || true
    echo "All services stopped."
    exit 0
}

trap cleanup SIGINT SIGTERM

wait
