#!/bin/sh
# Build release archives locally and in CI with the same command.
set -eu
cd "$(dirname "$0")/.."

source_version=$(cat VERSION)
version=${VERSION:-v$source_version}
go_command=${GO:-go}
if ! printf '%s\n' "$source_version" | LC_ALL=C grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || [ "$version" != "v$source_version" ]; then
    echo 'VERSION must match v plus the version in VERSION' >&2
    exit 1
fi

stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT HUP INT TERM
rm -rf dist
mkdir -p dist

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64; do
    target_os=${target%/*}
    target_arch=${target#*/}
    target_dir="$stage/${target_os}_${target_arch}"
    archive="plous_${version}_${target_os}_${target_arch}.tar.gz"
    mkdir -p "$target_dir"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" "$go_command" build \
        -trimpath -buildvcs=false -o "$target_dir/plous" .
    COPYFILE_DISABLE=1 tar -czf "dist/$archive" -C "$target_dir" plous -C "$PWD" \
        README.md LICENSE docs assets/logo/lockup.svg assets/logo/lockup-dark.svg
    printf 'Built %s\n' "$archive"
done

cd dist
shasum -a 256 ./*.tar.gz > checksums.txt
shasum -a 256 -c checksums.txt

# Exercise the downloaded-archive path on the build host when supported.
native_os=$("$go_command" env GOOS)
native_arch=$("$go_command" env GOARCH)
native_archive="plous_${version}_${native_os}_${native_arch}.tar.gz"
if [ -f "$native_archive" ]; then
    mkdir -p "$stage/smoke"
    tar -xzf "$native_archive" -C "$stage/smoke"
    "$stage/smoke/plous" --version > "$stage/actual-output.txt"
    printf '%s\n' "$source_version" | cmp - "$stage/actual-output.txt"
    printf 'Packaged executable: OK (%s/%s)\n' "$native_os" "$native_arch"
fi
