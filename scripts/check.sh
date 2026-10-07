#!/usr/bin/env bash
# Runs every check CI runs (except the Linux-only race detector) and stops at
# the first failure. Use before committing: scripts/check.sh && git commit ...
set -eo pipefail
cd "$(dirname "$0")/.."
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	echo "gofmt needed:"; echo "$unformatted"; exit 1
fi
go vet ./...
"$(go env GOPATH)/bin/staticcheck" ./...
go test -count=1 ./... | grep -v "no test files"
GOOS=js GOARCH=wasm go build ./...
echo "all checks passed"
