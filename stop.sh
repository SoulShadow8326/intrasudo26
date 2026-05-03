#!/bin/bash

# Gracefully stop services listening on :8080, intrasudo26 binary and bot.py

set -e

cd "$(dirname "$0")"

echo "Stopping services..."

# kill process listening on 8080
pids_8080=$(lsof -ti :8080 || true)
if [ -n "$pids_8080" ]; then
  for p in $pids_8080; do
    echo "Killing process $p listening on :8080"
    kill "$p" 2>/dev/null || true
  done
else
  echo "No process listening on :8080"
fi

# kill intrasudo26 by name
pids_main=$(pgrep -f ./intrasudo26 || true)
if [ -n "$pids_main" ]; then
  for p in $pids_main; do
    echo "Stopping intrasudo26 PID $p"
    kill "$p" 2>/dev/null || true
  done
else
  echo "No intrasudo26 process found"
fi

# kill bot.py by name
pids_bot=$(pgrep -f bot.py || true)
if [ -n "$pids_bot" ]; then
  for p in $pids_bot; do
    echo "Stopping bot.py PID $p"
    kill "$p" 2>/dev/null || true
  done
else
  echo "No bot.py process found"
fi

# wait a moment for graceful shutdown
sleep 1

# force kill any remaining of these
if [ -n "$pids_8080" ]; then
  for p in $pids_8080; do
    if kill -0 "$p" 2>/dev/null; then
      echo "Force killing $p"
      kill -9 "$p" 2>/dev/null || true
    fi
  done
fi
if [ -n "$pids_main" ]; then
  for p in $pids_main; do
    if kill -0 "$p" 2>/dev/null; then
      echo "Force killing $p"
      kill -9 "$p" 2>/dev/null || true
    fi
  done
fi
if [ -n "$pids_bot" ]; then
  for p in $pids_bot; do
    if kill -0 "$p" 2>/dev/null; then
      echo "Force killing $p"
      kill -9 "$p" 2>/dev/null || true
    fi
  done
fi

echo "Stop complete"
