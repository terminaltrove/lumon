#!/bin/sh
# Publication gate: development may be unreleased, an installable plugin may not.
set -eu
[ "$#" -eq 1 ] || { echo "usage: scripts/check-plugin-release.sh TAG" >&2; exit 1; }
tag=$1
printf '%s\n' "$tag" | LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || { echo "plugin-release: expected a release tag" >&2; exit 1; }
root=$(cd "$(dirname "$0")/.." && pwd)
for plugin in claude-code pi opencode; do
    dir=$root/plugins/$plugin
    [ "$(cat "$dir/version")" = "$tag" ] || { echo "plugin-release: $plugin is not pinned to $tag" >&2; exit 1; }
    LC_ALL=C awk '
    BEGIN {
        split("lumon_darwin_amd64 lumon_darwin_arm64 lumon_linux_amd64 lumon_linux_arm64 lumon_linux_386 lumon_linux_arm lumon_freebsd_amd64 lumon_freebsd_arm64 lumon_netbsd_amd64 lumon_netbsd_arm64 lumon_openbsd_amd64 lumon_openbsd_arm64", names, " ")
        for (i in names) required[names[i]]=1
    }
    {
        if (NF != 2 || length($1) != 64 || $1 !~ /^[0-9a-f]+$/ || !($2 in required) || seen[$2]++) bad=1
    }
    END {
        for (name in required) if (!(name in seen)) bad=1
        if (bad) exit 1
    }' "$dir/checksums.sha256sum" || { echo "plugin-release: $plugin needs exactly seven valid platform pins" >&2; exit 1; }
    cmp -s "$root/plugins/claude-code/checksums.sha256sum" "$dir/checksums.sha256sum" || { echo "plugin-release: plugin checksums differ" >&2; exit 1; }
done
echo "plugin-release: all three plugins pinned to $tag"
