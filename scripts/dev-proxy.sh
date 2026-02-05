#!/bin/bash
cd "$(dirname "$0")/../proxy"
go run ./cmd/agentgear/ --config ./configs/config.yaml
