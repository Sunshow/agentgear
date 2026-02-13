#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# 启动 Proxy (后台)
echo "Starting Proxy..."
cd "$PROJECT_ROOT/proxy"
go run ./cmd/agentgear/ --config ./configs/config.yaml &
PROXY_PID=$!
echo "Proxy PID: $PROXY_PID"

# 等待 Proxy 启动
sleep 2

# 启动 GUI
echo "Starting GUI..."
cd "$PROJECT_ROOT/gui"
if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi
npm run tauri:dev

# GUI 退出后清理 Proxy
echo "Stopping Proxy..."
kill $PROXY_PID 2>/dev/null
