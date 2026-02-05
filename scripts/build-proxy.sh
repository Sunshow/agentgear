#!/bin/bash
cd "$(dirname "$0")/../proxy"
echo "Building Proxy..."
go build -ldflags="-w -s" -o agentgear ./cmd/agentgear/
echo "Done: proxy/agentgear"
