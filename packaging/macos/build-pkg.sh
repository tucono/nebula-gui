#!/bin/sh
# @authorship: AI-Assisted
# @architect: AI (Claude Opus 5.5)
#
# Builds the macOS installer from binaries make package-macos has built:
#
#   packaging/macos/build-pkg.sh <version>
#
# It expects dist/macos/{amd64,arm64}/{nebula-gui,nebula-guid},
# dist/icons/nebula-gui.icns and dist/THIRD_PARTY_NOTICES.txt, joins
# each pair into a universal binary, makes the app bundle, and writes
# dist/nebula-gui-<version>-macos-universal.pkg. The package also holds
# Uninstall nebula-gui.app, compiled from uninstall.applescript. Run it
# from the repository's top directory, on a Mac.

set -eu

version=$1
# The bundle's versions must be numbers: 0.1.0-rc.1 gives 0.1.0.
numversion=${version%%-*}
here=packaging/macos
work=dist/macos/pkg
root=$work/root
app=$root/Applications/nebula-gui.app
uninstaller="$root/Applications/Uninstall nebula-gui.app"
libexec=$root/usr/local/libexec/nebula-gui
out=dist/nebula-gui-$version-macos-universal.pkg

rm -rf "$work"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$libexec" "$root/Library/LaunchDaemons"

lipo -create -output "$app/Contents/MacOS/nebula-gui" dist/macos/amd64/nebula-gui dist/macos/arm64/nebula-gui
lipo -create -output "$libexec/nebula-guid" dist/macos/amd64/nebula-guid dist/macos/arm64/nebula-guid
cp "$here/uninstall.sh" "$libexec/uninstall.sh"
cp dist/icons/nebula-gui.icns "$app/Contents/Resources/nebula-gui.icns"
cp dist/THIRD_PARTY_NOTICES.txt "$app/Contents/Resources/THIRD_PARTY_NOTICES.txt"
sed -e "s/@NUMVERSION@/$numversion/g" -e "s/@VERSION@/$version/g" "$here/Info.plist" >"$app/Contents/Info.plist"
printf 'APPL????' >"$app/Contents/PkgInfo"
cp "$here/io.github.tucono.nebula-guid.plist" "$root/Library/LaunchDaemons/"
plutil -lint "$app/Contents/Info.plist" "$root/Library/LaunchDaemons/io.github.tucono.nebula-guid.plist"

# The uninstaller: an AppleScript app with the app's icon and its own ID.
osacompile -o "$uninstaller" "$here/uninstall.applescript"
cp dist/icons/nebula-gui.icns "$uninstaller/Contents/Resources/applet.icns"
rm -f "$uninstaller/Contents/Resources/Assets.car"
plutil -replace CFBundleIdentifier -string io.github.tucono.nebulagui.uninstall "$uninstaller/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$numversion" "$uninstaller/Contents/Info.plist"
plutil -remove CFBundleIconName "$uninstaller/Contents/Info.plist" 2>/dev/null || true

chmod 755 "$app/Contents/MacOS/nebula-gui" "$libexec/nebula-guid" "$libexec/uninstall.sh"
chmod 644 "$app/Contents/Info.plist" "$app/Contents/PkgInfo" "$app/Contents/Resources/nebula-gui.icns" \
	"$app/Contents/Resources/THIRD_PARTY_NOTICES.txt" \
	"$root/Library/LaunchDaemons/io.github.tucono.nebula-guid.plist"

# Ad hoc signatures: Apple silicon runs only signed code, and a signed
# bundle keeps its resources sealed. Neither is a Developer ID (M5).
codesign --force --sign - "$libexec/nebula-guid"
codesign --force --sign - "$app"
codesign --force --sign - "$uninstaller"

# The installer would otherwise look for each app wherever a bundle with
# its identifier is (a development build, say) and update that instead,
# and would skip it if the one there looked newer.
pkgbuild --analyze --root "$root" "$work/component.plist"
i=0
while plutil -extract "$i" xml1 -o /dev/null "$work/component.plist" 2>/dev/null; do
	plutil -replace "$i.BundleIsRelocatable" -bool NO "$work/component.plist"
	plutil -replace "$i.BundleIsVersionChecked" -bool NO "$work/component.plist"
	i=$((i + 1))
done

pkgbuild --root "$root" \
	--component-plist "$work/component.plist" \
	--scripts "$here/scripts" \
	--identifier io.github.tucono.nebula-gui \
	--version "$version" \
	--install-location / \
	"$work/nebula-gui-component.pkg"

sed "s/@VERSION@/$version/g" "$here/Distribution.xml" >"$work/Distribution.xml"
productbuild --distribution "$work/Distribution.xml" --package-path "$work" "$out"
echo "built $out"
