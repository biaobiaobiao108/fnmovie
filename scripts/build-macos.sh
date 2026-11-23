#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ "$(uname -s)/$(uname -m)" != Darwin/arm64 ]]; then
    echo 'Build the Apple Silicon DMG on an arm64 Mac.' >&2
    exit 1
fi

resources=resources/darwin-arm64
mkdir "$resources"
trap 'rm -rf -- "$resources"' EXIT
python3 scripts/bundle-macos-mpv.py "$resources/player"

output=out/fnmovie-macos
go tool mygo build -platform darwin/arm64 -o "$output"

apps=("$output"/darwin-arm64/*.app)
images=("$output"/darwin-arm64/*-arm64.dmg)
[[ ${#apps[@]} -eq 1 && -d "${apps[0]}" && ${#images[@]} -eq 1 && -f "${images[0]}" ]]
app="${apps[0]}"
codesign --verify --deep --strict "$app"
hdiutil verify "${images[0]}"
bundled_lib="$(pwd)/$app/Contents/Resources/player/libmpv.dylib"
test -f "$bundled_lib"
FNMOVIE_TEST_LIBMPV="$bundled_lib" go test ./...
