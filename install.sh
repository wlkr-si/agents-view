#!/bin/sh
set -eu

fail() {
    printf 'agents-view: %s\n' "$*" >&2
    exit 1
}

case "$(uname -s)" in
    Darwin) platform=darwin ;;
    Linux) platform=linux ;;
    *) fail "The install script supports macOS and Linux" ;;
esac
case "$(uname -m)" in
    arm64|aarch64) arch=arm64 ;;
    x86_64|amd64) arch=amd64 ;;
    *) fail "Unsupported architecture: $(uname -m)" ;;
esac

release=https://github.com/wlkr-si/agents-view/releases
version=${AGENTS_VIEW_VERSION:-}
version=${version#v}
base="$release/latest/download"
if [ -n "$version" ]; then
    case "$version" in
        *[!0-9A-Za-z.-]*|[!0-9]*) fail "Invalid AGENTS_VIEW_VERSION" ;;
    esac
    base="$release/download/v$version"
fi

protocols='=https'
mirror=${AGENTS_VIEW_DOWNLOAD_BASE_URL:-}
if [ -n "$mirror" ]; then
    base=${mirror%/}
    case "$base" in
        https://*) ;;
        http://*)
            authority=${base#http://}
            authority=${authority%%/*}
            case "$authority" in
                localhost|127.0.0.1|'[::1]') ;;
                localhost:*|127.0.0.1:*|'[::1]':*)
                    case "${authority##*:}" in
                        ''|*[!0-9]*) fail "Invalid loopback mirror port" ;;
                    esac ;;
                *) fail "HTTP mirrors must use a loopback address" ;;
            esac
            protocols='=https,http' ;;
        *) fail "Release downloads require HTTPS (HTTP is available for loopback mirrors)" ;;
    esac
fi

download() {
    curl -q -fsSL --proto "$protocols" --proto-redir '=https' --max-redirs 5 \
        --connect-timeout 15 --max-time 120 "$1" -o "$2"
}

directory=${AGENTS_VIEW_INSTALL_DIR:-"$HOME/.local/bin"}
case "$directory" in
    /*) ;;
    *) directory="$PWD/$directory" ;;
esac
mkdir -p "$directory"
work=$(mktemp -d "$directory/.agents-view.XXXXXX")
trap 'rm -rf "$work"' 0
trap 'exit 1' HUP INT TERM

download "$base/checksums.txt" "$work/checksums.txt"
# The checksum file identifies the version selected by GitHub's latest redirect.
suffix="_${platform}_${arch}.tar.gz"
asset=$(awk -v version="$version" -v suffix="$suffix" '
    length($1) == 64 && $1 ~ /^[0-9a-f]+$/ && NF == 2 {
        name = $2
        if (length(name) <= length(suffix) ||
            substr(name, length(name) - length(suffix) + 1) != suffix) next
        stem = substr(name, 1, length(name) - length(suffix))
        if (stem !~ /^agents-view_[0-9][0-9A-Za-z.-]*$/) next
        if (version != "" && stem != "agents-view_" version) next
        selected = name
        count++
    }
    END { if (count == 1) print selected; else exit 1 }
' "$work/checksums.txt") || fail "Missing or ambiguous SHA-256 checksum for $platform/$arch"
expected=$(awk -v asset="$asset" '$2 == asset { print $1 }' "$work/checksums.txt")

if [ -z "$mirror" ] && [ -z "$version" ]; then
    # Pin the asset to the same version even if a newer release goes live meanwhile.
    version=${asset#agents-view_}
    version=${version%"$suffix"}
    base="$release/download/v$version"
fi
download "$base/$asset" "$work/$asset"
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum < "$work/$asset")
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 < "$work/$asset")
else
    fail "Install sha256sum or shasum to verify the download"
fi
[ "${actual%% *}" = "$expected" ] || fail "SHA-256 checksum mismatch for $asset"

tar -xzf "$work/$asset" -C "$work" agents-view
chmod 755 "$work/agents-view"
[ ! -d "$directory/agents-view" ] || fail "$directory/agents-view is a directory"
mv -f "$work/agents-view" "$directory/agents-view"
printf 'Installed %s (SHA-256 verified)\n' "$directory/agents-view"
case ":${PATH:-}:" in
    *":$directory:"*) ;;
    *) printf 'Add %s to PATH to run agents-view by name.\n' "$directory" ;;
esac
