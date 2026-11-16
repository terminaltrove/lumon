#!/bin/sh
# Pin all supported plugins only after the complete published release matches
# local binaries and each plugin binary has provenance from the tagged workflow.
# Usage: scripts/pin-plugin.sh [--check] TAG CHECKSUMS_FILE
set -eu
check=0
if [ "${1:-}" = --check ]; then check=1; shift; fi
[ "$#" -eq 2 ] || { echo "usage: scripts/pin-plugin.sh [--check] TAG CHECKSUMS_FILE" >&2; exit 1; }
tag=$1
local_sums=$2
fail() { echo "pin-plugin: $*" >&2; exit 1; }
printf '%s\n' "$tag" | LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || fail "not a release tag like v1.1.0"
root=$(cd "$(dirname "$0")/.." && pwd)
repo=terminaltrove/lumon
signer=https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag
built=$(dirname "$local_sums")
command -v gh >/dev/null 2>&1 || fail "gh is needed to check attestations"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM

# Reject empty, truncated, duplicate or unexpected lists, including Windows.
# Neither side may silently omit a target or smuggle in another path.
normalize() {
    LC_ALL=C awk '
    BEGIN {
        split("lumon_darwin_amd64 lumon_darwin_arm64 lumon_linux_amd64 lumon_linux_arm64 lumon_linux_386 lumon_linux_arm lumon_freebsd_amd64 lumon_freebsd_arm64 lumon_netbsd_amd64 lumon_netbsd_arm64 lumon_openbsd_amd64 lumon_openbsd_arm64 lumon_windows_amd64.exe lumon_windows_arm64.exe", names, " ")
        for (i in names) required[names[i]]=1
    }
    NF == 0 { next }
    {
        name=$2; sub(/^\*/, "", name)
        if (NF != 2 || length($1) != 64 || $1 !~ /^[0-9a-f]+$/ || !(name in required) || seen[name]++) bad=1
        print $1 "  " name
    }
    END {
        for (name in required) if (!(name in seen)) bad=1
        if (bad) exit 1
    }' "$1" > "$work/normalizing" || return 1
    LC_ALL=C sort -k2 "$work/normalizing"
}
normalize "$local_sums" > "$work/local" || fail "local checksums must list every release target exactly once"
curl --proto '=https' --proto-redir '=https' --tlsv1.2 -fsSL --max-time 120 \
    "https://github.com/$repo/releases/download/$tag/checksums.sha256sum" -o "$work/published" || fail "could not fetch published checksums"
normalize "$work/published" > "$work/remote" || fail "published checksums must list every release target exactly once"
cmp -s "$work/local" "$work/remote" || fail "published checksums differ from the local build"

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" > "$work/hash" || return 1
    else
        shasum -a 256 "$1" > "$work/hash" || return 1
    fi
    awk '{print $1}' "$work/hash"
}
# Verify every local file before trusting or updating any plugin metadata.
while read -r want name; do
    file=$built/$name
    [ -f "$file" ] && [ ! -L "$file" ] || fail "missing regular binary: $name"
    actual=$(sha256 "$file") || fail "could not hash $name"
    [ "$actual" = "$want" ] || fail "local binary differs from its checksum: $name"
done < "$work/local"
# The launchers do not run Windows, but its published hash was still compared.
grep -v '  lumon_windows_' "$work/local" > "$work/pins"
while read -r want name; do
    if ! gh attestation verify "$built/$name" --repo "$repo" \
        --cert-identity "$signer" --deny-self-hosted-runners; then
        fail "no valid release-workflow attestation for $name; ensure gh is current"
    fi
done < "$work/pins"
if [ "$check" -eq 1 ]; then
    echo "pin-plugin: verified $tag; no plugin files changed"
    exit 0
fi
for plugin in claude-code pi opencode; do
    [ -d "$root/plugins/$plugin" ] && [ -w "$root/plugins/$plugin" ] || fail "plugin directory is not writable: $plugin"
done
for plugin in claude-code pi opencode; do
    cp "$work/pins" "$root/plugins/$plugin/checksums.sha256sum"
    printf '%s\n' "$tag" > "$root/plugins/$plugin/version"
done
"$root/scripts/check-plugin-release.sh" "$tag"
echo "pin-plugin: pinned Claude Code, Pi and OpenCode to $tag"
