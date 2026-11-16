# Thin wrappers over the scripts and go commands, so everything that can be
# run is listed in one place (`make help`).

SHELL := /bin/bash
PKG   := ./cmd/lumon

.DEFAULT_GOAL := help

## help: list what there is
help:
	@grep -hE '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /' | column -t -s ':'

## run: play locally
run:
	go run $(PKG)

## host: host a LAN game
host:
	go run $(PKG) --host

## build: cross-build every target into dist/, with checksums
build:
	./scripts/build.sh

## test: the whole suite under the race detector
test:
	go test ./... -race -count=1

## test-fast: the suite without the race detector, for iterating
test-fast:
	go test ./... -count=1

## vet: go vet for this machine and for 32-bit, which catches constant overflow
vet:
	go vet ./...
	GOOS=linux GOARCH=386 go vet ./...

## fmt: gofmt every package
fmt:
	gofmt -w .

## lint: golangci-lint, if it is installed
lint:
	@command -v golangci-lint >/dev/null 2>&1 \
		|| { echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run

## check: fmt, vet and the full suite, what CI runs
check: fmt-check vet test

fmt-check:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "not gofmt'd:"; echo "$$out"; exit 1; fi

## clean: remove build output
clean:
	rm -rf dist prof

.PHONY: help run host build test test-fast vet fmt fmt-check lint check clean
