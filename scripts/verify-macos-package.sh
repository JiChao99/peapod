#!/usr/bin/env bash
set -euo pipefail

out="${1:-build/darwin-universal}"
app="$out/Peapod.app"
info="$app/Contents/Info.plist"

test -d "$app"
test -f "$info"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$info")" = "io.github.jichao99.peapod"
expected_version="$(node -p "require('./mygo.json').version")"
test "$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$info")" = "$expected_version"
executable="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleExecutable' "$info")"
binary="$app/Contents/MacOS/$executable"
test -x "$binary"
architectures="$(lipo -info "$binary")"
printf '%s\n' "$architectures"
grep -q 'x86_64' <<<"$architectures"
grep -q 'arm64' <<<"$architectures"
codesign --verify --deep --strict "$app"
shopt -s nullglob
images=("$out"/peapod-*.dmg)
test "${#images[@]}" -eq 1
hdiutil verify "${images[0]}"
