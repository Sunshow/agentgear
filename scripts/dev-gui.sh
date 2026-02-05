#!/bin/bash
cd "$(dirname "$0")/../gui"

if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

npm run tauri:dev
