#!/usr/bin/env bash
#
# Cross-build lumon and check the result.
#
# Needs only the Go toolchain.

set -euo pipefail

# Run from the repository root, wherever this was invoked from.
cd "$(dirname "$0")/.."

OUT="${OUT:-dist}"
PKG="./cmd/lumon"

# Keep linux/386: addressBase is 0xFFC00000, which overflows a 32-bit int, and
# only building for a 32-bit target catches a mistake with it.
TARGETS="${TARGETS:-
darwin/arm64
darwin/amd64
linux/amd64
linux/arm64
linux/386
linux/arm
freebsd/amd64
freebsd/arm64
netbsd/amd64
netbsd/arm64
openbsd/amd64
openbsd/arm64
windows/amd64
windows/arm64
}"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

printf '\nlumon %s\n\n' "$VERSION"

# Test before building.
if [ "${SKIP_TESTS:-0}" = "0" ]; then
    echo "  vet and test"
    go vet ./...
    go test ./... -count=1 >/dev/null
    echo "  ok"
fi

rm -rf "$OUT"
mkdir -p "$OUT"

for t in $TARGETS; do
    goos="${t%%/*}"
    goarch="${t##*/}"
    name="lumon_${goos}_${goarch}"
    [ "$goos" = "windows" ] && name="$name.exe"

    # CGO off for a static binary that runs in a container with no libc.
    # -trimpath keeps the build machine's paths out of it; "-w -s" drops DWARF
    # and the symbol table. -buildvcs=false leaves out the commit, so a binary
    # depends only on the source and VERSION, and the plugins can be pinned in
    # the commit that is tagged.
    # GOARM=6 makes linux/arm run on every 32-bit Raspberry Pi, the Zero and
    # Pi 1 included; other architectures ignore it.
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM=6 \
        go build -trimpath -buildvcs=false -ldflags="-w -s -X main.version=$VERSION" \
        -o "$OUT/$name" "$PKG"

    printf '  %-28s %s\n' "$name" "$(du -h "$OUT/$name" | cut -f1)"
done

# Beside the binaries, so `sha256sum -c` works from inside dist/.
( cd "$OUT" && shasum -a 256 lumon_* > checksums.sha256sum 2>/dev/null \
    || sha256sum lumon_* > checksums.sha256sum )

printf '\n  %s/checksums.sha256sum\n' "$OUT"
printf '  verify with:  ( cd %s && shasum -a 256 -c checksums.sha256sum )\n\n' "$OUT"
