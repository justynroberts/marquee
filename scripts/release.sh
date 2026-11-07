#!/usr/bin/env bash
# Build, sign, notarise and package Marquee.app as a disk image.
#
#   scripts/release.sh --dry-run 0.1.0   build, sign, notarise, staple, DMG, verify; no git, no publish
#   scripts/release.sh 0.1.0             the above, then tag v0.1.0, push and create a GitHub release
#   scripts/release.sh --local           build and ad-hoc/Developer-ID sign only (no notarisation)
#
# Releases are cut locally: CI has neither the certificate nor the notary profile.
set -euo pipefail
cd "$(dirname "$0")/.."

DRY=0; LOCAL=0; VERSION=""
for a in "$@"; do
  case "$a" in
    --dry-run) DRY=1 ;;
    --local) LOCAL=1 ;;
    -*) echo "unknown flag $a" >&2; exit 2 ;;
    *) VERSION="$a" ;;
  esac
done
[[ -z "$VERSION" ]] && VERSION="0.0.0-dev"
APP=Marquee
BUNDLE_ID=com.fintonlabs.marquee
OUT=dist
STAGE="$OUT/stage"
APPDIR="$OUT/$APP.app"
DMG="$OUT/$APP-$VERSION.dmg"
PROFILE=notarytool

say() { printf '\n== %s\n' "$*"; }
die() { printf 'release: %s\n' "$*" >&2; exit 1; }

# ---------------------------------------------------------------- 1. refuse first
IDENTITY=$(security find-identity -v -p codesigning | sed -n 's/.*"\(Developer ID Application: [^"]*\)".*/\1/p' | head -1)
if [[ $LOCAL -eq 0 ]]; then
  [[ -n "$IDENTITY" ]] || die "no Developer ID Application certificate in the keychain"
  say "checking the notary profile"
  xcrun notarytool history --keychain-profile "$PROFILE" >/dev/null 2>&1 \
    || die "notarytool profile '$PROFILE' does not answer (missing, or Apple account agreement pending)"
  if [[ $DRY -eq 0 ]]; then
    git rev-parse --git-dir >/dev/null 2>&1 || die "not a git repository"
    [[ -z "$(git status --porcelain)" ]] || die "working tree is dirty"
    git fetch --tags origin >/dev/null 2>&1 || true
    git rev-parse "v$VERSION" >/dev/null 2>&1 && die "tag v$VERSION already exists"
  fi
fi
[[ -n "$IDENTITY" ]] || IDENTITY="-"

say "tests"
go vet ./...
go test ./... >/dev/null

# ---------------------------------------------------------------- 2-3. build and sign
say "building $APP $VERSION (arm64)"
command rm -rf "$OUT"; mkdir -p "$APPDIR/Contents/MacOS" "$APPDIR/Contents/Resources"
CGO_ENABLED=1 GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" \
  -o "$APPDIR/Contents/MacOS/marquee" ./cmd/marquee
cp assets/Marquee.icns "$APPDIR/Contents/Resources/Marquee.icns"
cat > "$APPDIR/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP</string>
  <key>CFBundleDisplayName</key><string>$APP</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleExecutable</key><string>marquee</string>
  <key>CFBundleIconFile</key><string>Marquee</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$VERSION</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>LSUIElement</key><true/>
  <key>LSArchitecturePriority</key><array><string>arm64</string></array>
  <key>NSHumanReadableCopyright</key><string>Made by FintonLabs. MIT licence. Includes FreeType, HarfBuzz and code ported from Pillow; see Resources.</string>
  <key>NSMicrophoneUsageDescription</key><string>Marquee listens to sound only while an audio visualiser is playing, to draw it on the panel. Nothing is recorded or sent anywhere.</string>
  <key>NSCameraUsageDescription</key><string>Marquee uses the camera only while a camera scene is playing, to draw it on the panel. Nothing is recorded or sent anywhere.</string>
  <key>NSLocalNetworkUsageDescription</key><string>Marquee can let other devices on your network control the panel, and advertises itself so apps can find it.</string>
  <key>NSAppleEventsUsageDescription</key><string>Marquee asks Music and Spotify what is playing, to show the track on the panel. It never changes anything in them.</string>
  <key>NSBonjourServices</key><array><string>_marquee._tcp</string></array>
</dict>
</plist>
PLIST
cp LICENSE "$APPDIR/Contents/Resources/LICENSE.txt"
cp internal/emoji/LICENSE-TWEMOJI "$APPDIR/Contents/Resources/LICENSE-TWEMOJI.txt"
cp internal/jpeg444/LICENSE-GO "$APPDIR/Contents/Resources/LICENSE-GO.txt"
cp internal/ft/freetype/FTL.TXT "$APPDIR/Contents/Resources/LICENSE-FREETYPE.txt"
cp internal/ft/harfbuzz/COPYING "$APPDIR/Contents/Resources/LICENSE-HARFBUZZ.txt"
cp internal/pil/LICENSE-PILLOW "$APPDIR/Contents/Resources/LICENSE-PILLOW.txt"

say "signing with: $IDENTITY"
codesign --force --options runtime --timestamp --entitlements scripts/entitlements.plist \
  --sign "$IDENTITY" "$APPDIR"
codesign --verify --strict --deep -vv "$APPDIR"

# Paths of this machine must not be baked in (other than Go module source paths, trimmed above).
if strings -a "$APPDIR/Contents/MacOS/marquee" | grep -q "/Users/"; then
  die "binary contains /Users/ paths"
fi

say "smoke test: launch a copy, away from the checkout"
SMOKE=$(mktemp -d)
ditto "$APPDIR" "$SMOKE/$APP.app"
"$SMOKE/$APP.app/Contents/MacOS/marquee" version | grep -q "$VERSION" || die "smoke: version"
"$SMOKE/$APP.app/Contents/MacOS/marquee" serve --no-panel --addr 127.0.0.1:7799 --data "$SMOKE/data" >"$SMOKE/log" 2>&1 &
PID=$!
for _ in $(seq 1 30); do curl -sf http://127.0.0.1:7799/v1 >/dev/null && break; sleep 0.2; done
curl -sf http://127.0.0.1:7799/v1 | grep -q '"Marquee"' || { cat "$SMOKE/log"; kill $PID; die "smoke: API did not answer"; }
sleep 1
[[ $(curl -sf http://127.0.0.1:7799/v1/preview.jpg | wc -c) -gt 1000 ]] || { kill $PID; die "smoke: no frames rendered"; }
kill $PID; wait $PID 2>/dev/null || true
command rm -rf "$SMOKE"

if [[ $LOCAL -eq 1 ]]; then
  say "local build done: $APPDIR"; exit 0
fi

# ---------------------------------------------------------------- 4. notarise and staple the app
notarise() {
  local file="$1" log
  log=$(xcrun notarytool submit "$file" --keychain-profile "$PROFILE" --wait 2>&1) || true
  echo "$log" | tail -3
  if ! echo "$log" | grep -q "status: Accepted"; then
    local id; id=$(echo "$log" | sed -n 's/^ *id: \(.*\)/\1/p' | head -1)
    [[ -n "$id" ]] && xcrun notarytool log "$id" --keychain-profile "$PROFILE" || true
    die "notarisation refused for $file"
  fi
}
say "notarising the app"
ditto -c -k --keepParent "$APPDIR" "$OUT/app.zip"
notarise "$OUT/app.zip"
xcrun stapler staple "$APPDIR"
command rm -f "$OUT/app.zip"

# ---------------------------------------------------------------- 5-6. image: build, sign, notarise, staple
say "building the disk image"
mkdir -p "$STAGE"
ditto "$APPDIR" "$STAGE/$APP.app"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "$APP $VERSION" -srcfolder "$STAGE" -format UDZO -ov "$DMG" >/dev/null
command rm -rf "$STAGE"
codesign --force --timestamp --sign "$IDENTITY" "$DMG"
say "notarising the disk image"
notarise "$DMG"
xcrun stapler staple "$DMG"

# ---------------------------------------------------------------- 7. verify the artifact
say "verifying as a downloader would"
VERIFY=$(mktemp -d); cp "$DMG" "$VERIFY/"
V="$VERIFY/$(basename "$DMG")"
xattr -w com.apple.quarantine "0083;00000000;Safari;" "$V"
xcrun stapler validate "$V"
spctl -a -vvv -t open --context context:primary-signature "$V" 2>&1 | tee "$VERIFY/dmg.txt"
grep -q "Notarized Developer ID" "$VERIFY/dmg.txt" || die "disk image is not accepted as Notarized Developer ID"
MNT=$(hdiutil attach -nobrowse -readonly "$V" | grep Volumes | awk -F'\t' '{print $NF}')
spctl -a -vvv -t exec "$MNT/$APP.app" 2>&1 | tee "$VERIFY/app.txt"
xcrun stapler validate "$MNT/$APP.app"
hdiutil detach "$MNT" >/dev/null
grep -q "Notarized Developer ID" "$VERIFY/app.txt" || die "app is not accepted as Notarized Developer ID"
command rm -rf "$VERIFY"
say "verified: $DMG ($(du -h "$DMG" | cut -f1))"

if [[ $DRY -eq 1 ]]; then
  say "dry run: stopping before git and GitHub"; exit 0
fi

# ---------------------------------------------------------------- 8. publish
say "tagging and publishing v$VERSION"
git tag "v$VERSION"
git push origin HEAD "v$VERSION"
NOTES=$(mktemp)
{
  printf 'macOS 12 or later, Apple silicon. Signed and notarised: download the disk image, open it and drag Marquee to Applications.\n\n'
  # This version's section of CHANGELOG.md, if there is one.
  awk -v v="## $VERSION" '$0==v{on=1;next} /^## /{on=0} on' CHANGELOG.md 2>/dev/null
} > "$NOTES"
gh release create "v$VERSION" "$DMG" --title "Marquee $VERSION" --notes-file "$NOTES"
