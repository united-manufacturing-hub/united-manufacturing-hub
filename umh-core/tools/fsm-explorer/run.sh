#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "🔨 Building FSM Explorer..."
go build -o fsm-explorer main.go

echo "🚀 Starting FSM Explorer..."
echo ""
./fsm-explorer
