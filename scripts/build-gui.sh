#!/bin/bash
cd "$(dirname "$0")/../gui"

if [ ! -d "node_modules" ]; then
    echo "Installing dependencies..."
    npm install
fi

echo "Building GUI..."
npm run tauri:build
echo "Done: gui/src-tauri/target/release/"
