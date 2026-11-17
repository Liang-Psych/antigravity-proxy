#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."

echo "Validating Docker container tag rules..."
python3 scripts/test-docker-tags.py

echo "Validating accounts, admin, and routing configuration..."
go test -race -run '^(TestCLI|TestConfig|TestLogin|TestAdmin|TestModel)' ./cmd/antigravity-proxy ./internal/config ./internal/proxy

echo "Validating CLI release updates and model catalog tooling..."
python3 -m unittest discover -s scripts -p 'test_*.py'

echo "Project checks passed."
