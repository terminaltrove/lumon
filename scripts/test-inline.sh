#!/usr/bin/env bash
# Exercise both inline adapters with a fresh native binary from this checkout.
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
CGO_ENABLED=0 go build -trimpath -o "$work/lumon" ./cmd/lumon
LUMON_TEST_BINARY="$work/lumon" bun --conditions=browser test plugins/inline.test.ts
