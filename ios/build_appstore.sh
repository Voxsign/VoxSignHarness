#!/bin/bash
# VoxSign App Store build & upload prep
# Prereq: Apple Distribution certificate + App Store provisioning profile installed
#         (developer.apple.com -> Certificates/Profiles), then fill ExportOptions_appstore.plist
set -e
cd "$(dirname "$0")"

SCHEME=VoxSign
TEAM=6ASMXVQHKK
ARCHIVE=/tmp/voxsign-appstore.xcarchive
OUT=/tmp/voxsign-appstore-ipa
APPSTORE_ID="CUA4D3W2ZV"    # App Store Connect API key ID
ISSUER="e48e3228-9b08-4c3f-a60a-6248728e740f"  # API key issuer id

echo "== archive (Release, App Store signing) =="
rm -rf "$ARCHIVE" "$OUT"
xcodebuild archive \
  -project VoxSign.xcodeproj \
  -scheme "$SCHEME" \
  -configuration Release \
  -archivePath "$ARCHIVE" \
  -destination 'generic/platform=iOS' \
  CODE_SIGN_STYLE=Manual \
  DEVELOPMENT_TEAM="$TEAM" \
  CODE_SIGN_IDENTITY="Apple Distribution: Yongming Zou (6ASMXVQHKK)" \
  PROVISIONING_PROFILE_SPECIFIER="VoxSign AI App Store" \
  ASSETCATALOG_COMPILER_APPICON_NAME=AppIcon

echo "== export ipa (app-store) =="
xcodebuild -exportArchive \
  -archivePath "$ARCHIVE" \
  -exportPath "$OUT" \
  -exportOptionsPlist ExportOptions_appstore.plist

IPA=$(ls "$OUT"/*.ipa | head -1)
echo "== built: $IPA =="

echo "== upload to App Store Connect (TestFlight) =="
if [ -n "$APPSTORE_ID" ]; then
  xcrun altool --upload-app -f "$IPA" \
    --apiKey "$APPSTORE_ID" --apiIssuer "$ISSUER" \
    --type ios --verbose
else
  echo "Upload skipped: set APPSTORE_ID/ISSUER (App Store Connect API key) above,"
  echo "or run: xcrun altool --upload-app -f $IPA -u <AppleID> -p <app-specific-password>"
fi
echo "== done =="
